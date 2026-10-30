import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { AuthService } from '../../../core/services/auth.service';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';

type ErrorHttp = { error?: { message?: string } };

@Component({
  selector: 'app-cambiar-contrasena',
  standalone: true,
  imports: [FormsModule, RouterLink, TranslatePipe, PasswordFieldComponent],
  templateUrl: './cambiar-contrasena.component.html',
  styleUrl: './cambiar-contrasena.component.css',
})
export class CambiarContrasenaComponent {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  protected actual = '';
  protected nueva = '';
  protected repetir = '';
  protected readonly nuevaVisible = signal(false);
  protected readonly enviando = signal(false);
  protected readonly error = signal('');

  protected guardar(): void {
    if (this.enviando() || (!this.actual && !this.nueva && !this.repetir)) { return; }
    this.error.set('');
    if (this.nueva.length < 8) {
      this.error.set('ERROR_CONTRASENA_MINIMO_8_CARACTERES');
      return;
    }
    if (!this.nuevaVisible() && this.nueva !== this.repetir) {
      this.error.set('ERROR_CONTRASENAS_NO_COINCIDEN');
      return;
    }

    this.enviando.set(true);
    this.auth.cambiarPassword(this.actual, this.nueva).subscribe({
      next: () => {
        this.enviando.set(false);
        void this.router.navigateByUrl('/allstarlink');
      },
      error: (error: ErrorHttp) => {
        this.error.set(error.error?.message ?? 'ERROR_COMPLETAR_OPERACION');
        this.enviando.set(false);
      },
    });
  }
}
