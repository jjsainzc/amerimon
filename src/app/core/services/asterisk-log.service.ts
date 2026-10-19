import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export interface AsteriskOperationLog {
  id: number;
  created_at: string;
  action: string;
  local_node: string;
  remote_node: string;
  mode: string;
  permanent: boolean;
  command: string;
  result: string;
  details: string;
  duration_ms: number;
}

interface AsteriskLogsResponse {
  status: string;
  data: AsteriskOperationLog[];
}

@Injectable({ providedIn: 'root' })
export class AsteriskLogService {
  private readonly http = inject(HttpClient);

  listar(): Observable<AsteriskLogsResponse> {
    return this.http.get<AsteriskLogsResponse>(`${environment.apiUrl}/asterisk/logs`, { params: { limit: 2000 } });
  }
}
