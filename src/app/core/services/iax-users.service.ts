import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IAXUser } from '../models/iax-user.model';
import { environment } from '../environment';

@Injectable({
  providedIn: 'root'
})
export class IaxUsersService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/iax-users`;

  listar(): Observable<{[key: string]: IAXUser}> {
    return this.http.get<{[key: string]: IAXUser}>(this.apiUrl);
  }

  guardar(user: IAXUser): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(this.apiUrl, user);
  }

  eliminar(name: string): Observable<{ message: string }> {
    return this.http.delete<{ message: string }>(`${this.apiUrl}/${encodeURIComponent(name)}`);
  }
}
