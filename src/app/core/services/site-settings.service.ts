import { HttpClient } from '@angular/common/http';
import { Injectable, inject, signal } from '@angular/core';
import { Observable, tap } from 'rxjs';
import { environment } from '../environment';

export interface SiteSettings {
  title: string;
  subtitle: string;
  weather_location?: string;
  asterisk_version: string;
  app_version?: string;
}

@Injectable({ providedIn: 'root' })
export class SiteSettingsService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/site-settings`;
  readonly settings = signal<SiteSettings>({ title: 'Amerimon', subtitle: '', asterisk_version: '' });

  /** Consulta la cabecera pública y las versiones de Asterisk y Amerimon. */
  obtener(): Observable<SiteSettings> {
    return this.http.get<SiteSettings>(this.url).pipe(tap(settings => this.settings.set(settings)));
  }

  /** Guarda la cabecera personalizada y devuelve la configuración efectiva del servidor. */
  guardar(title: string, subtitle: string, weatherLocation = ''): Observable<SiteSettings> {
    return this.http.put<SiteSettings>(this.url, { title, subtitle, weather_location: weatherLocation })
      .pipe(tap(settings => this.settings.set(settings)));
  }
}
