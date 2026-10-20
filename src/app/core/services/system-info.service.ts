import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import { SystemInfo } from '../models/system-info.model';

export type SystemAction = 'asterisk_restart' | 'linux_reboot' | 'linux_poweroff';

export interface SystemControlResponse {
  status: string;
  message: string;
  data: {
    action: SystemAction;
    recovered: boolean;
    scheduled: boolean;
    details: string[];
    warnings: string;
  };
}

@Injectable({ providedIn: 'root' })
export class SystemInfoService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/system-info`;

  /** Consulta las métricas del equipo anfitrión y de Asterisk para el diálogo administrativo. */
  obtener(): Observable<SystemInfo> {
    return this.http.get<SystemInfo>(this.url);
  }

  /** Solicita una acción administrativa fija con confirmación explícita. */
  controlar(action: SystemAction, confirmation: string): Observable<SystemControlResponse> {
    return this.http.post<SystemControlResponse>(`${environment.apiUrl}/system/control`, { action, confirmation });
  }
}
