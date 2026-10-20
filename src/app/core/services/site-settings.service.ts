import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export interface SiteSettings {
  title: string;
  subtitle: string;
  asterisk_version: string;
  app_version?: string;
}

@Injectable({ providedIn: 'root' })
export class SiteSettingsService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/site-settings`;

  /** Consulta la cabecera pública y las versiones de Asterisk y Amerimon. */
  obtener(): Observable<SiteSettings> {
    return this.http.get<SiteSettings>(this.url);
  }

  /** Guarda la cabecera personalizada y devuelve la configuración efectiva del servidor. */
  guardar(title: string, subtitle: string): Observable<SiteSettings> {
    return this.http.put<SiteSettings>(this.url, { title, subtitle });
  }
}
