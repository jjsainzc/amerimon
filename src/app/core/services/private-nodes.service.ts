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
}
