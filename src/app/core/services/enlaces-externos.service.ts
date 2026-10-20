import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';
import { EnlaceExterno, EnlaceExternoEntrada } from '../models/enlace-externo.model';

@Injectable({ providedIn: 'root' })
export class EnlacesExternosService {
  private readonly http = inject(HttpClient);
  private readonly publicUrl = `${environment.apiUrl}/enlaces-externos`;
  private readonly adminUrl = `${environment.apiUrl}/admin/enlaces-externos`;

  /** Obtiene los enlaces activos publicados para todos los visitantes. */
  listarActivos(): Observable<EnlaceExterno[]> {
    return this.http.get<EnlaceExterno[]>(this.publicUrl);
  }

  /** Consulta el catálogo administrativo, incluidos los enlaces inactivos. */
  listarAdmin(): Observable<EnlaceExterno[]> {
    return this.http.get<EnlaceExterno[]>(this.adminUrl);
  }

  /** Publica un nuevo enlace con su descripción, destino y estado de activación. */
  crear(enlace: EnlaceExternoEntrada): Observable<EnlaceExterno> {
    return this.http.post<EnlaceExterno>(this.adminUrl, enlace);
  }

  /** Sustituye los datos del enlace identificado y devuelve el registro actualizado. */
  actualizar(id: number, enlace: EnlaceExternoEntrada): Observable<EnlaceExterno> {
    return this.http.put<EnlaceExterno>(`${this.adminUrl}/${id}`, enlace);
  }

  /** Elimina definitivamente del catálogo el enlace indicado. */
  borrar(id: number): Observable<void> {
    return this.http.delete<void>(`${this.adminUrl}/${id}`);
  }
}
