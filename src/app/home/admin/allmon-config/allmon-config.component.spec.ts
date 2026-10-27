import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { signal } from '@angular/core';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { AllmonConfigService } from '../../../core/services/allmon-config.service';
import { TranslationService } from '../../../core/services/translation.service';
import { AllmonConfigComponent } from './allmon-config.component';

describe('AllmonConfigComponent', () => {
  let fixture: ComponentFixture<AllmonConfigComponent>;
  let service: { obtener: ReturnType<typeof vi.fn>; guardar: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    service = {
      obtener: vi.fn(() => of({
        status: 'ok',
        path: '/etc/amerimon/allmon.ini',
        writable: true,
        nodes: [{
          node: '123456',
          host: '192.0.2.1',
          port: '5038',
          user: 'admin',
          passwd: '',
          passwd_set: true,
          tracking_enabled: true,
        }],
      })),
      guardar: vi.fn(() => of({ status: 'ok', path: '/etc/amerimon/allmon.ini' })),
    };
    TestBed.configureTestingModule({
      imports: [AllmonConfigComponent],
      providers: [
        { provide: AllmonConfigService, useValue: service },
        {
          provide: TranslationService,
          useValue: {
            currentLang: signal('es'),
            revision: signal(0),
            translate: (key: string, params?: Record<string, string | number>) => {
              const labels: Record<string, string> = { NODO: 'Nodo', HOST: 'Host' };
              return (labels[key] ?? key).replace('{{node}}', String(params?.['node'] ?? ''));
            },
          },
        },
        provideRouter([]),
      ],
    });
    fixture = TestBed.createComponent(AllmonConfigComponent);
    fixture.detectChanges();
  });

  it('renders and saves the tracking checkbox while retaining the node row', () => {
    const checkbox: HTMLInputElement = fixture.nativeElement.querySelector('input[type="checkbox"]');
    expect(checkbox.checked).toBe(true);
    expect(fixture.nativeElement.querySelector('.allmon-quitar i.fa-trash-can')).not.toBeNull();
    checkbox.click();
    fixture.detectChanges();

    fixture.componentInstance.guardar();

    expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(1);
    expect(service.guardar).toHaveBeenCalledWith([expect.objectContaining({
      node: '123456',
      tracking_enabled: false,
    })]);
  });

  it('copies edited host and node values without saving or exposing secrets', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      const host: HTMLInputElement = fixture.nativeElement.querySelector('[data-label="Host"] input');
      host.value = '192.0.2.55';
      host.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      host.parentElement!.querySelector<HTMLButtonElement>('.clipboard-copy')!.click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('192.0.2.55');
      const node: HTMLInputElement = fixture.nativeElement.querySelector('[data-label="Nodo"] input');
      node.parentElement!.querySelector<HTMLButtonElement>('.clipboard-copy')!.click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('123456');
      expect(fixture.nativeElement.querySelectorAll('tbody .clipboard-copy').length).toBe(2);
      expect(fixture.nativeElement.querySelector('app-password-field .clipboard-copy')).toBeNull();
      expect(service.guardar).not.toHaveBeenCalled();
    } finally {
      Reflect.deleteProperty(navigator, 'clipboard');
    }
  });
});
