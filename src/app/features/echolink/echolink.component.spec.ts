import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { EchoLinkComponent } from './echolink.component';
import { provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { PasswordFieldComponent } from '../../shared/password-field/password-field.component';

describe('EchoLink administration', () => {
  afterEach(() => vi.unstubAllGlobals());

  function browserLocation() {
    const getCurrentPosition = vi.fn<(success: PositionCallback, error?: PositionErrorCallback | null, options?: PositionOptions) => void>();
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } });
    return getCurrentPosition;
  }

  function position(latitude: number, longitude: number): GeolocationPosition {
    return {
      coords: {
        latitude, longitude, accuracy: 10, altitude: null, altitudeAccuracy: null,
        heading: null, speed: null, toJSON: () => ({}),
      },
      timestamp: 0, toJSON: () => ({}),
    };
  }

  function setup() {
    TestBed.configureTestingModule({
      imports: [EchoLinkComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    const http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(EchoLinkComponent);
    for (const request of http.match(req => req.url.includes('/assets/i18n/'))) {
      request.flush({});
    }
    http.expectOne('/mon/echolink/config').flush({
      path: '/etc/asterisk/echolink.conf', astnode: '616262', revision: 'revision', loaded: true,
      sections: [{ section: 'el0', password_set: true, values: { call: 'TEST-L', astnode: '12345' } }],
    });
    return { http, fixture, component: fixture.componentInstance };
  }

  it('highlights loaded status on entry and updates it after disabling', async () => {
    const { http, fixture, component } = setup();
    fixture.autoDetectChanges();
    await fixture.whenStable();
    const status = fixture.nativeElement.querySelector('.module-status') as HTMLElement;
    expect(status.getAttribute('role')).toBe('status');
    expect(status.classList.contains('module-loaded')).toBe(true);
    expect(status.textContent).toContain('ECHOLINK_LOADED');
    component.pending = 'disable';
    component.apply();
    http.expectOne('/mon/echolink/activate').flush({ loaded: false });
    await fixture.whenStable();
    expect(status.classList.contains('module-unloaded')).toBe(true);
    expect(status.classList.contains('module-loaded')).toBe(false);
    expect(status.textContent).toContain('ECHOLINK_UNLOADED');
    fixture.destroy(); http.verify();
  });

  it('retrieves the existing password only when showing an empty field', () => {
    const { http, fixture, component } = setup();
    component.showPassword(false);
    http.expectNone(req => req.url.endsWith('/password'));
    component.showPassword(true);
    const request = http.expectOne(req => req.url === '/mon/echolink/password');
    expect(request.request.params.get('section')).toBe('el0');
    expect(request.request.params.get('revision')).toBe('revision');
    request.flush({ password: 'test-placeholder' });
    expect(component.password()).toBe('test-placeholder');
    component.showPassword(false);
    component.showPassword(true);
    http.expectNone(req => req.url.endsWith('/password'));
    fixture.destroy(); http.verify();
  });

  it('displays the saved password in the actual input after clicking the eye', async () => {
    const { http, fixture, component } = setup();
    fixture.autoDetectChanges();
    await fixture.whenStable();
    const eye = fixture.nativeElement.querySelector('app-password-field button') as HTMLButtonElement;
    eye.click();
    await fixture.whenStable();
    http.expectOne(req => req.url === '/mon/echolink/password').flush({ password: 'saved-placeholder' });
    expect(component.password()).toBe('saved-placeholder');
    await fixture.whenStable();
    expect(component.password()).toBe('saved-placeholder');
    expect(fixture.debugElement.query(By.directive(PasswordFieldComponent)).componentInstance.valor()).toBe('saved-placeholder');
    const input = fixture.nativeElement.querySelector('app-password-field input') as HTMLInputElement;
    expect(input.type).toBe('text');
    expect(input.value).toBe('saved-placeholder');
    eye.click();
    await fixture.whenStable();
    expect(input.type).toBe('password');
    expect(input.value).toBe('saved-placeholder');
    fixture.destroy(); http.verify();
  });

  it('shows a typed password without fetching or replacing it', () => {
    const { http, fixture, component } = setup();
    component.password.set('new-placeholder');
    component.showPassword(true);
    expect(component.password()).toBe('new-placeholder');
    http.expectNone(req => req.url.endsWith('/password'));
    fixture.destroy(); http.verify();
  });

  it('reports password retrieval failure and leaves the field empty', () => {
    const { http, fixture, component } = setup();
    component.showPassword(true);
    http.expectOne(req => req.url.endsWith('/password')).flush(
      { code: 'ECHOLINK_STALE' }, { status: 409, statusText: 'Conflict' });
    expect(component.error()).toBe('ECHOLINK_STALE');
    expect(component.password()).toBe('');
    expect(component.busy()).toBe(false);
    fixture.destroy(); http.verify();
  });

  it('saves primary astnode and blank password without changing module state', () => {
    const { http, fixture, component } = setup();
    component.save();
    const save = http.expectOne('/mon/echolink/config');
    expect(save.request.body.astnode).toBe('616262');
    expect(save.request.body.password).toBe('');
    expect(save.request.body.values.astnode).toBeUndefined();
    save.flush({ revision: 'next' });
    http.expectNone('/mon/echolink/activate');
    expect(component.notice()).toBe('ECHOLINK_SAVED');
    fixture.destroy(); http.verify();
  });

  it('requests browser permission on demand and fills coordinates without saving', () => {
    const getCurrentPosition = browserLocation();
    const { http, fixture, component } = setup();
    expect(getCurrentPosition).not.toHaveBeenCalled();
    component.useBrowserLocation();
    component.useBrowserLocation();
    expect(getCurrentPosition).toHaveBeenCalledTimes(1);
    expect(getCurrentPosition).toHaveBeenCalledWith(expect.any(Function), expect.any(Function),
      { enableHighAccuracy: true, maximumAge: 0, timeout: 10_000 });
    expect(component.locating()).toBe(true);
    getCurrentPosition.mock.calls[0][0](position(40.1234567, -3.7654321));
    expect(component.values['lat']).toBe('40.123457');
    expect(component.values['lon']).toBe('-3.765432');
    expect(component.notice()).toBe('ECHOLINK_LOCATION_READY');
    expect(component.busy()).toBe(false);
    expect(component.locating()).toBe(false);
    http.expectNone('/mon/echolink/config');
    http.expectNone('/mon/echolink/activate');
    fixture.destroy(); http.verify();
  });

  it.each([1, 2, 3])('preserves coordinates and reports location error %s', code => {
    const getCurrentPosition = browserLocation();
    const { http, fixture, component } = setup();
    component.values['lat'] = '10';
    component.values['lon'] = '20';
    component.useBrowserLocation();
    getCurrentPosition.mock.calls[0][1]?.({
      code, message: 'test', PERMISSION_DENIED: 1, POSITION_UNAVAILABLE: 2, TIMEOUT: 3,
    });
    expect(component.error()).toBe(code === 1 ? 'ECHOLINK_LOCATION_DENIED'
      : code === 3 ? 'ECHOLINK_LOCATION_TIMEOUT' : 'ECHOLINK_LOCATION_FAILED');
    expect(component.values['lat']).toBe('10');
    expect(component.values['lon']).toBe('20');
    expect(component.busy()).toBe(false);
    expect(component.notice()).toBe('');
    fixture.destroy(); http.verify();
  });

  it('reports unavailable geolocation without changing values', () => {
    vi.stubGlobal('navigator', {});
    const { http, fixture, component } = setup();
    component.useBrowserLocation();
    expect(component.error()).toBe('ECHOLINK_LOCATION_UNAVAILABLE');
    expect(component.busy()).toBe(false);
    fixture.destroy(); http.verify();
  });

  it('rejects invalid coordinates and ignores results after destruction', () => {
    const getCurrentPosition = browserLocation();
    const { http, fixture, component } = setup();
    component.useBrowserLocation();
    getCurrentPosition.mock.calls[0][0](position(NaN, 20));
    expect(component.error()).toBe('ECHOLINK_LOCATION_FAILED');
    expect(component.values['lat']).toBeUndefined();
    component.useBrowserLocation();
    fixture.destroy();
    getCurrentPosition.mock.calls[1][0](position(10, 20));
    expect(component.values['lat']).toBeUndefined();
    http.verify();
  });

  it('sends confirmed temporary disable and exposes failures without success', () => {
    const { http, fixture, component } = setup();
    component.pending = 'disable';
    component.apply();
    const request = http.expectOne('/mon/echolink/activate');
    expect(request.request.body).toEqual({ action: 'disable', persist: false, confirmation: 'APPLY ECHOLINK' });
    request.flush({ code: 'ECHOLINK_ACTIVATE_ERROR' }, { status: 502, statusText: 'Error' });
    expect(component.notice()).toBe('');
    expect(component.error()).toBe('ECHOLINK_ACTIVATE_ERROR');
    expect(component.pending).toBe('disable');
    expect(component.busy()).toBe(false);
    fixture.destroy(); http.verify();
  });
});
