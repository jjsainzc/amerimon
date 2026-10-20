import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { App } from './app';
import { environment } from './core/environment';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();
  });

  afterEach(() => {
    TestBed.inject(HttpTestingController).verify({ ignoreCancelled: true });
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
