import { Component, Input, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { WeatherService } from '../../core/services/weather.service';

@Component({
  selector: 'app-weather',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './weather.component.html',
  styleUrls: ['./weather.component.css']
})
export class WeatherComponent {
  readonly weatherReport = signal('');
  readonly loading = signal(false);
  readonly errorMessage = signal('');

  private configuredLocation: string | undefined;

  constructor(private weatherService: WeatherService) {}

  /**
   * Ubicación configurada en la cabecera. Si tiene valor se usa siempre; si está vacía se pide la
   * ubicación al navegador. `undefined` significa que los ajustes aún no se han cargado.
   */
  @Input() set ubicacionConfigurada(value: string | null | undefined) {
    const next = value == null ? undefined : value.trim();
    if (next === undefined || next === this.configuredLocation) {
      return;
    }
    this.configuredLocation = next;
    this.loadWeather();
  }

  loadWeather(): void {
    if (this.configuredLocation) {
      this.fetchWeather(this.configuredLocation);
    } else {
      this.getWeatherForBrowserLocation();
    }
  }

  getWeatherForBrowserLocation(): void {
    // Sin geolocalización (o si el visitante la rechaza) el servidor la estima por la IP.
    if (!navigator.geolocation) {
      this.fetchWeather('');
      return;
    }

    this.loading.set(true);
    this.errorMessage.set('');

    navigator.geolocation.getCurrentPosition(
      (position) => {
        const lat = position.coords.latitude;
        const lon = position.coords.longitude;
        const locationQuery = `${lat},${lon}`;

        this.fetchWeather(locationQuery);
      },
      () => this.fetchWeather(''),
      { enableHighAccuracy: false, maximumAge: 300_000, timeout: 10_000 },
    );
  }

  fetchWeather(location: string): void {
    this.loading.set(true);
    this.errorMessage.set('');
    this.weatherService.getWeather(location, 'F').subscribe({
      next: (data) => {
        this.weatherReport.set(data.trim());
        this.loading.set(false);
      },
      error: (err) => {
        const message = typeof err.error === 'string'
          ? err.error
          : err.error?.message ?? 'Error al consultar la API de Go.';
        this.errorMessage.set(message);
        this.loading.set(false);
        console.error(err);
      }
    });
  }
}
