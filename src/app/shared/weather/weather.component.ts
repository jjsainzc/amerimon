import { Component, computed, Input, OnDestroy, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { FormsModule } from '@angular/forms';
import { Subscription } from 'rxjs';
import { WeatherService } from '../../core/services/weather.service';

interface CoordenadaDMS {
  grados: number;
  minutos: number;
  segundos: number;
  hemisferio: 'N' | 'S' | 'E' | 'O';
}

@Component({
  selector: 'app-weather',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './weather.component.html',
  styleUrls: ['./weather.component.css']
})
export class WeatherComponent implements OnInit, OnDestroy {
  readonly weatherReport = signal('');
  readonly loading = signal(false);
  readonly errorMessage = signal('');
  readonly consultedLocation = signal('');
  private readonly mapLocation = signal('');
  readonly locationMapUrl = computed(() => this.mapLocation()
    ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(this.mapLocation())}`
    : '');
  readonly configuredLocation = signal<string | undefined>(undefined);
  readonly locationError = signal('');
  readonly locating = signal(false);
  locationDraft = '';

  private temporaryLocation: string | undefined;
  private lastWeatherLocation: string | undefined;
  private weatherRequest: Subscription | undefined;
  private requestVersion = 0;
  private retryTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshTimer: ReturnType<typeof setInterval> | undefined;
  private static readonly retryDelayMs = 30_000;
  private static readonly refreshIntervalMs = 300_000;

  constructor(private weatherService: WeatherService) {}

  ngOnInit(): void {
    this.refreshTimer = setInterval(() => {
      if (!this.loading()) {
        this.refreshWeather();
      }
    }, WeatherComponent.refreshIntervalMs);
  }

  ngOnDestroy(): void {
    if (this.refreshTimer !== undefined) {
      clearInterval(this.refreshTimer);
    }
    this.cancelPendingRequest();
  }

  /**
   * Valor predeterminado del administrador, independiente de la selección temporal del visitante.
   * `undefined` significa que los ajustes aún no se han cargado.
   */
  @Input() set ubicacionConfigurada(value: string | null | undefined) {
    const next = value == null ? undefined : value.trim();
    if (next === undefined || next === this.configuredLocation()) {
      return;
    }
    this.configuredLocation.set(next);
    if (this.temporaryLocation === undefined && !this.locating()) {
      this.loadWeather();
    }
  }

  loadWeather(): void {
    const location = this.temporaryLocation ?? this.configuredLocation();
    if (location) {
      this.fetchWeather(location);
    } else {
      this.getWeatherForBrowserLocation();
    }
  }

  refreshWeather(): void {
    if (this.lastWeatherLocation !== undefined) {
      this.fetchWeather(this.lastWeatherLocation);
    } else {
      this.loadWeather();
    }
  }

  getWeatherForBrowserLocation(): void {
    this.cancelPendingRequest();
    const version = this.requestVersion;
    // Sin geolocalización (o si el visitante la rechaza) el servidor la estima por la IP.
    if (!navigator.geolocation) {
      this.fetchWeather('');
      return;
    }

    this.loading.set(true);
    this.errorMessage.set('');

    navigator.geolocation.getCurrentPosition(
      (position) => {
        if (version !== this.requestVersion) { return; }
        const lat = position.coords.latitude;
        const lon = position.coords.longitude;
        const locationQuery = `${lat},${lon}`;

        this.fetchWeather(locationQuery);
      },
      () => {
        if (version === this.requestVersion) { this.fetchWeather(''); }
      },
      { enableHighAccuracy: false, maximumAge: 300_000, timeout: 10_000 },
    );
  }

  openLocationDialog(dialog: HTMLDialogElement): void {
    this.locationDraft = this.temporaryLocation ?? this.configuredLocation() ?? '';
    this.locationError.set('');
    dialog.showModal();
  }

  closeLocationDialog(): void {
    if (this.locating()) {
      this.cancelPendingRequest();
      this.locating.set(false);
      this.loading.set(false);
    }
  }

  useEnteredLocation(dialog: HTMLDialogElement): void {
    const location = this.locationDraft.trim();
    if (!location) {
      this.locationError.set('Introduce una ubicación.');
      return;
    }
    this.temporaryLocation = location;
    this.locating.set(false);
    dialog.close();
    this.fetchWeather(location);
  }

  useConfiguredLocation(dialog: HTMLDialogElement): void {
    const location = this.configuredLocation();
    if (!location) {
      this.locationError.set('No hay una ubicación configurada por el administrador.');
      return;
    }
    this.temporaryLocation = undefined;
    this.locating.set(false);
    dialog.close();
    this.fetchWeather(location);
  }

  useBrowserLocation(dialog: HTMLDialogElement): void {
    this.locationError.set('');
    if (!navigator.geolocation) {
      this.locationError.set('Este navegador no permite geolocalización. Usa HTTPS o introduce una ubicación.');
      return;
    }
    this.cancelPendingRequest();
    const version = this.requestVersion;
    this.locating.set(true);
    this.loading.set(true);
    navigator.geolocation.getCurrentPosition(
      (position) => {
        if (version !== this.requestVersion) { return; }
        this.temporaryLocation = `${position.coords.latitude},${position.coords.longitude}`;
        this.locating.set(false);
        dialog.close();
        this.fetchWeather(this.temporaryLocation);
      },
      (error) => {
        if (version !== this.requestVersion) { return; }
        this.locating.set(false);
        this.loading.set(false);
        this.locationError.set(error.code === 1
          ? 'Permiso de ubicación denegado. Habilítalo en el navegador o introduce una ubicación.'
          : 'No se pudo obtener tu ubicación. Inténtalo de nuevo o introduce una ubicación.');
      },
      { enableHighAccuracy: true, maximumAge: 0, timeout: 10_000 },
    );
  }

  fetchWeather(location: string): void {
    this.cancelPendingRequest();
    const trimmedLocation = location.trim();
    this.lastWeatherLocation = trimmedLocation;
    this.mapLocation.set(trimmedLocation);

    // Formatear location para el servicio si es lat,lon
    let apiLocation = trimmedLocation;

    // Si la ubicación contiene una coma, se asume que es latitud,longitud
    // Asignar el valor formateado en Grados, Minutos, Segundos a consultedLocation
    // Si no es así, se usa la ubicación tal cual
    this.consultedLocation.set(this.formatearCoordenadasDMS(trimmedLocation));

    this.loading.set(true);
    this.errorMessage.set('');
    this.weatherRequest = this.weatherService.getWeather(apiLocation, 'F').subscribe({
      next: (data) => {
        this.weatherReport.set(data.trim());
        this.loading.set(false);
      },
      error: (error: HttpErrorResponse) => {
        this.errorMessage.set(this.getErrorMessage(error));
        this.loading.set(false);
        console.error(error);
        this.retryTimer = setTimeout(() => {
          this.retryTimer = undefined;
          this.fetchWeather(trimmedLocation);
        }, WeatherComponent.retryDelayMs);
      }
    });
  }

  private convertirAFormatoDMS(decimalStr: string, esLatitud: boolean): CoordenadaDMS {
    const numStr = decimalStr.replace(',', '.');
    const val = parseFloat(numStr.trim());

    let hemisferio: 'N' | 'S' | 'E' | 'O';
    if (esLatitud) {
      hemisferio = val >= 0 ? 'N' : 'S';
    } else {
      hemisferio = val >= 0 ? 'E' : 'O';
    }

    const valAbs = Math.abs(val);
    const grados = Math.floor(valAbs);
    const minutosFloat = (valAbs - grados) * 60;
    const minutos = Math.floor(minutosFloat);
    const segundos = Number(((minutosFloat - minutos) * 60).toFixed(2));

    return { grados, minutos, segundos, hemisferio };
  }

  private formatearCoordenadasDMS(locationStr: string): string {
    if (!locationStr) {
      return 'ubicación automática por IP';
    }

    const partes = locationStr.split(',');
    // Verificamos si son coordenadas numéricas (lat, lon)
    if (partes.length === 2 && !isNaN(Number(partes[0])) && !isNaN(Number(partes[1]))) {
      try {
        const latDMS = this.convertirAFormatoDMS(partes[0], true);
        const lonDMS = this.convertirAFormatoDMS(partes[1], false);
        return `${latDMS.grados}° ${latDMS.minutos}' ${latDMS.segundos}" ${latDMS.hemisferio}, ${lonDMS.grados}° ${lonDMS.minutos}' ${lonDMS.segundos}" ${lonDMS.hemisferio}`;
      } catch {
        return locationStr;
      }
    }

    // Si el usuario introdujo un nombre de lugar (ej. "Quito"), se retorna tal cual
    return locationStr;
  }

  private cancelPendingRequest(): void {
    this.requestVersion++;
    this.clearRetryTimer();
    this.weatherRequest?.unsubscribe();
  }

  private clearRetryTimer(): void {
    if (this.retryTimer !== undefined) {
      clearTimeout(this.retryTimer);
      this.retryTimer = undefined;
    }
  }

  private getErrorMessage(error: HttpErrorResponse): string {
    if (typeof error.error === 'string') {
      try {
        const payload: unknown = JSON.parse(error.error);
        if (payload && typeof payload === 'object' && 'message' in payload
          && typeof payload.message === 'string') {
          return payload.message;
        }
      } catch {
        return error.error;
      }
      return error.error;
    }
    return error.error?.message ?? 'Error al consultar la API de Go.';
  }
}
