import { Component, input, signal } from '@angular/core';
import { GeoIpDialogComponent } from '../geoip-dialog/geoip-dialog.component';

@Component({
  selector: 'app-geoip-link',
  standalone: true,
  imports: [GeoIpDialogComponent],
  templateUrl: './geoip-link.component.html',
  styleUrl: './geoip-link.component.css'
})
export class GeoIpLinkComponent {
  readonly ip = input<string | null>(null);
  readonly label = input<string | null>(null);
  readonly title = input<string | null>(null);
  readonly abierto = signal(false);

  cerrar(): void {
    this.abierto.set(false);
  }
}
