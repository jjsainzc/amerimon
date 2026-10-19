import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { environment } from '../environment';
import { ListenLiveSession, ListenLiveSessionResponse, ListenLiveSessionsResponse } from '../models/listenlive-session.model';

@Injectable({ providedIn: 'root' })
export class ListenLiveSessionsService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/listen-live/sessions`;

  async listar(): Promise<ListenLiveSession[]> {
    const response = await firstValueFrom(this.http.get<ListenLiveSessionsResponse>(this.url));
    return response.data;
  }

  async obtener(id: number): Promise<ListenLiveSession> {
    const response = await firstValueFrom(this.http.get<ListenLiveSessionResponse>(`${this.url}/${id}`));
    return response.data;
  }

  async crear(): Promise<ListenLiveSession> {
    const response = await firstValueFrom(this.http.post<ListenLiveSessionResponse>(this.url, {}));
    return response.data;
  }

  async desconectar(id: number): Promise<void> {
    await firstValueFrom(
      this.http.post(`${environment.apiUrl}/listen-live/stop`, { listenliveid: id })
    );
  }

  async cerrar(id: number): Promise<ListenLiveSession> {
    const response = await firstValueFrom(
      this.http.patch<ListenLiveSessionResponse>(`${this.url}/${id}`, { fecha_fin: new Date().toISOString() })
    );
    return response.data;
  }

  async borrar(id: number): Promise<void> {
    await firstValueFrom(this.http.delete(`${this.url}/${id}`));
  }
}
