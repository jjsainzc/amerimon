import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { environment } from '../../../core/environment';
import { TranslationService } from '../../../core/services/translation.service';
import { NextionSettings, NextionStatus } from '../../../core/services/nextion.service';
import { NextionComponent } from './nextion.component';

describe('Nextion admin', () => {
  const initialSettings: NextionSettings = {
    enabled: false, port: '/dev/ttyUSB0', baud_rate: 9600, interface: '', favorite_node: '',
  };
  const status = (settings = initialSettings): NextionStatus => ({
    settings,
    ports: [{ path: '/dev/ttyUSB0', label: '/dev/ttyUSB0 — Silicon Labs CP210x (10c4:ea60)', vendor_id: '10c4', product_id: 'ea60' }],
    models: [{
      name: 'AMERINET_Nextion_NX4832K035-es', model: 'NX4832K035', language: 'es',
      preview_url: 'assets/images/AMERINET_Nextion_NX4832K035-es.png', source_available: true,
    }],
    runtime: { running: false, port: '' },
    interfaces: [],
    current_ipv4: '192.0.2.10',
    local_node: '616262',
    ami_configured: true,
  });

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [NextionComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: TranslationService, useValue: { revision: signal(0), translate: (key: string) => key } },
      ],
    });
  });

  it('saves incomplete settings when the controller is disabled', async () => {
    const fixture = TestBed.createComponent(NextionComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/nextion`).flush(status());
    http.expectOne(`${environment.apiUrl}/favoritos`).flush([]);
    await fixture.whenStable();
    fixture.detectChanges();

    const port: HTMLInputElement = fixture.nativeElement.querySelector('input[name="port"]');
    port.value = '/dev/ttyUSB';
    port.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));

    const request = http.expectOne(`${environment.apiUrl}/nextion`);
    expect(request.request.method).toBe('PUT');
    expect(request.request.body).toEqual({
      enabled: false, port: '/dev/ttyUSB', baud_rate: 9600, interface: '', favorite_node: '',
    });
    request.flush(status({ ...initialSettings, port: '/dev/ttyUSB' }));
    await fixture.whenStable();
    http.verify();
  });

  it('tests the selected serial port with an icon-only action button', async () => {
    const fixture = TestBed.createComponent(NextionComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/nextion`).flush(status());
    http.expectOne(`${environment.apiUrl}/favoritos`).flush([]);
    await fixture.whenStable();
    fixture.detectChanges();

    const button: HTMLButtonElement = fixture.nativeElement.querySelector('button[aria-label="NEXTION_TEST_PORT"]');
    expect(button.textContent?.trim()).toBe('');
    button.click();
    const request = http.expectOne(`${environment.apiUrl}/nextion/test-port`);
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ port: '/dev/ttyUSB0', baud_rate: 9600 });
    request.flush({ status: 'ok' });
    await fixture.whenStable();
    http.verify();
  });

  it('opens the preview in a lightbox and closes it with overlay click or Escape', async () => {
    const fixture = TestBed.createComponent(NextionComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/nextion`).flush(status());
    http.expectOne(`${environment.apiUrl}/favoritos`).flush([]);
    await fixture.whenStable();
    fixture.detectChanges();

    fixture.nativeElement.querySelector('.preview-trigger').click();
    fixture.detectChanges();
    const overlay: HTMLElement = fixture.nativeElement.querySelector('.lightbox-overlay');
    expect(overlay).not.toBeNull();
    expect(overlay.querySelector('.lightbox-image')?.getAttribute('src'))
      .toContain('/assets/images/AMERINET_Nextion_NX4832K035-es.png');

    overlay.click();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.lightbox-overlay')).toBeNull();

    fixture.nativeElement.querySelector('.preview-trigger').click();
    fixture.detectChanges();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.lightbox-overlay')).toBeNull();
    http.verify();
  });
});
