import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, inject, output, signal } from '@angular/core';
import { CopyValueDirective } from '../copy-value/copy-value.directive';
import { SystemInfo } from '../../core/models/system-info.model';
import { SystemAction, SystemControlResponse, SystemInfoService } from '../../core/services/system-info.service';

@Component({
  selector: 'app-system-info-dialog',
  standalone: true,
  imports: [CommonModule, CopyValueDirective],
  templateUrl: './system-info-dialog.component.html',
  styleUrl: './system-info-dialog.component.css',
})
export class SystemInfoDialogComponent {
  private readonly service = inject(SystemInfoService);

  readonly cerrar = output<void>();
  readonly informacion = signal<SystemInfo | null>(null);
  readonly cargando = signal(false);
  readonly error = signal('');
  readonly accion = signal<SystemAction | null>(null);
  readonly confirmacion = signal('');
  readonly ejecutando = signal(false);
  readonly programado = signal(false);
  readonly resultado = signal<SystemControlResponse | null>(null);
  readonly errorControl = signal('');

  /** Devuelve el texto que el administrador debe escribir antes de enviar una operación destructiva. */
  textoConfirmacion(action: SystemAction | null): string {
    switch (action) {
      case 'asterisk_restart': return 'REINICIAR ASTERISK';
      case 'linux_reboot': return 'REINICIAR LINUX';
      case 'linux_poweroff': return 'APAGAR LINUX';
      default: return '';
    }
  }

  /** Abre la confirmación sin ejecutar aún ningún comando. */
  preparar(action: SystemAction): void {
    if (this.ejecutando() || this.programado()) { return; }
    this.accion.set(action);
    this.confirmacion.set('');
    this.resultado.set(null);
    this.errorControl.set('');
  }

  /** Ejecuta solo una acción confirmada y conserva los diagnósticos tanto de éxito como de fallo. */
  ejecutar(): void {
    const action = this.accion();
    if (!action || this.ejecutando() || this.programado() || this.confirmacion() !== this.textoConfirmacion(action)) { return; }
    this.ejecutando.set(true);
    this.errorControl.set('');
    this.service.controlar(action, this.confirmacion()).subscribe({
      next: result => {
        this.resultado.set(result);
        this.programado.set(result.data.scheduled);
        this.ejecutando.set(false);
        this.accion.set(null);
        this.confirmacion.set('');
      },
      error: (err: HttpErrorResponse) => {
        this.ejecutando.set(false);
        this.errorControl.set(err.error?.message ?? 'No se recibió confirmación del servidor. Comprueba su estado antes de repetir la acción.');
        if (err.error?.data) {
          this.resultado.set(err.error);
        }
      },
    });
  }

  /** Solicita las métricas iniciales al abrir el diálogo. */
  constructor() {
    this.cargar();
  }

  /** Actualiza las métricas del sistema y expone cualquier error de consulta en el diálogo. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.obtener().subscribe({
      next: info => {
        this.informacion.set(info);
        this.cargando.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(err.error?.message ?? 'No se pudo consultar el sistema.');
      },
    });
  }

  /** Convierte una lectura textual en un porcentaje entre 0 y 100; usa cero si no es numérica. */
  porcentaje(valor: string): number {
    const parsed = Number.parseFloat(valor);
    return Number.isFinite(parsed) ? Math.min(100, Math.max(0, parsed)) : 0;
  }

  /** Emite el cierre con Escape y evita que la tecla alcance otros diálogos. */
  cerrarConEscape(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.stopPropagation();
      this.cerrar.emit();
    }
  }
}
