import { Component, HostListener, OnInit, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { NavigationCancel, NavigationEnd, NavigationError, NavigationStart, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from './core/services/auth.service';
import { SiteSettingsService } from './core/services/site-settings.service';
import { SystemInfoDialogComponent } from './shared/system-info-dialog/system-info-dialog.component';
import { PasswordFieldComponent } from './shared/password-field/password-field.component';
import { EnlaceExterno } from './core/models/enlace-externo.model';
import { EnlacesExternosService } from './core/services/enlaces-externos.service';
import { WeatherComponent } from './shared/weather/weather.component';
import { AboutModalComponent } from './shared/about-modal/about-modal.component';
import { TranslationService } from './core/services/translation.service';
import { TranslatePipe } from './core/pipes/translate.pipe';
import { NoticePreferencesService } from './core/services/notice-preferences.service';
import { LoadingIndicatorComponent } from './shared/loading-indicator/loading-indicator.component';

type Modo = 'login' | 'crear';
type ErrorHttp = { error?: { message?: string } };

@Component({
  imports: [ TranslatePipe, AboutModalComponent, RouterOutlet, RouterLink, RouterLinkActive, FormsModule, PasswordFieldComponent, SystemInfoDialogComponent, WeatherComponent, LoadingIndicatorComponent],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App implements OnInit {
  protected readonly noticePreferences = inject(NoticePreferencesService);
  private readonly auth = inject(AuthService);
  private readonly siteSettingsService = inject(SiteSettingsService);
  private readonly enlacesService = inject(EnlacesExternosService);
  private readonly router = inject(Router);

  protected readonly esAdmin = this.auth.esAdmin;
  protected readonly headerOculto = signal(false);
  protected readonly navegacionCargando = signal(false);
  protected readonly anioActual = new Date().getFullYear();
  protected readonly ajustesCabecera = this.siteSettingsService.settings;
  protected readonly ajustesCabeceraCargados = signal(false);
  /** Espera los ajustes antes de elegir entre ubicación configurada y automática. */
  protected readonly ubicacionClima = computed(() => this.ajustesCabeceraCargados()
    ? this.siteSettingsService.settings().weather_location?.trim() ?? ''
    : undefined);
  protected readonly tituloAplicacion = computed(() => this.siteSettingsService.settings().title.trim() || 'Amerimon');
  /** Combina el subtítulo y la versión de Asterisk, con texto alternativo si faltan. */
  protected readonly subtituloAplicacion = computed(() => {
    const settings = this.siteSettingsService.settings();
    const subtitle = settings.subtitle.trim();
    const version = settings.asterisk_version ? `Asterisk ${settings.asterisk_version}` : '';
    return [subtitle, version].filter(Boolean).join(' · ') || 'Monitoreo AllStarLink';
  });
  protected readonly menuAbierto = signal(false);
  protected readonly adminMenuAbierto = signal(false);
  protected readonly dialogoAbierto = signal(false);
  protected readonly sistemaAbierto = signal(false);
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

  public translationService = inject(TranslationService);
  public idiomaMenuAbierto = signal<boolean>(false);

  protected usuario = '';
  protected recordarSesion = false;
  protected password = '';
  protected nueva = '';
  protected repetir = '';
  private ultimaPosicionScroll = 0;

  constructor() {
    this.router.events.pipe(takeUntilDestroyed()).subscribe(event => {
      if (event instanceof NavigationStart) {
        this.navegacionCargando.set(true);
      } else if (event instanceof NavigationEnd || event instanceof NavigationCancel || event instanceof NavigationError) {
        this.navegacionCargando.set(false);
      }
    });
  }

  /** Carga al iniciar la aplicación la configuración visible de la cabecera. */
  ngOnInit(): void {
    this.cargarAjustesCabecera();

    // Puedes verificar el idioma actual que tiene el servicio o forzar uno por defecto
    const idiomaInicial = this.translationService.currentLang() || 'es';

    // Opcional: Si quieres asegurar que cargue explícitamente al iniciar
    this.translationService.loadLanguage(idiomaInicial);
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

    // 3. Sincronizamos el idioma con el servidor para la pantalla Nextion
    this.translationService.syncServerLanguage(lang);
  }

  /** Abrir acerca de */
  abrirAcercaDe(): void {
    this.isAboutOpen.set(true);
  }

  /** Devuelve la clave del título adecuado al modo actual del diálogo de acceso. */
  protected readonly titulo = computed(() =>
    this.modo() === 'crear' ? 'DEFINIR_CONTRASENA_ADMINISTRADOR'
      : 'ACCESO_ADMINISTRADOR'
  );

  /** Alterna la visibilidad del menú principal. */
  protected alternarMenu(): void {
    this.menuAbierto.update(v => !v);
    if (this.menuAbierto()) {
      this.headerOculto.set(false);
    }
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

  /** Oculta la cabecera al bajar, salvo mientras se utiliza el menú móvil. */
  @HostListener('window:scroll')
  protected controlarHeader(): void {
    const posicion = window.scrollY;
    this.headerOculto.set(!this.menuAbierto() && posicion > this.ultimaPosicionScroll && posicion > 100);
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

    if (modo === 'crear' && this.nueva.length < 8) {
      this.error.set('ERROR_CONTRASENA_MINIMO_8_CARACTERES');
      return;
    }
    if (modo === 'crear' && !this.nuevaContrasenaVisible() && this.nueva !== this.repetir) {
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
    } else {
      this.auth.configurar(this.usuario, this.nueva, this.recordarSesion).subscribe({ next: () => this.cerrar(), error: alFallar });
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
      next: () => this.ajustesCabeceraCargados.set(true),
      error: error => {
        this.ajustesCabeceraCargados.set(true);
        console.error('No se pudieron cargar los ajustes de cabecera.', error);
      },
    });
  }
}
