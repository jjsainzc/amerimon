import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { AllmonConfigService } from '../../../core/services/allmon-config.service';
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
});
