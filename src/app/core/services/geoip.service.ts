import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../environment';

/** Datos de geolocalización devueltos por el backend. */
export interface GeoData {
  ip: string;
  country: string;
  country_code: string;
  city: string;
  latitude: number;
  longitude: number;
  time_zone: string;
}

/** Respuesta del endpoint /geoip. */
export interface GeoIpResponse {
  status: string;
  message?: string;
  hostname?: string;
  data?: GeoData;
}

/**
 * Servicio de geolocalización por IP o hostname.
 * Endpoint de administrador: GET {apiUrl}/geoip/:ip_or_host
 */
@Injectable({ providedIn: 'root' })
export class GeoIpService {
  private readonly http = inject(HttpClient);
  private readonly API_URL = `${environment.apiUrl}/geoip`;

  /** Consulta la geolocalización de una IP o hostname. */
  consultar(ipOrHost: string): Observable<GeoIpResponse> {
    return this.http.get<GeoIpResponse>(`${this.API_URL}/${encodeURIComponent(ipOrHost.trim())}`);
  }
}
