import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { environment } from '../../core/environment';
import { WeatherComponent } from './weather.component';

describe('WeatherComponent', () => {
  let http: HttpTestingController;
  let component: WeatherComponent;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [WeatherComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(WeatherComponent);
    component = fixture.componentInstance;
    vi.spyOn(component, 'getWeatherForBrowserLocation').mockImplementation(() => {});
    fixture.detectChanges();
  });

  afterEach(() => http.verify());

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
});
