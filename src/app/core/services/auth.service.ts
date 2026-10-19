import { HttpClient } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { Observable, tap } from 'rxjs';
import { environment } from '../environment';

interface TokenResponse {
  status: string;
  token: string;
}

/** Un solo administrador. El token vive solo en memoria: al recargar se vuelve al modo público. */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly tokenSignal = signal<string | null>(null);

  readonly isAuthenticated = computed(() => !!this.tokenSignal());
  readonly esAdmin = computed(() => (this.leerClaim<string>('role') ?? '').toUpperCase() === 'A');

  /** true si el admin ya definió su contraseña. */
  configurada(): Observable<{ configured: boolean }> {
    return this.http.get<{ configured: boolean }>(`${environment.apiUrl}/auth/status`);
  }

  /** Define la primera contraseña (solo si no existe) y abre sesión. */
  configurar(user: string, password: string): Observable<TokenResponse> {
    return this.http
      .post<TokenResponse>(`${environment.apiUrl}/auth/setup`, { user, new: password })
      .pipe(tap((resp) => this.tokenSignal.set(resp.token)));
  }

  login(user: string, password: string): Observable<TokenResponse> {
    return this.http
      .post<TokenResponse>(`${environment.apiUrl}/auth/login`, { user, password })
      .pipe(tap((resp) => this.tokenSignal.set(resp.token)));
  }

  cambiarPassword(current: string, nueva: string): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${environment.apiUrl}/auth/password`, { current, new: nueva });
  }

  logout(): void {
    this.tokenSignal.set(null);
  }

  getToken(): string | null {
    return this.tokenSignal();
  }

  getCurrentUserId(): number | null {
    return this.leerClaim<number>('userid');
  }

  // Solo lee el payload para la UI; la firma la valida el backend.
  private leerClaim<T>(clave: string): T | null {
    const partes = this.tokenSignal()?.split('.');
    if (!partes || partes.length < 2) { return null; }
    try {
      const base64 = partes[1].replace(/-/g, '+').replace(/_/g, '/');
      const json = decodeURIComponent(
        atob(base64).split('').map(c => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2)).join('')
      );
      return (JSON.parse(json)?.[clave] ?? null) as T | null;
    } catch {
      return null;
    }
  }
}
