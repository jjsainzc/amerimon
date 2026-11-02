import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { vi } from 'vitest';
import { App } from './app';
import { environment } from './core/environment';
import { WeatherComponent } from './shared/weather/weather.component';

describe('App', () => {
  beforeEach(async () => {
    localStorage.clear();
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();
  });

  afterEach(() => {
    const http = TestBed.inject(HttpTestingController);
    http.match(req => req.url === './assets/i18n/es.json')
      .forEach(req => req.flush({}));
    http.match(req => req.url === `${environment.apiUrl}/weather`)
      .filter(req => !req.cancelled)
      .forEach(req => req.flush(''));
    http.verify({ ignoreCancelled: true });
    localStorage.clear();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    expect(fixture.componentInstance).toBeTruthy();
  });

  it.each([undefined, '', ' Quito '])('loads weather after header settings arrive with location %s', async (location) => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    const weather: WeatherComponent = fixture.debugElement.query(By.directive(WeatherComponent)).componentInstance;
    const browserLocation = vi.spyOn(weather, 'getWeatherForBrowserLocation')
      .mockImplementation(() => weather.fetchWeather(''));
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '', weather_location: location });
    await fixture.whenStable();

    const request = http.expectOne(req => req.url === `${environment.apiUrl}/weather`);
    expect(request.request.params.get('location')).toBe(location?.trim() || null);
    expect(browserLocation).toHaveBeenCalledTimes(location?.trim() ? 0 : 1);
    request.flush('Quito: 60°F');
    expect(weather.weatherReport()).toBe('Quito: 60°F');
  });

  it('shows the admin login button when not authenticated', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('.menu .sesion > button.boton-enlace')?.textContent).toContain('Admin');
    TestBed.inject(HttpTestingController).expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
  });

  it('shows the combined password tools page in the public navigation', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    TestBed.inject(HttpTestingController).expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });

    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelectorAll('.enlaces a[href="/password-tools"]')).toHaveLength(1);
    expect(element.querySelector('.enlaces a[href="/password-tools"]')?.textContent)
      .toContain('PASSWORD_TOOLS_NAV');
  });

  it('shows the Amerimon logo and remember-session checkbox in the login dialog', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    const element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.menu .sesion > button.boton-enlace')!.click();
    http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: true });
    await fixture.whenStable();
    expect(element.querySelector<HTMLImageElement>('.login-logo')?.getAttribute('src'))
      .toBe('assets/images/amerimon.png');
    expect(element.querySelector('input[name="recordarSesion"]')).not.toBeNull();
  });

  it('does not validate the repeated password when the new password is visible', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    const element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.menu .sesion > button.boton-enlace')!.click();
    http.expectOne(`${environment.apiUrl}/auth/status`).flush({ configured: false });
    await fixture.whenStable();

    const username = element.querySelector<HTMLInputElement>('input[name="usuario"]')!;
    username.value = 'admin';
    username.dispatchEvent(new Event('input', { bubbles: true }));
    const password = element.querySelector<HTMLInputElement>('#dialogo-nueva')!;
    password.value = 'pass1234';
    password.dispatchEvent(new Event('input', { bubbles: true }));
    password.parentElement!.querySelector<HTMLButtonElement>('.pf-ojo')!.click();
    await fixture.whenStable();

    element.querySelector<HTMLFormElement>('form.dialogo')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    const request = http.expectOne(`${environment.apiUrl}/auth/setup`);
    expect(request.request.body).toEqual({ user: 'admin', new: 'pass1234' });
    request.flush({ status: 'ok', token: 'header.payload.signature' });
    await fixture.whenStable();
    expect(element.querySelector('.error')).toBeNull();
    expect(element.querySelector('form.dialogo')).toBeNull();
  });

  it('shows a compact footer with the backend version and SQLite instead of SqLite3', async () => {
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
    expect(footer.textContent).not.toMatch(/SqLite3|Términos|Condiciones|Nosotros|Privacidad/);
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
      element.querySelector<HTMLButtonElement>('.menu .sesion > button.boton-enlace')!.click();
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
    element.querySelector<HTMLButtonElement>('.menu .sesion > button.boton-enlace')!.click();
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
    expect(element.querySelector('.sesion a[routerLink="/admin/favoritos"]')).not.toBeNull();
    expect(element.querySelector('.sesion a[routerLink="/admin/enlaces-externos"]')).not.toBeNull();
    expect(element.querySelector('.sesion a[routerLink="/admin/allmon"]')).toBeNull();
    const adminMenuButton = element.querySelector<HTMLButtonElement>('.menu .admin-desplegable-boton')!;
    adminMenuButton.click();
    await fixture.whenStable();
    expect(element.querySelector('.admin-desplegable-panel a[href="/admin/allmon"]')).not.toBeNull();
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await fixture.whenStable();
    expect(element.querySelector('.admin-desplegable-panel')).toBeNull();
    const hamburger = element.querySelector<HTMLButtonElement>('.hamburguesa')!;
    hamburger.click();
    await fixture.whenStable();
    expect(element.querySelector('.menu')?.classList.contains('abierto')).toBe(true);
    expect(element.querySelector('.admin-desplegable-panel a[href="/admin/allmon"]')).not.toBeNull();
    expect(element.querySelector('.admin-desplegable-panel a[href="/admin/perfil"]')).not.toBeNull();
    expect(element.querySelector('.admin-desplegable-panel a[href="/admin/cabecera"]')).toBeNull();
    expect(element.querySelector('.admin-desplegable-panel a[href="/admin/cambiar-contrasena"]')).not.toBeNull();
    expect(element.querySelector('.admin-desplegable-panel a[href="/manual-tecnico"]')).not.toBeNull();
    hamburger.click();
    await fixture.whenStable();
    expect(element.querySelector('.admin-desplegable-panel')).toBeNull();
    element.querySelector<HTMLButtonElement>('[aria-label="Salir"]')!.click();
    await fixture.whenStable();
    expect(localStorage.getItem('amerimon.admin.token')).toBeNull();
    fixture.destroy();
    fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    http.expectOne(`${environment.apiUrl}/site-settings`)
      .flush({ title: 'Amerimon', subtitle: '', asterisk_version: '' });
    element = fixture.nativeElement as HTMLElement;
    element.querySelector<HTMLButtonElement>('.menu .sesion > button.boton-enlace')!.click();
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
