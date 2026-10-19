import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import {
  AllstarlinkActivityResponse,
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

  listarNodos(): Observable<AllstarlinkResponse> {
    return this.http.get<AllstarlinkResponse>(this.apiUrl);
  }

  listarActividad(): Observable<AllstarlinkActivityResponse> {
    return this.http.get<AllstarlinkActivityResponse>(`${environment.apiUrl}/allstarlink/activity`);
  }

  listarHistorialConexiones(filtros: AllstarlinkConnectionHistoryFilters = {}): Observable<AllstarlinkConnectionHistoryResponse> {
    const params = Object.fromEntries(
      Object.entries(filtros).filter(([, value]) => value !== undefined && value !== '')
    );
    return this.http.get<AllstarlinkConnectionHistoryResponse>(
      `${environment.apiUrl}/allstarlink/connections/history`,
      { params }
    );
  }

  listarComandos(): Observable<SupermonCommandsResponse> {
    return this.http.get<SupermonCommandsResponse>(`${environment.apiUrl}/asterisk/commands`);
  }

  ejecutarComando(request: SupermonCommandRequest): Observable<SupermonCommandResponse> {
    return this.http.post<SupermonCommandResponse>(`${environment.apiUrl}/asterisk/commands`, request);
  }
}
