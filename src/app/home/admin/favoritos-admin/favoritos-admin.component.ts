import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { EMPTY, catchError, debounceTime, distinctUntilChanged, map, switchMap } from 'rxjs';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Favorito, FavoritoEntrada } from '../../../core/models/favorito.model';
import { FavoritosService } from '../../../core/services/favoritos.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';
import { LoadingIndicatorComponent } from '../../../shared/loading-indicator/loading-indicator.component';

@Component({
  selector: 'app-favoritos-admin',
  standalone: true,
  imports: [ReactiveFormsModule, AdminVolverComponent, ConfirmacionDialogComponent, LoadingIndicatorComponent],
  templateUrl: './favoritos-admin.component.html',
  styleUrl: './favoritos-admin.component.css'
})
export class FavoritosAdminComponent implements OnInit {
  private readonly svc = inject(FavoritosService);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);

  readonly favoritos = signal<Favorito[]>([]);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly exito = signal('');
  readonly formularioAbierto = signal(false);
  readonly editandoId = signal<number | null>(null);
  readonly pendienteBorrar = signal<Favorito | null>(null);
  readonly sugerida = signal('');

  readonly form = this.fb.nonNullable.group({
    numero: ['', [Validators.required, Validators.pattern(/^[A-Za-z0-9][A-Za-z0-9._/-]{0,31}$/)]],
    descripcion: ['', [Validators.maxLength(120)]]
  });

  /** Carga los favoritos y conecta el formulario con la sugerencia de astdb. */
  ngOnInit(): void {
    this.cargar();
    // Sugiere la descripción de astdb.txt al escribir el número del nodo.
    this.form.controls.numero.valueChanges.pipe(
      map(v => v.trim()),
      debounceTime(300),
      distinctUntilChanged(),
      switchMap(numero => {
        if (!this.form.controls.numero.valid) { this.sugerida.set(''); return EMPTY; }
        return this.svc.descripcionAstdb(numero).pipe(catchError(() => { this.sugerida.set(''); return EMPTY; }));
      }),
      takeUntilDestroyed(this.destroyRef)
    ).subscribe(r => this.sugerida.set(r.descripcion ?? ''));
  }

  /** Obtiene la lista de favoritos y comunica errores de consulta. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.svc.listar().subscribe({
      next: items => {
        this.favoritos.set(Array.isArray(items) ? items : []);
        this.cargando.set(false);
      },
      error: err => {
        this.cargando.set(false);
        this.error.set(this.mensajeError(err));
      }
    });
  }

  /** Prepara el formulario vacío para crear un favorito. */
  abrirNuevo(): void {
    this.editandoId.set(null);
    this.error.set('');
    this.sugerida.set('');
    this.form.reset({ numero: '', descripcion: '' });
    this.formularioAbierto.set(true);
  }

  /** Carga los datos del favorito seleccionado en el formulario. */
  abrirEditar(f: Favorito): void {
    this.editandoId.set(f.id);
    this.error.set('');
    this.sugerida.set(f.descripcion_astdb ?? '');
    this.form.reset({ numero: f.numero, descripcion: f.descripcion });
    this.formularioAbierto.set(true);
  }

  /** Cierra el formulario solo cuando no hay una operación de guardado. */
  cerrarFormulario(): void {
    if (this.guardando()) { return; }
    this.formularioAbierto.set(false);
  }

  /** Valida y crea o actualiza el favorito según el modo de edición. */
  guardar(): void {
    this.form.markAllAsTouched();
    if (this.form.invalid || this.guardando()) { return; }

    const v = this.form.getRawValue();
    const datos: FavoritoEntrada = { numero: v.numero.trim(), descripcion: v.descripcion.trim() };
    const id = this.editandoId();
    this.guardando.set(true);
    this.error.set('');
    this.exito.set('');
    const peticion = id === null ? this.svc.crear(datos) : this.svc.actualizar(id, datos);
    peticion.subscribe({
      next: () => {
        this.guardando.set(false);
        this.formularioAbierto.set(false);
        this.exito.set(id === null ? 'Favorito creado.' : 'Favorito actualizado.');
        this.cargar();
      },
      error: err => {
        this.guardando.set(false);
        this.error.set(this.mensajeError(err));
      }
    });
  }

  /** Selecciona el favorito que se presentará para confirmación de borrado. */
  solicitarBorrado(f: Favorito): void {
    this.error.set('');
    this.pendienteBorrar.set(f);
  }

  /** Descarta la solicitud de borrado pendiente. */
  cancelarBorrado(): void {
    this.pendienteBorrar.set(null);
  }

  /** Elimina el favorito seleccionado y sincroniza la lista local. */
  confirmarBorrado(): void {
    const f = this.pendienteBorrar();
    if (!f) { return; }
    this.svc.borrar(f.id).subscribe({
      next: () => {
        this.pendienteBorrar.set(null);
        this.favoritos.update(items => items.filter(i => i.id !== f.id));
        this.exito.set('Favorito eliminado.');
      },
      error: err => this.error.set(this.mensajeError(err))
    });
  }

  /** Extrae el mensaje del servidor o devuelve uno genérico para la interfaz. */
  private mensajeError(error: any): string {
    return error?.error?.message || 'No se pudo completar la operación.';
  }
}
