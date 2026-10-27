import { Component, HostListener, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from './core/services/auth.service';
import { SiteSettings, SiteSettingsService } from './core/services/site-settings.service';
import { SystemInfoDialogComponent } from './shared/system-info-dialog/system-info-dialog.component';
import { PasswordFieldComponent } from './shared/password-field/password-field.component';
import { EnlaceExterno } from './core/models/enlace-externo.model';
import { EnlacesExternosService } from './core/services/enlaces-externos.service';
import { WeatherComponent } from './shared/weather/weather.component';
import { AboutModalComponent } from './shared/about-modal/about-modal.component';
import { MarkdownViewerComponent } from './shared/markdown-viewer/markdown-viewer.component';
import { TranslationService } from './core/services/translation.service';
import { TranslatePipe } from './core/pipes/translate.pipe';

type Modo = 'login' | 'crear' | 'cambiar';
type ErrorHttp = { error?: { message?: string } };

@Component({
  imports: [ TranslatePipe, MarkdownViewerComponent, AboutModalComponent, RouterOutlet, RouterLink, RouterLinkActive, FormsModule, PasswordFieldComponent, SystemInfoDialogComponent, WeatherComponent],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App implements OnInit {
  private readonly auth = inject(AuthService);
  private readonly siteSettingsService = inject(SiteSettingsService);
  private readonly enlacesService = inject(EnlacesExternosService);

  protected readonly esAdmin = this.auth.esAdmin;
  protected readonly headerOculto = signal(false);
  protected readonly anioActual = new Date().getFullYear();
  protected readonly ajustesCabecera = signal<SiteSettings>({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
  /** Devuelve el título configurado o el nombre predeterminado del sitio. */
  /** Ubicación del clima configurada; `undefined` mientras no se han cargado los ajustes. */
  protected readonly ubicacionClima = signal<string | undefined>(undefined);
  protected readonly tituloAplicacion = computed(() => this.ajustesCabecera().title.trim() || 'Amerimon');
  /** Combina el subtítulo y la versión de Asterisk, con texto alternativo si faltan. */
  protected readonly subtituloAplicacion = computed(() => {
    const settings = this.ajustesCabecera();
    const subtitle = settings.subtitle.trim();
    const version = settings.asterisk_version ? `Asterisk ${settings.asterisk_version}` : '';
    return [subtitle, version].filter(Boolean).join(' · ') || 'Monitoreo AllStarLink';
  });
  protected readonly menuAbierto = signal(false);
  protected readonly adminMenuAbierto = signal(false);
  protected readonly dialogoAbierto = signal(false);
  protected readonly sistemaAbierto = signal(false);
  protected readonly edicionCabeceraAbierta = signal(false);
  protected readonly guardandoCabecera = signal(false);
  protected readonly errorCabecera = signal('');
  protected tituloCabecera = 'Amerimon';
  protected subtituloCabecera = '';
  protected ubicacionClimaCabecera = '';
  protected readonly modo = signal<Modo>('login');
  protected readonly nuevaContrasenaVisible = signal(false);
  protected readonly error = signal('');
  protected readonly aviso = signal('');
  protected readonly enviando = signal(false);
  protected readonly enlacesAbiertos = signal(false);
  protected readonly enlacesExternos = signal<EnlaceExterno[]>([]);
  protected readonly cargandoEnlaces = signal(false);
  protected readonly errorEnlaces = signal('');

  readonly isAboutOpen = signal(false);
  readonly isMarkdownOpen = signal(false);

  public translationService = inject(TranslationService);
  public idiomaMenuAbierto = signal<boolean>(false);

  protected usuario = '';
  protected recordarSesion = false;
  protected password = '';
  protected nueva = '';
  protected repetir = '';
  private ultimaPosicionScroll = 0;

  constructor(translationService: TranslationService) {}

  /** Carga al iniciar la aplicación la configuración visible de la cabecera. */
  ngOnInit(): void {
    this.cargarAjustesCabecera();

    // Puedes verificar el idioma actual que tiene el servicio o forzar uno por defecto
    const idiomaInicial = this.translationService.currentLang() || 'es';

    // Opcional: Si quieres asegurar que cargue explícitamente al iniciar
    this.translationService.loadLanguage(idiomaInicial);
  }

  /** Abrir manual */
  abrirManual(): void {
    this.isMarkdownOpen.set(true);
  }

  /** Cambia el idioma de la aplicación. */
  alternarMenuIdioma() {
    this.idiomaMenuAbierto.update(v => !v);
  }

  seleccionarIdioma(lang: string) {
    // 1. Cambiamos el estado del menú inmediatamente
    this.idiomaMenuAbierto.set(false);

    // 2. Llamamos al servicio para cargar el idioma
    this.translationService.loadLanguage(lang);
  }

  /** Abrir acerca de */
  abrirAcercaDe(): void {
    this.isAboutOpen.set(true);
  }

  /** Devuelve la clave del título adecuado al modo actual del diálogo de acceso. */
  protected readonly titulo = computed(() =>
    this.modo() === 'crear' ? 'DEFINIR_CONTRASENA_ADMINISTRADOR'
      : this.modo() === 'cambiar' ? 'CAMBIAR_CONTRASENA'
      : 'ACCESO_ADMINISTRADOR'
  );

  /** Alterna la visibilidad del menú principal. */
  protected alternarMenu(): void {
    this.menuAbierto.update(v => !v);
  }

  /** Alterna el submenú de administración. */
  protected alternarMenuAdmin(): void {
    this.adminMenuAbierto.update(open => !open);
  }

  /** Cierra el submenú al hacer clic fuera de su botón y opciones. */
  @HostListener('document:click', ['$event'])
  protected cerrarMenuAdminFuera(event: MouseEvent): void {
    const target = event.target;
    if (!(target instanceof Element) || !target.closest('.admin-desplegable')) {
      this.adminMenuAbierto.set(false);
    }
  }

  /** Oculta la cabecera al desplazarse hacia abajo y la muestra al subir. */
  @HostListener('window:scroll')
  protected controlarHeader(): void {
    const posicion = window.scrollY;
    this.headerOculto.set(posicion > this.ultimaPosicionScroll && posicion > 100);
    this.ultimaPosicionScroll = posicion;
  }

  /** Cierra los menús al pulsar Escape. */
  @HostListener('document:keydown.escape')
  protected cerrarMenu(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
  }

  /** Abre los enlaces externos y carga los destinos activos desde el servicio. */
  protected abrirEnlaces(): void {
    this.cerrarMenu();
    this.enlacesAbiertos.set(true);
    this.cargandoEnlaces.set(true);
    this.errorEnlaces.set('');
    this.enlacesService.listarActivos().subscribe({
      next: enlaces => {
        this.enlacesExternos.set(enlaces);
        this.cargandoEnlaces.set(false);
      },
      error: (error: ErrorHttp) => {
        this.cargandoEnlaces.set(false);
        this.errorEnlaces.set(error.error?.message ?? 'No se pudieron cargar los enlaces externos.');
      }
    });
  }

  /** Abre el acceso administrativo y decide si corresponde iniciar sesión o configurar. */
  protected abrirLogin(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
    this.limpiar();
    this.password = this.auth.passwordRecordada();
    this.dialogoAbierto.set(true);
    this.modo.set('login');
    this.auth.configurada().subscribe({
      next: (r) => this.modo.set(r.configured ? 'login' : 'crear'),
      error: () => this.error.set('ERROR_CONSULTAR_ESTADO_SERVIDOR'),
    });
  }

  /** Abre el diálogo para cambiar la contraseña de la sesión actual. */
  protected abrirCambio(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
    this.limpiar();
    this.modo.set('cambiar');
    this.dialogoAbierto.set(true);
  }

  /** Muestra el diálogo con información del sistema. */
  protected abrirSistema(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
    this.sistemaAbierto.set(true);
  }

  /** Oculta el diálogo de información del sistema. */
  protected cerrarSistema(): void {
    this.sistemaAbierto.set(false);
  }

  /** Copia los ajustes actuales a los campos de edición de la cabecera. */
  protected abrirEdicionCabecera(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
    this.tituloCabecera = this.ajustesCabecera().title;
    this.subtituloCabecera = this.ajustesCabecera().subtitle;
    this.ubicacionClimaCabecera = this.ajustesCabecera().weather_location ?? '';
    this.errorCabecera.set('');
    this.edicionCabeceraAbierta.set(true);
  }

  /** Guarda los textos de cabecera y actualiza la vista con la respuesta recibida. */
  protected guardarCabecera(): void {
    this.errorCabecera.set('');
    this.guardandoCabecera.set(true);
    this.siteSettingsService.guardar(this.tituloCabecera, this.subtituloCabecera, this.ubicacionClimaCabecera).subscribe({
      next: settings => {
        this.ajustesCabecera.set(settings);
        this.ubicacionClima.set(settings.weather_location?.trim() ?? '');
        this.guardandoCabecera.set(false);
        this.edicionCabeceraAbierta.set(false);
      },
      error: error => {
        this.guardandoCabecera.set(false);
        this.errorCabecera.set(error.error?.message ?? 'HEADER_SAVE_ERROR');
      }
    });
  }

  /** Cierra el diálogo de acceso y limpia los campos sensibles. */
  protected cerrar(): void {
    this.dialogoAbierto.set(false);
    this.limpiar();
  }

  /** Cierra el menú e inicia el cierre de sesión. */
  protected salir(): void {
    this.menuAbierto.set(false);
    this.adminMenuAbierto.set(false);
    this.auth.logout();
  }

  /** Valida las contraseñas y ejecuta el flujo de autenticación seleccionado. */
  protected enviar(): void {
    this.error.set('');
    this.aviso.set('');
    const modo = this.modo();

    if (modo !== 'login' && this.nueva.length < 8) {
      this.error.set('ERROR_CONTRASENA_MINIMO_8_CARACTERES');
      return;
    }
    if (modo !== 'login' && this.nueva !== this.repetir) {
      this.error.set('ERROR_CONTRASENAS_NO_COINCIDEN');
      return;
    }

    this.enviando.set(true);
    const alFallar = (e: ErrorHttp) => {
      this.enviando.set(false);
      this.error.set(e.error?.message ?? 'ERROR_COMPLETAR_OPERACION');
    };

    if (modo === 'login') {
      this.auth.login(this.usuario, this.password, this.recordarSesion).subscribe({ next: () => this.cerrar(), error: alFallar });
    } else if (modo === 'crear') {
      this.auth.configurar(this.usuario, this.nueva, this.recordarSesion).subscribe({ next: () => this.cerrar(), error: alFallar });
    } else {
      this.auth.cambiarPassword(this.password, this.nueva).subscribe({
        next: () => this.cerrar(),
        error: alFallar,
      });
    }
  }

  /** Borra contraseñas y restaura el usuario y la casilla desde la preferencia persistente. */
  private limpiar(): void {
    this.password = this.nueva = this.repetir = '';
    this.nuevaContrasenaVisible.set(false);
    this.usuario = this.auth.usuarioRecordado();
    this.recordarSesion = this.auth.recordarSesion();
    this.error.set('');
    this.aviso.set('');
    this.enviando.set(false);
  }

  /** Recupera los ajustes de cabecera; conserva los valores actuales si falla. */
  private cargarAjustesCabecera(): void {
    this.siteSettingsService.obtener().subscribe({
      next: settings => {
        this.ajustesCabecera.set(settings);
        this.ubicacionClima.set(settings.weather_location?.trim() ?? '');
      },
      error: () => this.ubicacionClima.set(''),
    });
  }
}
