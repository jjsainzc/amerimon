import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, inject, output, signal } from '@angular/core';
import { SystemInfo } from '../../core/models/system-info.model';
import { SystemInfoService } from '../../core/services/system-info.service';

@Component({
  selector: 'app-system-info-dialog',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './system-info-dialog.component.html',
  styleUrl: './system-info-dialog.component.css',
})
export class SystemInfoDialogComponent {
  private readonly service = inject(SystemInfoService);

  readonly cerrar = output<void>();
  readonly informacion = signal<SystemInfo | null>(null);
  readonly cargando = signal(false);
  readonly error = signal('');

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
