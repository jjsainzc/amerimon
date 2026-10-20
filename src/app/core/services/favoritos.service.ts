import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import { Favorito, FavoritoEntrada } from '../models/favorito.model';

@Injectable({ providedIn: 'root' })
export class FavoritosService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/favoritos`;

  /** Obtiene los nodos favoritos disponibles para el administrador. */
  listar(): Observable<Favorito[]> {
    return this.http.get<Favorito[]>(this.url);
  }

  /** Busca en AstDB la descripción del nodo para completar su ficha de favorito. */
  descripcionAstdb(numero: string): Observable<{ descripcion: string }> {
    return this.http.get<{ descripcion: string }>(`${this.url}/astdb/${encodeURIComponent(numero)}`);
  }

  /** Guarda un nuevo favorito y devuelve el registro con su identificador. */
  crear(favorito: FavoritoEntrada): Observable<Favorito> {
    return this.http.post<Favorito>(this.url, favorito);
  }

  /** Guarda los cambios del favorito identificado y devuelve sus datos actualizados. */
  actualizar(id: number, favorito: FavoritoEntrada): Observable<Favorito> {
    return this.http.put<Favorito>(`${this.url}/${id}`, favorito);
  }

  /** Elimina el favorito indicado sin desconectar el nodo de Asterisk. */
  borrar(id: number): Observable<unknown> {
    return this.http.delete(`${this.url}/${id}`);
  }
}
