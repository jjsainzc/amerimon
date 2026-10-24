import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { AllstarlinkConnectionHistory } from '../../../core/models/allstarlink.model';
import { AllstarlinkService } from '../../../core/services/allstarlink.service';
import { AllstarlinkHistorialComponent } from './allstarlink-historial.component';

describe('History clipboard controls', () => {
  it('copies local and remote nodes, callsigns and IPs in desktop and mobile views without opening links', async () => {
    const connection: AllstarlinkConnectionHistory = {
      id: 1, instance_id: 'test', local_node: '616262', remote_node: '700001',
      connection_type: 'iax', remote_ip: '192.0.2.5', remote_callsign: 'HC1LNM',
      first_seen_at: '2026-10-24T12:00:00Z', last_seen_at: '2026-10-24T12:05:00Z',
      disconnected_at: '2026-10-24T12:05:00Z'
    };
    const list = vi.fn(() => of({ status: 'success', data: [connection] }));
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      TestBed.configureTestingModule({
        imports: [AllstarlinkHistorialComponent],
        providers: [
          provideRouter([]), provideHttpClient(), provideHttpClientTesting(),
          { provide: AllstarlinkService, useValue: { listarHistorialConexiones: list } }
        ]
      });
      const fixture = TestBed.createComponent(AllstarlinkHistorialComponent);
      fixture.detectChanges();
      const element: HTMLElement = fixture.nativeElement;
      for (const view of ['.historial-tabla-wrap', '.historial-cards']) {
        const region = element.querySelector(view)!;
        for (const value of ['616262', '700001', 'HC1LNM', '192.0.2.5']) {
          const button = region.querySelector<HTMLButtonElement>(`[aria-label="Copiar ${value} al portapapeles"]`)!;
          expect(button).not.toBeNull();
          button.click();
          await fixture.whenStable();
          expect(writeText).toHaveBeenLastCalledWith(value);
        }
        expect(region.querySelector('a button, button button')).toBeNull();
      }
      expect(element.querySelector('.geoip-overlay')).toBeNull();
      expect(list).toHaveBeenCalledTimes(1);
    } finally {
      Reflect.deleteProperty(navigator, 'clipboard');
    }
  });
});
