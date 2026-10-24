import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Subject, catchError, finalize, of, switchMap } from 'rxjs';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { AstdbCoincidencia, Favorito, FavoritoEntrada } from '../../../core/models/favorito.model';
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
  private readonly solicitudesBusqueda = new Subject<string | null>();

  readonly favoritos = signal<Favorito[]>([]);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly exito = signal('');
  readonly formularioAbierto = signal(false);
  readonly editandoId = signal<number | null>(null);
  readonly pendienteBorrar = signal<Favorito | null>(null);
  readonly sugerida = signal('');
  readonly nodoSeleccionado = signal('');
  readonly coincidencias = signal<AstdbCoincidencia[]>([]);
  readonly buscando = signal(false);
  readonly errorBusqueda = signal('');
  readonly estadoBusqueda = signal('');
  readonly avisoBusqueda = signal('');

  readonly form = this.fb.nonNullable.group({
    busqueda: ['', [Validators.required, Validators.maxLength(120)]],
    descripcion: ['', [Validators.maxLength(120)]]
  });

  /** Carga favoritos y prepara la búsqueda explícita de nodos de AstDB. */
  ngOnInit(): void {
    this.cargar();
    this.form.controls.busqueda.valueChanges.pipe(
      takeUntilDestroyed(this.destroyRef)
    ).subscribe(() => this.solicitudesBusqueda.next(null));
    this.solicitudesBusqueda.pipe(
      switchMap(query => {
        this.nodoSeleccionado.set('');
        this.sugerida.set('');
        this.coincidencias.set([]);
        this.errorBusqueda.set('');
        this.estadoBusqueda.set('');
        this.avisoBusqueda.set('');
        if (!query) {
          this.buscando.set(false);
          return of<AstdbCoincidencia[] | null>(null);
        }
        this.buscando.set(true);
        return this.svc.buscarAstdb(query).pipe(
          catchError(error => {
            this.errorBusqueda.set(this.mensajeError(error));
            return of<AstdbCoincidencia[] | null>(null);
          }),
          finalize(() => this.buscando.set(false))
        );
      }),
      takeUntilDestroyed(this.destroyRef)
    ).subscribe(results => {
      if (results === null) { return; }
      this.coincidencias.set(results.slice(0, 50));
      if (results.length > 50) {
        this.avisoBusqueda.set(`Se encontraron ${results.length} nodos. Solo se muestran los primeros 50 para evitar sobrecargar el navegador. Detalla más la búsqueda para reducir la cantidad de resultados.`);
      }
      if (results.length === 1) {
        this.elegirCoincidencia(results[0]);
      } else if (results.length > 1) {
        this.estadoBusqueda.set(`Se encontraron ${results.length} nodos. Elige el destino deseado.`);
      } else if (this.form.controls.busqueda.value.trim()) {
        this.estadoBusqueda.set('No se encontraron nodos en AstDB para esa búsqueda.');
      }
    });
  }

  buscar(): void {
    const control = this.form.controls.busqueda;
    control.markAsTouched();
    if (control.invalid || !control.value.trim() || this.guardando()) { return; }
    this.solicitudesBusqueda.next(control.value.trim());
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
    this.solicitudesBusqueda.next(null);
    this.editandoId.set(null);
    this.error.set('');
    this.sugerida.set('');
    this.nodoSeleccionado.set('');
    this.coincidencias.set([]);
    this.errorBusqueda.set('');
    this.estadoBusqueda.set('');
    this.form.reset({ busqueda: '', descripcion: '' });
    this.formularioAbierto.set(true);
  }

  /** Carga los datos del favorito seleccionado en el formulario. */
  abrirEditar(f: Favorito): void {
    this.solicitudesBusqueda.next(null);
    this.editandoId.set(f.id);
    this.error.set('');
    this.sugerida.set(f.descripcion_astdb ?? '');
    this.nodoSeleccionado.set(f.numero);
    this.coincidencias.set([]);
    this.errorBusqueda.set('');
    this.estadoBusqueda.set('Nodo guardado seleccionado.');
    this.form.reset({ busqueda: f.numero, descripcion: f.descripcion }, { emitEvent: false });
    this.formularioAbierto.set(true);
  }

  /** Cierra el formulario solo cuando no hay una operación de guardado. */
  cerrarFormulario(): void {
    if (this.guardando()) { return; }
    this.solicitudesBusqueda.next(null);
    this.formularioAbierto.set(false);
  }

  /** Valida y crea o actualiza el favorito según el modo de edición. */
  guardar(): void {
    this.form.markAllAsTouched();
    if (this.form.invalid || this.guardando() || this.buscando()) { return; }
    if (!this.nodoSeleccionado()) {
      this.errorBusqueda.set('Selecciona un nodo de los resultados de AstDB.');
      return;
    }

    const v = this.form.getRawValue();
    const datos: FavoritoEntrada = {
      numero: this.nodoSeleccionado(),
      descripcion: v.descripcion.trim()
    };
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

  /** Selecciona el nodo de AstDB que se guardará como favorito. */
  elegirCoincidencia(coincidencia: AstdbCoincidencia): void {
    this.avisoBusqueda.set('');
    this.nodoSeleccionado.set(coincidencia.nodo);
    this.sugerida.set(this.textoCoincidencia(coincidencia));
    this.estadoBusqueda.set(`Nodo ${coincidencia.nodo} seleccionado.`);
    this.coincidencias.set([]);
  }

  /** Presenta los datos de AstDB asociados a una coincidencia de indicativo. */
  textoCoincidencia(coincidencia: AstdbCoincidencia): string {
    return [coincidencia.callsign, coincidencia.descripcion, coincidencia.ubicacion]
      .map(value => value.trim()).filter(Boolean).join(' ');
  }

  /** Extrae el mensaje del servidor o devuelve uno genérico para la interfaz. */
  private mensajeError(error: any): string {
    return error?.error?.message || 'No se pudo completar la operación.';
  }
}
