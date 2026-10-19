import { Component, HostListener, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from './core/services/auth.service';
import { SiteSettings, SiteSettingsService } from './core/services/site-settings.service';
import { SystemInfoDialogComponent } from './shared/system-info-dialog/system-info-dialog.component';
import { PasswordFieldComponent } from './shared/password-field/password-field.component';

type Modo = 'login' | 'crear' | 'cambiar';
type ErrorHttp = { error?: { message?: string } };

@Component({
  imports: [RouterOutlet, RouterLink, RouterLinkActive, FormsModule, PasswordFieldComponent, SystemInfoDialogComponent],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App implements OnInit {
  private readonly auth = inject(AuthService);
  private readonly siteSettingsService = inject(SiteSettingsService);

  protected readonly esAdmin = this.auth.esAdmin;
  protected readonly headerOculto = signal(false);
  protected readonly ajustesCabecera = signal<SiteSettings>({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
  protected readonly tituloAplicacion = computed(() => this.ajustesCabecera().title.trim() || 'Amerimon');
  protected readonly subtituloAplicacion = computed(() => {
    const settings = this.ajustesCabecera();
    const subtitle = settings.subtitle.trim();
    const version = settings.asterisk_version ? `Asterisk ${settings.asterisk_version}` : '';
    return [subtitle, version].filter(Boolean).join(' · ') || 'Monitoreo AllStarLink';
  });
  protected readonly menuAbierto = signal(false);
  protected readonly dialogoAbierto = signal(false);
  protected readonly sistemaAbierto = signal(false);
  protected readonly edicionCabeceraAbierta = signal(false);
  protected readonly guardandoCabecera = signal(false);
  protected readonly errorCabecera = signal('');
  protected tituloCabecera = 'Amerimon';
  protected subtituloCabecera = '';
  protected readonly modo = signal<Modo>('login');
  protected readonly error = signal('');
  protected readonly aviso = signal('');
  protected readonly enviando = signal(false);

  protected usuario = '';
  protected password = '';
  protected nueva = '';
  protected repetir = '';
  private ultimaPosicionScroll = 0;

  ngOnInit(): void {
    this.cargarAjustesCabecera();
  }

  protected readonly titulo = computed(() =>
    this.modo() === 'crear' ? 'Definir contraseña de administrador'
      : this.modo() === 'cambiar' ? 'Cambiar contraseña'
      : 'Acceso de administrador'
  );

  protected alternarMenu(): void {
    this.menuAbierto.update(v => !v);
  }

  @HostListener('window:scroll')
  protected controlarHeader(): void {
    const posicion = window.scrollY;
    this.headerOculto.set(posicion > this.ultimaPosicionScroll && posicion > 100);
    this.ultimaPosicionScroll = posicion;
  }

  @HostListener('document:keydown.escape')
  protected cerrarMenu(): void {
    this.menuAbierto.set(false);
  }

  protected abrirLogin(): void {
    this.menuAbierto.set(false);
    this.limpiar();
    this.dialogoAbierto.set(true);
    this.modo.set('login');
    this.auth.configurada().subscribe({
      next: (r) => this.modo.set(r.configured ? 'login' : 'crear'),
      error: () => this.error.set('No se pudo consultar el estado del servidor'),
    });
  }

  protected abrirCambio(): void {
    this.menuAbierto.set(false);
    this.limpiar();
    this.modo.set('cambiar');
    this.dialogoAbierto.set(true);
  }

  protected abrirSistema(): void {
    this.menuAbierto.set(false);
    this.sistemaAbierto.set(true);
  }

  protected cerrarSistema(): void {
    this.sistemaAbierto.set(false);
  }

  protected abrirEdicionCabecera(): void {
    this.menuAbierto.set(false);
    this.tituloCabecera = this.ajustesCabecera().title;
    this.subtituloCabecera = this.ajustesCabecera().subtitle;
    this.errorCabecera.set('');
    this.edicionCabeceraAbierta.set(true);
  }

  protected guardarCabecera(): void {
    this.errorCabecera.set('');
    this.guardandoCabecera.set(true);
    this.siteSettingsService.guardar(this.tituloCabecera, this.subtituloCabecera).subscribe({
      next: settings => {
        this.ajustesCabecera.set(settings);
        this.guardandoCabecera.set(false);
        this.edicionCabeceraAbierta.set(false);
      },
      error: error => {
        this.guardandoCabecera.set(false);
        this.errorCabecera.set(error.error?.message ?? 'No se pudo guardar el encabezado.');
      }
    });
  }

  protected cerrar(): void {
    this.dialogoAbierto.set(false);
    this.limpiar();
  }

  protected salir(): void {
    this.menuAbierto.set(false);
    this.auth.logout();
  }

  protected enviar(): void {
    this.error.set('');
    this.aviso.set('');
    const modo = this.modo();

    if (modo !== 'login' && this.nueva.length < 8) {
      this.error.set('La nueva contraseña debe tener al menos 8 caracteres');
      return;
    }
    if (modo !== 'login' && this.nueva !== this.repetir) {
      this.error.set('Las contraseñas no coinciden');
      return;
    }

    this.enviando.set(true);
    const alFallar = (e: ErrorHttp) => {
      this.enviando.set(false);
      this.error.set(e.error?.message ?? 'No se pudo completar la operación');
    };

    if (modo === 'login') {
      this.auth.login(this.usuario, this.password).subscribe({ next: () => this.cerrar(), error: alFallar });
    } else if (modo === 'crear') {
      this.auth.configurar(this.usuario, this.nueva).subscribe({ next: () => this.cerrar(), error: alFallar });
    } else {
      this.auth.cambiarPassword(this.password, this.nueva).subscribe({
        next: () => this.cerrar(),
        error: alFallar,
      });
    }
  }

  private limpiar(): void {
    this.usuario = this.password = this.nueva = this.repetir = '';
    this.error.set('');
    this.aviso.set('');
    this.enviando.set(false);
  }

  private cargarAjustesCabecera(): void {
    this.siteSettingsService.obtener().subscribe({
      next: settings => this.ajustesCabecera.set(settings),
      error: () => undefined,
    });
  }
}
