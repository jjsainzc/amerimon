import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { environment } from '../../core/environment';
import { WeatherComponent } from './weather.component';

describe('WeatherComponent', () => {
  let http: HttpTestingController;
  let component: WeatherComponent;
  let fixture: ComponentFixture<WeatherComponent>;
  let dialog: HTMLDialogElement;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [WeatherComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(WeatherComponent);
    component = fixture.componentInstance;
    vi.spyOn(component, 'getWeatherForBrowserLocation').mockImplementation(() => {});
    fixture.detectChanges();
    dialog = fixture.nativeElement.querySelector('dialog');
    dialog.showModal = vi.fn(() => dialog.setAttribute('open', ''));
    dialog.close = vi.fn(() => {
      dialog.removeAttribute('open');
      dialog.dispatchEvent(new Event('close'));
    });
  });

  afterEach(() => {
    http.verify();
    fixture.destroy();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('loads weather for the supplied browser coordinates', () => {
    component.fetchWeather('37.7749,-122.4194');

    const request = http.expectOne(
      req => req.url === `${environment.apiUrl}/weather`
        && req.params.get('location') === '37.7749,-122.4194'
        && req.params.get('mode') === 'F',
    );
    expect(request.request.params.get('location')).toBe('37.7749,-122.4194');
    expect(request.request.params.get('mode')).toBe('F');
    request.flush('68°F / Sunny');

    expect(component.weatherReport()).toBe('68°F / Sunny');
    expect(component.consultedLocation()).toBe('37.7749,-122.4194');
    expect(component.loading()).toBe(false);
  });

  it.each(['Quito', '40.4,-3.7', 'San José & alrededores'])('links %s to Google Maps in a new tab', (location) => {
    component.fetchWeather(location);
    http.expectOne(req => req.params.get('location') === location).flush('60°F');
    fixture.detectChanges();

    const link: HTMLAnchorElement = fixture.nativeElement.querySelector('.location-message a');
    expect(link.textContent).toBe(location);
    expect(link.href).toBe(`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(location)}`);
    expect(link.target).toBe('_blank');
    expect(link.rel).toBe('noopener noreferrer');
  });

  it('does not search Google Maps for the automatic IP placeholder', () => {
    component.fetchWeather('');
    http.expectOne(req => !req.params.has('location')).flush('60°F');
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.location-message a')).toBeNull();
    expect(component.consultedLocation()).toBe('WEATHER_AUTOMATIC_IP_LOCATION');
  });

  it('refreshes the current location from the manual refresh button', () => {
    component.fetchWeather('Quito');
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    fixture.detectChanges();

    const refreshButton: HTMLButtonElement = fixture.nativeElement.querySelector('.weather-refresh');
    expect(refreshButton.getAttribute('aria-label')).toBe('Actualizar clima ahora');
    expect(refreshButton.parentElement?.querySelector('.report-content')).not.toBeNull();
    expect(refreshButton.closest('.location-row')).toBeNull();
    refreshButton.click();
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 61°F');
    expect(component.weatherReport()).toBe('Quito: 61°F');
  });

  it('refreshes automatically every five minutes and clears the timer on destroy', () => {
    vi.useFakeTimers();
    const timedFixture = TestBed.createComponent(WeatherComponent);
    timedFixture.componentRef.setInput('ubicacionConfigurada', 'Quito');
    timedFixture.detectChanges();
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');

    vi.advanceTimersByTime(299_999);
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
    vi.advanceTimersByTime(1);
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 61°F');

    timedFixture.destroy();
    vi.advanceTimersByTime(300_000);
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
  });

  it('retries failed requests automatically in the background', () => {
    vi.useFakeTimers();
    component.fetchWeather('Quito');

    http.expectOne(req => req.params.get('location') === 'Quito')
      .flush(JSON.stringify({ message: 'No se pudo obtener el clima' }), {
        status: 502,
        statusText: 'Bad Gateway',
      });

    expect(component.errorMessage()).toBe('No se pudo obtener el clima');
    expect(component.consultedLocation()).toBe('Quito');
    expect(component.loading()).toBe(false);

    vi.advanceTimersByTime(30_000);

    const retry = http.expectOne(req => req.params.get('location') === 'Quito');
    expect(component.loading()).toBe(true);
    retry.flush('Quito: 60°F');
    expect(component.weatherReport()).toBe('Quito: 60°F');
    expect(component.loading()).toBe(false);
  });

  it('falls back to the server location when the visitor denies geolocation', () => {
    vi.mocked(component.getWeatherForBrowserLocation).mockRestore();
    const getCurrentPosition = vi.fn((_ok: PositionCallback, fail?: PositionErrorCallback | null) =>
      fail?.({ code: 1, message: 'denied' } as GeolocationPositionError));
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });

    component.getWeatherForBrowserLocation();
    vi.unstubAllGlobals();

    const request = http.expectOne(req => req.url === `${environment.apiUrl}/weather` && !req.params.has('location'));
    request.flush('Quito: 60°F');
    expect(component.weatherReport()).toBe('Quito: 60°F');
  });

  it('uses the configured header location and skips browser geolocation', () => {
    component.ubicacionConfigurada = ' Quito ';

    expect(component.getWeatherForBrowserLocation).not.toHaveBeenCalled();
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    expect(component.weatherReport()).toBe('Quito: 60°F');
  });

  it('asks the browser for its location when no location is configured', () => {
    component.ubicacionConfigurada = '';

    expect(component.getWeatherForBrowserLocation).toHaveBeenCalledTimes(1);
  });

  it('waits until the header settings are loaded', () => {
    component.ubicacionConfigurada = undefined;

    expect(component.getWeatherForBrowserLocation).not.toHaveBeenCalled();
  });

  it('opens the location popup from the consulted location row without requiring admin access', () => {
    component.fetchWeather('Quito');
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    fixture.detectChanges();

    const row: HTMLElement = fixture.nativeElement.querySelector('.location-row');
    expect(row.textContent).toContain('Localidad consultada: Quito');
    (row.querySelector('.weather-change') as HTMLButtonElement).click();
    expect(dialog.showModal).toHaveBeenCalled();
  });

  it('only offers the configured location when a nonempty admin value exists', () => {
    component.ubicacionConfigurada = '   ';
    fixture.detectChanges();
    expect(dialog.textContent).not.toContain('Usar configuración');

    component.ubicacionConfigurada = 'Quito';
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    fixture.detectChanges();
    expect(dialog.textContent).toContain('Usar configuración');
  });

  it('uses a manually entered location without modifying the configured default', () => {
    component.ubicacionConfigurada = 'Quito';
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    component.openLocationDialog(dialog);
    const input: HTMLInputElement = dialog.querySelector('input')!;
    input.value = ' Madrid ';
    input.dispatchEvent(new Event('input'));
    dialog.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

    http.expectOne(req => req.params.get('location') === 'Madrid').flush('Madrid: 70°F');
    expect(component.configuredLocation()).toBe('Quito');
    expect(component.consultedLocation()).toBe('Madrid');
    expect(dialog.close).toHaveBeenCalled();

    component.loadWeather();
    http.expectOne(req => req.params.get('location') === 'Madrid').flush('Madrid: 70°F');
    component.useConfiguredLocation(dialog);
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    expect(component.consultedLocation()).toBe('Quito');
  });

  it('preserves the temporary selection when admin settings finish loading or change', () => {
    component.locationDraft = 'Madrid';
    component.useEnteredLocation(dialog);
    http.expectOne(req => req.params.get('location') === 'Madrid').flush('Madrid: 70°F');
    component.ubicacionConfigurada = 'Quito';
    component.ubicacionConfigurada = 'Lima';
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
    expect(component.consultedLocation()).toBe('Madrid');
    component.useConfiguredLocation(dialog);
    http.expectOne(req => req.params.get('location') === 'Lima').flush('Lima: 65°F');
  });

  it('rejects an empty manual location with a visible message', () => {
    component.locationDraft = '   ';
    component.useEnteredLocation(dialog);
    fixture.detectChanges();

    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain('Introduce una ubicación');
    expect(dialog.close).not.toHaveBeenCalled();
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
  });

  it('forces fresh browser geolocation even when an admin default exists', () => {
    component.ubicacionConfigurada = 'Quito';
    http.expectOne(req => req.params.get('location') === 'Quito').flush('Quito: 60°F');
    const getCurrentPosition = vi.fn();
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });

    component.useBrowserLocation(dialog);
    expect(getCurrentPosition).toHaveBeenCalledWith(
      expect.any(Function), expect.any(Function),
      { enableHighAccuracy: true, maximumAge: 0, timeout: 10_000 },
    );
    const success: PositionCallback = getCurrentPosition.mock.calls[0][0];
    success({ coords: { latitude: 40.4, longitude: -3.7 } } as GeolocationPosition);
    http.expectOne(req => req.params.get('location') === '40.4,-3.7').flush('Madrid: 70°F');
    expect(component.configuredLocation()).toBe('Quito');
    component.loadWeather();
    http.expectOne(req => req.params.get('location') === '40.4,-3.7').flush('Madrid: 70°F');
  });

  it.each([1, 2, 3])('reports geolocation failure code %s without silently using a different location', (code) => {
    const getCurrentPosition = vi.fn();
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });
    component.useBrowserLocation(dialog);
    const fail: PositionErrorCallback = getCurrentPosition.mock.calls[0][1];
    fail({ code } as GeolocationPositionError);
    fixture.detectChanges();

    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain(
      code === 1 ? 'Permiso de ubicación denegado' : 'No se pudo obtener tu ubicación',
    );
    expect(component.locating()).toBe(false);
    expect(component.loading()).toBe(false);
    expect(dialog.close).not.toHaveBeenCalled();
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
  });

  it('explains unavailable browser geolocation without falling back to the admin default', () => {
    vi.stubGlobal('navigator', {});
    component.useBrowserLocation(dialog);
    expect(component.locationError()).toContain('Usa HTTPS');
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
  });

  it('ignores an old geolocation callback after a manual selection', () => {
    const getCurrentPosition = vi.fn();
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });
    component.useBrowserLocation(dialog);
    component.locationDraft = 'Madrid';
    component.useEnteredLocation(dialog);
    const request = http.expectOne(req => req.params.get('location') === 'Madrid');
    const success: PositionCallback = getCurrentPosition.mock.calls[0][0];
    success({ coords: { latitude: 0, longitude: 0 } } as GeolocationPosition);
    request.flush('Madrid: 70°F');
    http.expectNone(req => req.params.get('location') === '0,0');
    expect(component.consultedLocation()).toBe('Madrid');
  });

  it('ignores a geolocation result after the dialog is dismissed', () => {
    const getCurrentPosition = vi.fn();
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });
    component.useBrowserLocation(dialog);
    dialog.close();
    const success: PositionCallback = getCurrentPosition.mock.calls[0][0];
    success({ coords: { latitude: 0, longitude: 0 } } as GeolocationPosition);
    expect(component.loading()).toBe(false);
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
  });

  it('cancels old weather requests and retries when a new location is selected', () => {
    vi.useFakeTimers();
    component.fetchWeather('Quito');
    const oldRequest = http.expectOne(req => req.params.get('location') === 'Quito');
    component.locationDraft = 'Madrid';
    component.useEnteredLocation(dialog);
    expect(oldRequest.cancelled).toBe(true);
    http.expectOne(req => req.params.get('location') === 'Madrid')
      .flush('Unavailable', { status: 502, statusText: 'Bad Gateway' });
    component.locationDraft = 'Lima';
    component.useEnteredLocation(dialog);
    http.expectOne(req => req.params.get('location') === 'Lima').flush('Lima: 65°F');
    vi.advanceTimersByTime(30_000);
    http.expectNone(req => req.url === `${environment.apiUrl}/weather`);
    expect(component.weatherReport()).toBe('Lima: 65°F');
  });
});
