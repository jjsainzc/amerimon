import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import { Favorito, FavoritoEntrada } from '../models/favorito.model';

@Injectable({ providedIn: 'root' })
export class FavoritosService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/favoritos`;

  listar(): Observable<Favorito[]> {
    return this.http.get<Favorito[]>(this.url);
  }

  descripcionAstdb(numero: string): Observable<{ descripcion: string }> {
    return this.http.get<{ descripcion: string }>(`${this.url}/astdb/${encodeURIComponent(numero)}`);
  }

  crear(favorito: FavoritoEntrada): Observable<Favorito> {
    return this.http.post<Favorito>(this.url, favorito);
  }

  actualizar(id: number, favorito: FavoritoEntrada): Observable<Favorito> {
    return this.http.put<Favorito>(`${this.url}/${id}`, favorito);
  }

  borrar(id: number): Observable<unknown> {
    return this.http.delete(`${this.url}/${id}`);
  }
}
