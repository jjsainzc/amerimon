import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import { SystemInfo } from '../models/system-info.model';

@Injectable({ providedIn: 'root' })
export class SystemInfoService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/system-info`;

  /** Consulta las métricas del equipo anfitrión y de Asterisk para el diálogo administrativo. */
  obtener(): Observable<SystemInfo> {
    return this.http.get<SystemInfo>(this.url);
  }
}
