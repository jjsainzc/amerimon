import { Component, HostListener, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from './core/services/auth.service';
import { PasswordFieldComponent } from './shared/password-field/password-field.component';

type Modo = 'login' | 'crear' | 'cambiar';
type ErrorHttp = { error?: { message?: string } };

@Component({
  imports: [RouterOutlet, RouterLink, RouterLinkActive, FormsModule, PasswordFieldComponent],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {
  private readonly auth = inject(AuthService);

  protected readonly esAdmin = this.auth.esAdmin;
  protected readonly menuAbierto = signal(false);
  protected readonly dialogoAbierto = signal(false);
  protected readonly modo = signal<Modo>('login');
  protected readonly error = signal('');
  protected readonly aviso = signal('');
  protected readonly enviando = signal(false);

  protected usuario = '';
  protected password = '';
  protected nueva = '';
  protected repetir = '';

  protected readonly titulo = computed(() =>
    this.modo() === 'crear' ? 'Definir contraseña de administrador'
      : this.modo() === 'cambiar' ? 'Cambiar contraseña'
      : 'Acceso de administrador'
  );

  protected alternarMenu(): void {
    this.menuAbierto.update(v => !v);
  }

  @HostListener('document:keydown.escape')
  protected cerrarMenu(): void {
    this.menuAbierto.set(false);
  }

  protected abrirLogin(): void {
    this.limpiar();
    this.dialogoAbierto.set(true);
    this.modo.set('login');
    this.auth.configurada().subscribe({
      next: (r) => this.modo.set(r.configured ? 'login' : 'crear'),
      error: () => this.error.set('No se pudo consultar el estado del servidor'),
    });
  }

  protected abrirCambio(): void {
    this.limpiar();
    this.modo.set('cambiar');
    this.dialogoAbierto.set(true);
  }

  protected cerrar(): void {
    this.dialogoAbierto.set(false);
    this.limpiar();
  }

  protected salir(): void {
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
}
