import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { App } from './app';
import { environment } from './core/environment';

describe('App', () => {
  beforeEach(async () => {
    localStorage.clear();
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();
  });

  afterEach(() => {
    TestBed.inject(HttpTestingController).verify({ ignoreCancelled: true });
    localStorage.clear();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    expect(fixture.componentInstance).toBeTruthy();
  });

  it('shows the admin login button when not authenticated', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('.sesion button')?.textContent).toContain('Admin');
    TestBed.inject(HttpTestingController).expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
  });

  it('shows the Amerimon logo and remember-session checkbox in the login dialog', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    const element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.sesion button')!.click();
    http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: true });
    await fixture.whenStable();
    expect(element.querySelector<HTMLImageElement>('.login-logo')?.getAttribute('src'))
      .toBe('assets/images/amerimon.png');
    expect(element.querySelector('input[name="recordarSesion"]')).not.toBeNull();
  });

  it('shows a compact footer with the backend version and SQLite instead of MariaDB', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    TestBed.inject(HttpTestingController).expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '', app_version: '1.0.9' });
    await fixture.whenStable();
    const footer = (fixture.nativeElement as HTMLElement).querySelector('footer')!;
    expect(footer.textContent).toContain('Versión 1.0.9');
    expect(footer.textContent).toContain('Powered by');
    expect(footer.textContent).toContain('Angular Framework');
    expect(footer.textContent).toContain('SQLite3');
    expect(footer.textContent).toContain('Go Language');
    expect(footer.textContent).not.toMatch(/MariaDB|Términos|Condiciones|Nosotros|Privacidad/);
    expect(footer.querySelector('img[src="assets/images/sqlite.svg"]')).not.toBeNull();
    expect(footer.querySelectorAll('a')).toHaveLength(0);
  });

  it('keeps the remember checkbox checked when reopening login without a token', async () => {
    localStorage.setItem('amerimon.admin.remember', 'true');
    localStorage.setItem('amerimon.admin.user', 'last-admin');
    localStorage.setItem('amerimon.admin.password', 'last-password');
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    const element = fixture.nativeElement as HTMLElement;
    for (let attempt = 0; attempt < 2; attempt++) {
      element.querySelector<HTMLButtonElement>('.sesion button')!.click();
      http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: true });
      await fixture.whenStable();
      expect(element.querySelector<HTMLInputElement>('input[name="recordarSesion"]')?.checked).toBe(true);
      expect(element.querySelector<HTMLInputElement>('input[name="usuario"]')?.value).toBe('last-admin');
      expect(element.querySelector<HTMLInputElement>('input[type="password"]')?.value).toBe('last-password');
      element.querySelector<HTMLButtonElement>('.dialogo .cerrar')!.click();
      await fixture.whenStable();
    }
  });

  it('restores the checked box, username and password after login, logout and app recreation', async () => {
    const http = TestBed.inject(HttpTestingController);
    let fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    let element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.sesion button')!.click();
    http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: true });
    await fixture.whenStable();
    const username = element.querySelector<HTMLInputElement>('input[name="usuario"]')!;
    username.value = 'last-admin';
    username.dispatchEvent(new Event('input'));
    const password = element.querySelector<HTMLInputElement>('input[type="password"]')!;
    password.value = 'last-password';
    password.dispatchEvent(new Event('input'));
    const remember = element.querySelector<HTMLInputElement>('input[name="recordarSesion"]')!;
    remember.checked = true;
    remember.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    element.querySelector<HTMLFormElement>('form.dialogo')!.dispatchEvent(new Event('submit', { cancelable: true }));
    const request = http.expectOne(`${environment.apiUrl}/auth/login`);
    expect(request.request.body).toEqual({ user: 'last-admin', password: 'last-password' });
    request.flush({ status: 'ok', token: `header.${btoa(JSON.stringify({ role: 'A' }))}.signature` });
    await fixture.whenStable();
    expect(element.querySelector('form.dialogo')).toBeNull();
    Array.from(element.querySelectorAll<HTMLButtonElement>('.sesion button'))
      .find(button => button.textContent?.trim() === 'Salir')!.click();
    await fixture.whenStable();
    expect(localStorage.getItem('amerimon.admin.token')).toBeNull();
    fixture.destroy();
    fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.sesion button')!.click();
    http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: true });
    await fixture.whenStable();
    expect(element.querySelector<HTMLInputElement>('input[name="recordarSesion"]')?.checked).toBe(true);
    expect(element.querySelector<HTMLInputElement>('input[name="usuario"]')?.value).toBe('last-admin');
    expect(element.querySelector<HTMLInputElement>('input[type="password"]')?.value).toBe('last-password');
  });

  it('makes the external-links selector available without authentication', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    const element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.enlaces button')!.click();
    http.expectOne(`${environment.apiUrl}/enlaces-externos`).flush([
      { dashid: 1, descripcion: 'Red', url: 'https://example.org/', target: '_blank', is_active: 1 }
    ]);
    await fixture.whenStable();
    const link = element.querySelector<HTMLAnchorElement>('.enlaces-lista a');
    expect(link?.getAttribute('href')).toBe('https://example.org/');
    expect(link?.target).toBe('_blank');
    expect(link?.rel).toContain('noopener');
  });
});
