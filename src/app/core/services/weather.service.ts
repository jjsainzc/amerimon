import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../environment';

@Injectable({
  providedIn: 'root'
})
export class WeatherService {
  // Ajusta la URL base según corresponda a tu entorno (ej. http://localhost:8080/weather)
  private apiUrl = `${environment.apiUrl}/weather`;

  constructor(private http: HttpClient) {}

  /** Sin `location`, el servidor usa la ubicación configurada en la cabecera o la IP del visitante. */
  getWeather(location: string = '', mode: string = 'F'): Observable<string> {
    let params = new HttpParams().set('mode', mode);
    if (location.trim()) {
      params = params.set('location', location.trim());
    }

    // Como la API devuelve texto plano (string), usamos { responseType: 'text' }
    return this.http.get(this.apiUrl, { params, responseType: 'text' });
  }
}
