import { HttpClient } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { Observable, tap } from 'rxjs';
import { environment } from '../environment';

interface TokenResponse {
  status: string;
  token: string;
}

const rememberedTokenKey = 'amerimon.admin.token';
const rememberPreferenceKey = 'amerimon.admin.remember';
const rememberedUserKey = 'amerimon.admin.user';
const rememberedPasswordKey = 'amerimon.admin.password';

/** Un solo administrador; la sesión solo se persiste si el usuario lo solicita. */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly tokenSignal = signal<string | null>(localStorage.getItem(rememberedTokenKey));

  /** Indica si hay un token cargado; su validez efectiva se comprueba en el backend. */
  readonly isAuthenticated = computed(() => !!this.tokenSignal());
  /** Habilita la UI administrativa cuando el payload del token contiene el rol A. */
  readonly esAdmin = computed(() => (this.leerClaim<string>('role') ?? '').toUpperCase() === 'A');

  /** true si el admin ya definió su contraseña. */
  configurada(): Observable<{ configured: boolean }> {
    return this.http.get<{ configured: boolean }>(`${environment.apiUrl}/auth/status`);
  }

  /** Define la primera contraseña (solo si no existe) y abre sesión. */
  configurar(user: string, password: string, recordar = false): Observable<TokenResponse> {
    return this.http
      .post<TokenResponse>(`${environment.apiUrl}/auth/setup`, { user, new: password })
      .pipe(tap((resp) => this.iniciarSesion(resp.token, recordar, user, password)));
  }

  /** Autentica al administrador y conserva el token si se solicita recordar la sesión. */
  login(user: string, password: string, recordar = false): Observable<TokenResponse> {
    return this.http
      .post<TokenResponse>(`${environment.apiUrl}/auth/login`, { user, password })
      .pipe(tap((resp) => this.iniciarSesion(resp.token, recordar, user, password)));
  }

  /** Solicita cambiar la contraseña verificando primero la contraseña actual. */
  cambiarPassword(current: string, nueva: string): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${environment.apiUrl}/auth/password`, { current, new: nueva })
      .pipe(tap(() => {
        if (this.recordarSesion() && this.usuarioRecordado()) {
          localStorage.setItem(rememberedPasswordKey, nueva);
        }
      }));
  }

  /** Cierra el acceso sin borrar las credenciales que se eligió recordar. */
  logout(): void {
    localStorage.removeItem(rememberedTokenKey);
    this.tokenSignal.set(null);
  }

  /** Actualiza la sesión en memoria y sincroniza su persistencia según la opción elegida. */
  private iniciarSesion(token: string, recordar: boolean, user: string, password: string): void {
    if (recordar) {
      localStorage.setItem(rememberedTokenKey, token);
      localStorage.setItem(rememberPreferenceKey, 'true');
      localStorage.setItem(rememberedUserKey, user);
      localStorage.setItem(rememberedPasswordKey, password);
    } else {
      localStorage.removeItem(rememberedTokenKey);
      localStorage.removeItem(rememberPreferenceKey);
      localStorage.removeItem(rememberedUserKey);
      localStorage.removeItem(rememberedPasswordKey);
    }
    this.tokenSignal.set(token);
  }

  /** Restaura la casilla, incluyendo sesiones recordadas antes de existir la preferencia separada. */
  recordarSesion(): boolean {
    return localStorage.getItem(rememberPreferenceKey) === 'true' || localStorage.getItem(rememberedTokenKey) !== null;
  }

  /** Recupera el nombre de usuario recordado. */
  usuarioRecordado(): string {
    return this.recordarSesion() ? localStorage.getItem(rememberedUserKey) ?? '' : '';
  }

  passwordRecordada(): string {
    return this.recordarSesion() ? localStorage.getItem(rememberedPasswordKey) ?? '' : '';
  }

  /** Devuelve el token actual para autenticar peticiones HTTP y conexiones de audio. */
  getToken(): string | null {
    return this.tokenSignal();
  }

  /** Lee el identificador de usuario del token; devuelve null si no está disponible. */
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
