import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { environment } from '../environment';
import { ListenLiveSession, ListenLiveSessionResponse, ListenLiveSessionsResponse } from '../models/listenlive-session.model';

@Injectable({ providedIn: 'root' })
export class ListenLiveSessionsService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/listen-live/sessions`;

  /** Recupera los registros de escucha; propaga los errores HTTP al componente. */
  async listar(): Promise<ListenLiveSession[]> {
    const response = await firstValueFrom(this.http.get<ListenLiveSessionsResponse>(this.url));
    return response.data;
  }

  /** Consulta los datos de una sesión de escucha concreta. */
  async obtener(id: number): Promise<ListenLiveSession> {
    const response = await firstValueFrom(this.http.get<ListenLiveSessionResponse>(`${this.url}/${id}`));
    return response.data;
  }

  /** Abre un registro de sesión antes de iniciar la transmisión de audio. */
  async crear(): Promise<ListenLiveSession> {
    const response = await firstValueFrom(this.http.post<ListenLiveSessionResponse>(this.url, {}));
    return response.data;
  }

  /** Solicita al backend detener el audio asociado al registro de escucha indicado. */
  async desconectar(id: number): Promise<void> {
    await firstValueFrom(
      this.http.post(`${environment.apiUrl}/listen-live/stop`, { listenliveid: id })
    );
  }

  /** Marca el fin de la sesión con la fecha UTC del navegador y devuelve el registro. */
  async cerrar(id: number): Promise<ListenLiveSession> {
    const response = await firstValueFrom(
      this.http.patch<ListenLiveSessionResponse>(`${this.url}/${id}`, { fecha_fin: new Date().toISOString() })
    );
    return response.data;
  }

  /** Elimina el registro de sesión mediante la API administrativa. */
  async borrar(id: number): Promise<void> {
    await firstValueFrom(this.http.delete(`${this.url}/${id}`));
  }
}
