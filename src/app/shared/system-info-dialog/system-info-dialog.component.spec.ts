import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { SystemInfoDialogComponent } from './system-info-dialog.component';
import { environment } from '../../core/environment';
import { TranslationService } from '../../core/services/translation.service';

describe('System maintenance controls', () => {
  let http: HttpTestingController;
  let component: SystemInfoDialogComponent;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [SystemInfoDialogComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: TranslationService, useValue: { translate: (key: string) => key } },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    component = TestBed.createComponent(SystemInfoDialogComponent).componentInstance;
    http.expectOne(`${environment.apiUrl}/system-info`).flush(
      { message: 'Metrics unavailable in this test' }, { status: 503, statusText: 'Unavailable' },
    );
  });

  afterEach(() => http.verify());

  it('does not send any operation until the exact phrase is confirmed', () => {
    component.preparar('asterisk_restart');
    component.confirmacion.set('REINICIAR');
    component.ejecutar();
    http.expectNone(`${environment.apiUrl}/system/control`);
    component.confirmacion.set('RESTART ASTERISK');
    component.ejecutar();
    const req = http.expectOne(`${environment.apiUrl}/system/control`);
    expect(req.request.body).toEqual({ action: 'asterisk_restart', confirmation: 'RESTART ASTERISK' });
    req.flush({
      status: 'success', message: 'Asterisk operativo',
      data: { action: 'asterisk_restart', recovered: false, scheduled: false, details: ['AMI OK'], warnings: '' },
    });
    expect(component.ejecutando()).toBe(false);
    expect(component.accion()).toBeNull();
    expect(component.resultado()?.data.details).toEqual(['AMI OK']);
  });

  it('blocks repeated submissions while busy and after scheduling a host shutdown', () => {
    component.preparar('linux_poweroff');
    component.confirmacion.set('POWER OFF LINUX');
    component.ejecutar();
    component.ejecutar();
    const req = http.expectOne(`${environment.apiUrl}/system/control`);
    req.flush({
      status: 'success', message: 'Apagado programado',
      data: { action: 'linux_poweroff', recovered: false, scheduled: true, details: [], warnings: '' },
    });
    component.preparar('linux_reboot');
    component.ejecutar();
    http.expectNone(`${environment.apiUrl}/system/control`);
    expect(component.programado()).toBe(true);
  });

  it('shows backend failure and diagnostic warnings instead of success', () => {
    component.preparar('asterisk_restart');
    component.confirmacion.set('RESTART ASTERISK');
    component.ejecutar();
    http.expectOne(`${environment.apiUrl}/system/control`).flush({
      status: 'error', message: 'AMI no responde',
      data: { action: 'asterisk_restart', recovered: true, scheduled: false, details: ['Se requirió SIGKILL'], warnings: 'Error del arranque' },
    }, { status: 502, statusText: 'Bad Gateway' });
    expect(component.errorControl()).toBe('AMI no responde');
    expect(component.resultado()?.data.warnings).toBe('Error del arranque');
    expect(component.ejecutando()).toBe(false);
  });
});
