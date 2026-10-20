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
  });

  it('persists a remembered login and removes it on logout', () => {
    const service = TestBed.inject(AuthService);
    service.login('admin', 'password', true).subscribe();
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({ status: 'ok', token: 'remembered-token' });
    expect(localStorage.getItem(key)).toBe('remembered-token');
    service.logout();
    expect(service.getToken()).toBeNull();
    expect(localStorage.getItem(key)).toBeNull();
  });

  it('restores the remembered token on initialization', () => {
    localStorage.setItem(key, 'restored-token');
    expect(TestBed.inject(AuthService).getToken()).toBe('restored-token');
  });

  it('removes an old remembered login when the new login is not remembered', () => {
    localStorage.setItem(key, 'old-token');
    const service = TestBed.inject(AuthService);
    service.login('admin', 'password', false).subscribe();
    http.expectOne(`${environment.apiUrl}/auth/login`).flush({ status: 'ok', token: 'new-token' });
    expect(service.getToken()).toBe('new-token');
    expect(localStorage.getItem(key)).toBeNull();
  });
});
