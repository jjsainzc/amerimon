import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { PJSIPExtension } from '../../../core/models/pjsip-extension.model';
import { PjsipUsersService } from '../../../core/services/pjsip-users.service';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';
import { isRadioAmateurCallsign } from '../../../core/utils/radio-callsign';

@Component({
  selector: 'app-pjsip-users',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, PasswordFieldComponent, CopyValueDirective, ConfirmacionDialogComponent, TranslatePipe],
  templateUrl: './pjsip-users.component.html',
  styleUrls: ['./pjsip-users.component.css']
})
export class PjsipUsersComponent implements OnInit {
  private readonly service = inject(PjsipUsersService);

  readonly extensionesList = signal<PJSIPExtension[]>([]);
  readonly cargando = signal(false);
  readonly error = signal('');
  readonly editando = signal(false);
  readonly esEdicion = signal(false);
  readonly guardando = signal(false);
  readonly pendienteEliminar = signal<string | null>(null);
  readonly confirmacionGuardadoPendiente = signal(false);

  readonly extActual = signal<PJSIPExtension>({
    id: '',
    context: 'sip-phones',
    disallow: ['all'],
    allow: ['ulaw', 'alaw', 'slin', 'gsm'],
    caller_id: '"PJSIP-User"',
    password: '',
    max_contacts: '4',
    direct_media: 'no',
    media_encryption: 'no'
  });

  ngOnInit(): void {
    this.cargarExtensiones();
  }

  cargarExtensiones(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.listar().subscribe({
      next: (data) => {
        this.extensionesList.set(Array.isArray(data) ? data : []);
        this.cargando.set(false);
      },
      error: (error: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(error.error?.message ?? error.error?.error ?? 'PJSIP_USERS_LOAD_ERROR');
      }
    });
  }

  nuevaExtension(): void {
    this.error.set('');
    this.confirmacionGuardadoPendiente.set(false);
    this.esEdicion.set(false);
    this.extActual.set({
      id: '',
      context: 'sip-phones',
      disallow: ['all'],
      allow: ['ulaw', 'alaw', 'slin', 'gsm'],
      caller_id: '"PJSIP-User"',
      password: '',
      max_contacts: '4',
      direct_media: 'no',
      media_encryption: 'no'
    });
    this.editando.set(true);
  }

  seleccionarExtension(ext: PJSIPExtension): void {
    this.error.set('');
    this.confirmacionGuardadoPendiente.set(false);
    this.esEdicion.set(true);
    this.extActual.set({ ...ext, allow: [...(ext.allow ?? [])], disallow: [...(ext.disallow ?? [])] });
    this.editando.set(true);
  }

  guardarExtension(): void {
    if (this.guardando()) { return; }
    if (!isRadioAmateurCallsign(this.extActual().id)) {
      this.confirmacionGuardadoPendiente.set(true);
      return;
    }
    this.persistirExtension();
  }

  confirmarGuardado(): void {
    if (!this.confirmacionGuardadoPendiente() || this.guardando()) { return; }
    this.confirmacionGuardadoPendiente.set(false);
    this.persistirExtension();
  }

  cancelarGuardado(): void {
    if (!this.guardando()) { this.confirmacionGuardadoPendiente.set(false); }
  }

  private persistirExtension(): void {
    if (this.guardando()) { return; }
    this.error.set('');
    this.guardando.set(true);
    this.service.guardar(this.extActual()).subscribe({
      next: () => {
        this.guardando.set(false);
        this.editando.set(false);
        this.cargarExtensiones();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.error.set(error.error?.message ?? error.error?.error ?? 'PJSIP_USERS_SAVE_ERROR');
      }
    });
  }

  eliminarExtension(id: string): void {
    if (this.guardando()) { return; }
    this.error.set('');
    this.pendienteEliminar.set(id);
  }

  cancelarEliminacion(): void {
    if (!this.guardando()) { this.pendienteEliminar.set(null); }
  }

  confirmarEliminacion(): void {
    const id = this.pendienteEliminar();
    if (!id || this.guardando()) { return; }
    this.guardando.set(true);
    this.service.eliminar(id).subscribe({
      next: () => {
        this.guardando.set(false);
        this.pendienteEliminar.set(null);
        this.editando.set(false);
        this.cargarExtensiones();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.pendienteEliminar.set(null);
        this.error.set(error.error?.message ?? error.error?.error ?? 'PJSIP_USERS_DELETE_ERROR');
      }
    });
  }
}
