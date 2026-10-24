import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import {
  AllstarlinkActivityResponse,
  ConnectionAudio,
  AllstarlinkConnectionHistoryFilters,
  AllstarlinkConnectionHistoryResponse,
  AllstarlinkResponse,
  SupermonCommandRequest,
  SupermonCommandResponse,
  SupermonCommandsResponse
} from '../models/allstarlink.model';

@Injectable({ providedIn: 'root' })
export class AllstarlinkService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/allstarlink/nodes`;

  /** Consulta las conexiones de un servidor o el listado completo. */
  listarNodos(localNode?: string): Observable<AllstarlinkResponse> {
    return this.http.get<AllstarlinkResponse>(this.apiUrl, {
      params: localNode ? { local_node: localNode } : {}
    });
  }

  /** Descubre los servidores configurados sin abrir conexiones AMI. */
  listarServidores(): Observable<AllstarlinkResponse> {
    return this.http.get<AllstarlinkResponse>(this.apiUrl, { params: { discovery: 'true' } });
  }

  /** Obtiene la última actividad registrada para cada pareja de nodos local y remoto. */
  listarActividad(): Observable<AllstarlinkActivityResponse> {
    return this.http.get<AllstarlinkActivityResponse>(`${environment.apiUrl}/allstarlink/activity`);
  }

  /** Consulta el historial enviando solo los filtros con valor definido y no vacío. */
  listarHistorialConexiones(filtros: AllstarlinkConnectionHistoryFilters = {}): Observable<AllstarlinkConnectionHistoryResponse> {
    const params = Object.fromEntries(
      Object.entries(filtros).filter(([, value]) => value !== undefined && value !== '')
    );
    return this.http.get<AllstarlinkConnectionHistoryResponse>(
      `${environment.apiUrl}/allstarlink/connections/history`,
      { params }
    );
  }

  /** Obtiene las operaciones Asterisk disponibles y sus requisitos de ejecución. */
  listarComandos(): Observable<SupermonCommandsResponse> {
    return this.http.get<SupermonCommandsResponse>(`${environment.apiUrl}/asterisk/commands`);
  }

  /** Solicita una operación real sobre Asterisk y devuelve la salida del comando. */
  ejecutarComando(request: SupermonCommandRequest): Observable<SupermonCommandResponse> {
    return this.http.post<SupermonCommandResponse>(`${environment.apiUrl}/asterisk/commands`, request);
  }

  cambiarAudio(local: string, remoto: string, connectionId: string, muted: boolean) {
    return this.http.post<{ status: string; message?: string; data?: ConnectionAudio }>(
      `${environment.apiUrl}/asterisk/connections/audio`,
      { local_node: local, remote_node: remoto, connection_id: connectionId, muted }
    );
  }
}
