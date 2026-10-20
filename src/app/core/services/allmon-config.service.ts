import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export interface AllmonNodoEntrada {
  node: string;
  host: string;
  port: string;
  user: string;
  passwd: string;
}

export interface AllmonConfig {
  status: string;
  path: string;
  writable: boolean;
  nodes: { node: string; host: string; port: string; user: string; passwd: string; passwd_set: boolean }[];
}

@Injectable({ providedIn: 'root' })
export class AllmonConfigService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/allmon/config`;

  /** Consulta los nodos de Allmon, su archivo de origen y los permisos de escritura. */
  obtener(): Observable<AllmonConfig> {
    return this.http.get<AllmonConfig>(this.url);
  }

  /** Envía la lista completa de nodos para guardar la configuración de Allmon. */
  guardar(nodes: AllmonNodoEntrada[]): Observable<{ status: string; path: string }> {
    return this.http.put<{ status: string; path: string }>(this.url, { nodes });
  }
}
