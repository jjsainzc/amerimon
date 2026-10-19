import { Component, HostListener, effect, inject, input, output, signal } from '@angular/core';
import { timeout } from 'rxjs';
import { GeoData, GeoIpService } from '../../core/services/geoip.service';

@Component({
  selector: 'app-geoip-dialog',
  standalone: true,
  templateUrl: './geoip-dialog.component.html',
  styleUrl: './geoip-dialog.component.css'
})
export class GeoIpDialogComponent {
  readonly ip = input<string | null>(null);
  readonly closed = output<void>();
  readonly cargando = signal(false);
  readonly error = signal('');
  readonly datos = signal<GeoData | null>(null);

  private readonly geoService = inject(GeoIpService);

  constructor() {
    effect(onCleanup => {
      const ip = this.ip();
      this.cargando.set(false);
      this.error.set('');
      this.datos.set(null);
      if (!ip) { return; }

      this.cargando.set(true);
      const subscription = this.geoService.consultar(ip).pipe(timeout({ each: 15_000 })).subscribe({
        next: respuesta => {
          this.cargando.set(false);
          if (respuesta?.status === 'success' && respuesta.data) {
            this.datos.set(respuesta.data);
          } else {
            this.error.set(respuesta?.message || 'No se obtuvieron datos de geolocalización.');
          }
        },
        error: err => {
          this.cargando.set(false);
          this.error.set(err?.name === 'TimeoutError'
            ? 'El servidor de geolocalización no respondió a tiempo.'
            : err?.error?.message || 'No se pudo geolocalizar esta IP.');
        }
      });

      onCleanup(() => subscription.unsubscribe());
    });
  }

  cerrar(): void {
    this.closed.emit();
  }

  @HostListener('document:keydown.escape')
  cerrarConEscape(): void {
    if (this.ip()) { this.cerrar(); }
  }
}
