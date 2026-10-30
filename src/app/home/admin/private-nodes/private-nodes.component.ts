import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PrivateNodesService } from '../../../core/services/private-nodes.service';
import { TranslationService } from '../../../core/services/translation.service';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { FormChanges } from '../../../core/utils/form-changes';

@Component({
  selector: 'app-private-nodes',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, CopyValueDirective, TranslatePipe],
  templateUrl: './private-nodes.component.html',
  styleUrl: './private-nodes.component.css'
})
export class PrivateNodesComponent implements OnInit {
  private readonly service = inject(PrivateNodesService);
  private readonly translation = inject(TranslationService);
  readonly cambios = new FormChanges();

  readonly lineas = signal<string[]>([]);
  readonly ruta = signal('');
  readonly escribible = signal(false);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly aviso = signal('');

  /** Carga al iniciar el componente las líneas y permisos del archivo de nodos. */
  ngOnInit(): void {
    this.cargar();
  }

  /** Lee el archivo y actualiza su ruta, permisos, contenido y estado de error. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.obtener().subscribe({
      next: respuesta => {
        this.cargando.set(false);
        this.ruta.set(respuesta.path);
        this.escribible.set(respuesta.writable);
        this.lineas.set(respuesta.lines);
        this.cambios.reset(this.lineas());
      },
      error: (error: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(error.error?.message ?? this.translation.translate('NO_SE_PUDO_CARGAR_ARCHIVO_NODOS_PRIVADOS'));
      }
    });
  }

  /** Añade una línea vacía para introducir otro nodo privado. */
  agregar(): void {
    this.lineas.update(lineas => [...lineas, '']);
  }

  /** Sustituye el contenido de una línea sin alterar las demás. */
  actualizar(indice: number, valor: string): void {
    this.lineas.update(lineas => lineas.map((linea, actual) => actual === indice ? valor : linea));
  }

  /** Elimina la línea indicada de la configuración en edición. */
  quitar(indice: number): void {
    this.lineas.update(lineas => lineas.filter((_, actual) => actual !== indice));
  }

  /** Guarda las líneas editadas y vuelve a cargar el archivo confirmado. */
  guardar(): void {
    if (this.guardando() || this.cargando() || !this.cambios.changed(this.lineas())) { return; }
    this.error.set('');
    this.aviso.set('');
    this.guardando.set(true);
    this.service.guardar(this.lineas()).subscribe({
      next: () => {
        this.guardando.set(false);
        this.aviso.set(this.translation.translate('ARCHIVO_NODOS_PRIVADOS_GUARDADO'));
        this.cargar();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.error.set(error.error?.message ?? this.translation.translate('NO_SE_PUDO_GUARDAR_ARCHIVO_NODOS_PRIVADOS'));
      }
    });
  }
}
