import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export interface PrivateNodesConfig {
  status: string;
  path: string;
  writable: boolean;
  lines: string[];
}

export interface PrivateRPTNode {
  id: string;
  rxchannel: string;
  settings: Record<string, string>;
  show_as_server: boolean;
}

export interface PrivateRPTNodesConfig {
  status: string;
  path: string;
  nodes: PrivateRPTNode[];
  template_settings: Record<string, string>;
  template_descriptions: Record<string, string>;
}

@Injectable({ providedIn: 'root' })
export class PrivateNodesService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/private-nodes`;

  /** Consulta las líneas de nodos privados, su ruta efectiva y los permisos de escritura. */
  obtener(): Observable<PrivateNodesConfig> {
    return this.http.get<PrivateNodesConfig>(this.url);
  }

  /** Envía el contenido completo para validarlo y guardar el archivo de nodos privados. */
  guardar(lines: string[]): Observable<{ status: string; path: string }> {
    return this.http.put<{ status: string; path: string }>(this.url, { lines });
  }

  /** Consulta los nodos de cuatro dígitos configurados en rpt.conf. */
  obtenerRPT(): Observable<PrivateRPTNodesConfig> {
    return this.http.get<PrivateRPTNodesConfig>(`${this.url}/rpt`);
  }

  /** Guarda el conjunto administrado de nodos privados sin reemplazar ajustes ajenos. */
  guardarRPT(nodes: PrivateRPTNode[]): Observable<{ status: string; path: string }> {
    return this.http.put<{ status: string; path: string }>(`${this.url}/rpt`, { nodes });
  }
}
