import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AuthService } from './auth.service';
import { environment } from '../environment';

describe('AuthService session persistence', () => {
  let http: HttpTestingController;
  const key = 'amerimon.admin.token';

  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()]
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    http.verify();
    localStorage.clear();
  });

  it('keeps an unremembered login only in memory', () => {
    const service = TestBed.inject(AuthService);
    service.login('admin', 'password').subscribe();
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({ status: 'ok', token: 'memory-token' });
    expect(service.getToken()).toBe('memory-token');
    expect(localStorage.getItem(key)).toBeNull();
    expect(service.passwordRecordada()).toBe('');
  });

  it('persists a remembered login and removes it on logout', () => {
    const service = TestBed.inject(AuthService);
    service.login('admin', 'password', true).subscribe();
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({ status: 'ok', token: 'remembered-token' });
    expect(localStorage.getItem(key)).toBe('remembered-token');
    service.logout();
    expect(service.getToken()).toBeNull();
    expect(service.recordarSesion()).toBe(true);
    expect(service.usuarioRecordado()).toBe('admin');
    expect(service.passwordRecordada()).toBe('password');
    expect(localStorage.getItem('amerimon.admin.remember')).toBe('true');
    expect(localStorage.getItem(key)).toBeNull();
  });

  it('restores the remembered token on initialization', () => {
    localStorage.setItem(key, 'restored-token');
    expect(TestBed.inject(AuthService).getToken()).toBe('restored-token');
  });

  it('removes an old remembered login when the new login is not remembered', () => {
    localStorage.setItem(key, 'old-token');
    localStorage.setItem('amerimon.admin.remember', 'true');
    localStorage.setItem('amerimon.admin.user', 'old-admin');
    localStorage.setItem('amerimon.admin.password', 'old-password');
    const service = TestBed.inject(AuthService);
    service.login('admin', 'password', false).subscribe();
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({ status: 'ok', token: 'new-token' });
    expect(service.getToken()).toBe('new-token');
    expect(localStorage.getItem(key)).toBeNull();
    expect(service.recordarSesion()).toBe(false);
    expect(service.usuarioRecordado()).toBe('');
    expect(localStorage.getItem('amerimon.admin.user')).toBeNull();
    expect(localStorage.getItem('amerimon.admin.password')).toBeNull();
  });

  it('restores the preference and username after logout and a new service instance', () => {
    localStorage.setItem('amerimon.admin.remember', 'true');
    localStorage.setItem('amerimon.admin.user', 'last-admin');
    localStorage.setItem('amerimon.admin.password', 'last-password');
    const service = TestBed.inject(AuthService);
    expect(service.getToken()).toBeNull();
    expect(service.recordarSesion()).toBe(true);
    expect(service.usuarioRecordado()).toBe('last-admin');
    expect(service.passwordRecordada()).toBe('last-password');
  });

  it('does not overwrite remembered credentials after a failed login', () => {
    localStorage.setItem('amerimon.admin.remember', 'true');
    localStorage.setItem('amerimon.admin.user', 'last-admin');
    localStorage.setItem('amerimon.admin.password', 'last-password');
    const service = TestBed.inject(AuthService);
    service.login('other-admin', 'wrong-password', true).subscribe({ error: () => {} });
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({}, { status: 401, statusText: 'Unauthorized' });
    expect(service.usuarioRecordado()).toBe('last-admin');
    expect(service.passwordRecordada()).toBe('last-password');
  });

  it('updates the remembered password only after a successful password change', () => {
    localStorage.setItem('amerimon.admin.remember', 'true');
    localStorage.setItem('amerimon.admin.user', 'admin');
    localStorage.setItem('amerimon.admin.password', 'old-password');
    const service = TestBed.inject(AuthService);
    service.cambiarPassword('old-password', 'new-password').subscribe();
    http.expectOne(`${environment.apiUrl}/auth/password`).flush({ status: 'ok' });
    expect(service.passwordRecordada()).toBe('new-password');
    service.cambiarPassword('wrong-password', 'bad-change').subscribe({ error: () => {} });
    http.expectOne(`${environment.apiUrl}/auth/password`).flush({}, { status: 401, statusText: 'Unauthorized' });
    expect(service.passwordRecordada()).toBe('new-password');
  });
});
