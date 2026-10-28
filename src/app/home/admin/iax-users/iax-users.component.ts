import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { IAXUser } from '../../../core/models/iax-user.model';
import { IaxUsersService } from '../../../core/services/iax-users.service';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';
import { isRadioAmateurCallsign } from '../../../core/utils/radio-callsign';

@Component({
  selector: 'app-iax-users',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, PasswordFieldComponent, CopyValueDirective, ConfirmacionDialogComponent, TranslatePipe],
  templateUrl: './iax-users.component.html',
  styleUrls: ['./iax-users.component.css']
})
export class IaxUsersComponent implements OnInit {
  private readonly service = inject(IaxUsersService);

  readonly usuariosList = signal<IAXUser[]>([]);
  readonly cargando = signal(false);
  readonly error = signal('');
  readonly editando = signal(false);
  readonly esEdicion = signal(false);
  readonly guardando = signal(false);
  readonly pendienteEliminar = signal<string | null>(null);
  readonly confirmacionGuardadoPendiente = signal(false);

  readonly usuarioActual = signal<IAXUser>({
    name: '',
    type: 'friend',
    context: 'iax-client',
    auth: 'md5',
    secret: '',
    host: 'dynamic',
    disallow: ['all'],
    allow: ['ulaw', 'adpcm', 'gsm'],
    transfer: 'no'
  });

  ngOnInit(): void {
    this.cargarUsuarios();
  }

  cargarUsuarios(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.listar().subscribe({
      next: (data) => {
        this.usuariosList.set(data ? Object.values(data) : []);
        this.cargando.set(false);
      },
      error: (error: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(error.error?.message ?? error.error?.error ?? 'IAX_USERS_LOAD_ERROR');
      }
    });
  }

  nuevoUsuario(): void {
    this.error.set('');
    this.confirmacionGuardadoPendiente.set(false);
    this.esEdicion.set(false);
    this.usuarioActual.set({
      name: '',
      type: 'friend',
      context: 'iax-client',
      auth: 'md5',
      secret: '',
      host: 'dynamic',
      disallow: ['all'],
      allow: ['ulaw', 'adpcm', 'gsm'],
      transfer: 'no'
    });
    this.editando.set(true);
  }

  seleccionarUsuario(u: IAXUser): void {
    this.error.set('');
    this.confirmacionGuardadoPendiente.set(false);
    this.esEdicion.set(true);
    this.usuarioActual.set({ ...u, allow: [...(u.allow ?? [])], disallow: [...(u.disallow ?? [])] });
    this.editando.set(true);
  }

  puedeEliminarUsuario(name: string): boolean {
    return isRadioAmateurCallsign(name);
  }

  guardarUsuario(): void {
    if (this.guardando()) { return; }
    if (!isRadioAmateurCallsign(this.usuarioActual().name)) {
      this.confirmacionGuardadoPendiente.set(true);
      return;
    }
    this.persistirUsuario();
  }

  confirmarGuardado(): void {
    if (!this.confirmacionGuardadoPendiente() || this.guardando()) { return; }
    this.confirmacionGuardadoPendiente.set(false);
    this.persistirUsuario();
  }

  cancelarGuardado(): void {
    if (!this.guardando()) { this.confirmacionGuardadoPendiente.set(false); }
  }

  private persistirUsuario(): void {
    if (this.guardando()) { return; }
    this.error.set('');
    this.guardando.set(true);
    this.service.guardar(this.usuarioActual()).subscribe({
      next: () => {
        this.guardando.set(false);
        this.editando.set(false);
        this.cargarUsuarios();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.error.set(error.error?.message ?? error.error?.error ?? 'IAX_USERS_SAVE_ERROR');
      }
    });
  }

  eliminarUsuario(name: string): void {
    if (this.guardando() || !this.puedeEliminarUsuario(name)) { return; }
    this.error.set('');
    this.pendienteEliminar.set(name);
  }

  cancelarEliminacion(): void {
    if (!this.guardando()) { this.pendienteEliminar.set(null); }
  }

  confirmarEliminacion(): void {
    const name = this.pendienteEliminar();
    if (!name || this.guardando() || !this.puedeEliminarUsuario(name)) { return; }
    this.guardando.set(true);
    this.service.eliminar(name).subscribe({
      next: () => {
        this.guardando.set(false);
        this.pendienteEliminar.set(null);
        this.editando.set(false);
        this.cargarUsuarios();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.pendienteEliminar.set(null);
        this.error.set(error.error?.message ?? error.error?.error ?? 'IAX_USERS_DELETE_ERROR');
      }
    });
  }
}
