import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { EnlaceExterno, EnlaceExternoEntrada } from '../../../core/models/enlace-externo.model';
import { EnlacesExternosService } from '../../../core/services/enlaces-externos.service';
import { TranslationService } from '../../../core/services/translation.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { FormChanges } from '../../../core/utils/form-changes';

@Component({
  selector: 'app-enlaces-externos-admin',
  standalone: true,
  imports: [ReactiveFormsModule, AdminVolverComponent, ConfirmacionDialogComponent, TranslatePipe],
  templateUrl: './enlaces-externos-admin.component.html',
  styleUrl: './enlaces-externos-admin.component.css'
})
export class EnlacesExternosAdminComponent implements OnInit {
  private readonly service = inject(EnlacesExternosService);
  private readonly formBuilder = inject(FormBuilder);
  private readonly translation = inject(TranslationService);
  readonly cambios = new FormChanges();

  readonly enlaces = signal<EnlaceExterno[]>([]);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly exito = signal('');
  readonly formularioAbierto = signal(false);
  readonly editandoId = signal<number | null>(null);
  readonly pendienteBorrar = signal<EnlaceExterno | null>(null);
  readonly borrando = signal(false);

  readonly form = this.formBuilder.nonNullable.group({
    descripcion: ['', [Validators.required, Validators.maxLength(255)]],
    url: ['', [Validators.required, Validators.maxLength(255), Validators.pattern(/^https?:\/\/\S+$/i)]],
    target: ['_blank', Validators.required],
    is_active: [true]
  });

  /** Carga los enlaces administrables al inicializar la pantalla. */
  ngOnInit(): void {
    this.cargar();
  }

  /** Consulta los enlaces y refleja el resultado o el error en la interfaz. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.listarAdmin().subscribe({
      next: enlaces => {
        this.enlaces.set(Array.isArray(enlaces) ? enlaces : []);
        this.cargando.set(false);
      },
      error: error => {
        this.cargando.set(false);
        this.error.set(this.mensajeError(error));
      }
    });
  }

  /** Inicializa el formulario para registrar un enlace externo. */
  abrirNuevo(): void {
    this.editandoId.set(null);
    this.error.set('');
    this.form.reset({ descripcion: '', url: '', target: '_blank', is_active: true });
    this.cambios.reset(this.form.getRawValue());
    this.formularioAbierto.set(true);
  }

  /** Rellena el formulario con los datos del enlace que se va a editar. */
  abrirEditar(enlace: EnlaceExterno): void {
    this.editandoId.set(enlace.dashid);
    this.error.set('');
    this.form.reset({
      descripcion: enlace.descripcion ?? '',
      url: enlace.url ?? '',
      target: enlace.target || '_blank',
      is_active: enlace.is_active === 1
    });
    this.cambios.reset(this.form.getRawValue());
    this.formularioAbierto.set(true);
  }

  /** Cierra el formulario si no hay un guardado en curso. */
  cerrarFormulario(): void {
    if (!this.guardando()) { this.formularioAbierto.set(false); }
  }

  /** Valida y persiste el enlace, creando uno nuevo o actualizando el existente. */
  guardar(): void {
    this.form.markAllAsTouched();
    if (this.form.invalid || this.guardando() || !this.cambios.changed(this.form.getRawValue())) { return; }
    const values = this.form.getRawValue();
    const input: EnlaceExternoEntrada = {
      descripcion: values.descripcion.trim(),
      url: values.url.trim(),
      target: values.target,
      is_active: values.is_active ? 1 : 0
    };
    const id = this.editandoId();
    this.guardando.set(true);
    this.error.set('');
    this.exito.set('');
    const request = id === null ? this.service.crear(input) : this.service.actualizar(id, input);
    request.subscribe({
      next: () => {
        this.guardando.set(false);
        this.formularioAbierto.set(false);
        this.exito.set(this.translation.translate(id === null ? 'ENLACE_EXTERNO_CREADO' : 'ENLACE_EXTERNO_ACTUALIZADO'));
        this.cargar();
      },
      error: error => {
        this.guardando.set(false);
        this.error.set(this.mensajeError(error));
      }
    });
  }

  /** Marca el enlace elegido para que la interfaz solicite confirmar su borrado. */
  solicitarBorrado(enlace: EnlaceExterno): void {
    this.error.set('');
    this.pendienteBorrar.set(enlace);
  }

  /** Cancela el borrado pendiente cuando no hay una eliminación en curso. */
  cancelarBorrado(): void {
    if (!this.borrando()) { this.pendienteBorrar.set(null); }
  }

  /** Elimina el enlace confirmado y actualiza la lista o informa del error. */
  confirmarBorrado(): void {
    const enlace = this.pendienteBorrar();
    if (!enlace || this.borrando()) { return; }
    this.borrando.set(true);
    this.service.borrar(enlace.dashid).subscribe({
      next: () => {
        this.borrando.set(false);
        this.pendienteBorrar.set(null);
        this.enlaces.update(enlaces => enlaces.filter(item => item.dashid !== enlace.dashid));
        this.exito.set(this.translation.translate('ENLACE_EXTERNO_ELIMINADO'));
      },
      error: error => {
        this.borrando.set(false);
        this.pendienteBorrar.set(null);
        this.error.set(this.mensajeError(error));
      }
    });
  }

  /** Obtiene el mensaje enviado por el servidor o un texto alternativo. */
  private mensajeError(error: HttpErrorResponse): string {
    return error.error?.message ?? this.translation.translate('NO_SE_PUDO_COMPLETAR_LA_OPERACION');
  }
}
