import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { PJSIPExtension } from '../models/pjsip-extension.model';
import { environment } from '../environment';

@Injectable({
  providedIn: 'root'
})
export class PjsipUsersService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/pjsip-users`;

  listar(): Observable<PJSIPExtension[]> {
    return this.http.get<PJSIPExtension[]>(this.apiUrl);
  }

  guardar(ext: PJSIPExtension): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(this.apiUrl, ext);
  }

  eliminar(id: string): Observable<{ message: string }> {
    return this.http.delete<{ message: string }>(`${this.apiUrl}/${encodeURIComponent(id)}`);
  }
}
