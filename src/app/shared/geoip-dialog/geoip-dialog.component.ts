import { Component, HostListener, effect, inject, input, output, signal } from '@angular/core';
import { timeout } from 'rxjs';
import { CopyValueDirective } from '../copy-value/copy-value.directive';
import { GeoData, GeoIpService } from '../../core/services/geoip.service';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { TranslationService } from '../../core/services/translation.service';

@Component({
  selector: 'app-geoip-dialog',
  standalone: true,
  imports: [CopyValueDirective, TranslatePipe],
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
  private readonly translation = inject(TranslationService);

  /** Consulta cada nueva IP y cancela la petición anterior al cambiarla o destruir el diálogo. */
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
            this.error.set(respuesta?.message || this.translation.translate('NO_SE_OBTUVIERON_DATOS_GEOLOCALIZACION'));
          }
        },
        error: err => {
          this.cargando.set(false);
          this.error.set(err?.name === 'TimeoutError'
            ? this.translation.translate('SERVIDOR_GEOLOCALIZACION_NO_RESPONDIO')
            : err?.error?.message || this.translation.translate('NO_SE_PUDO_GEOLOCALIZAR_IP'));
        }
      });

      onCleanup(() => subscription.unsubscribe());
    });
  }

  /** Notifica al componente padre que debe cerrar el diálogo. */
  cerrar(): void {
    this.closed.emit();
  }

  /** Cierra con Escape únicamente si hay una IP seleccionada. */
  @HostListener('document:keydown.escape')
  cerrarConEscape(): void {
    if (this.ip()) { this.cerrar(); }
  }
}
