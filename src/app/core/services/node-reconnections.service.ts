import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export type ReconnectionAction = 'connect' | 'monitor' | 'localmonitor';

export interface NodeReconnectionInput {
  source_node: string;
  node: string;
  enabled: boolean;
  interval_seconds: number;
  action: ReconnectionAction;
  permanent: boolean;
}

export interface NodeReconnection extends NodeReconnectionInput {
  id: number;
  next_check_at: number;
  last_check_at: number;
  last_result: string;
  last_details: string;
}

@Injectable({ providedIn: 'root' })
export class NodeReconnectionsService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/node-reconnections`;

  listar(): Observable<{ entries: NodeReconnection[] }> {
    return this.http.get<{ entries: NodeReconnection[] }>(this.url);
  }

  guardar(input: NodeReconnectionInput, id: number | null): Observable<{ status: string }> {
    return id === null
      ? this.http.post<{ status: string }>(this.url, input)
      : this.http.put<{ status: string }>(`${this.url}/${id}`, input);
  }

  eliminar(id: number): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${this.url}/${id}`);
  }
}
