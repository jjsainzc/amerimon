import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { SystemInfoDialogComponent } from './system-info-dialog.component';
import { environment } from '../../core/environment';
import { TranslationService } from '../../core/services/translation.service';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';

describe('System maintenance controls', () => {
  let http: HttpTestingController;
  let component: SystemInfoDialogComponent;
  let fixture: ComponentFixture<SystemInfoDialogComponent>;

  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      imports: [SystemInfoDialogComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: TranslationService, useValue: {
          currentLang: signal('es'), revision: signal(0), translate: (key: string) => key
        } },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(SystemInfoDialogComponent);
    component = fixture.componentInstance;
    http.expectOne(`${environment.apiUrl}/system-info`).flush(
      { message: 'Metrics unavailable in this test' }, { status: 503, statusText: 'Unavailable' },
    );
  });

  afterEach(() => {
    http.verify();
    localStorage.clear();
  });

  it('remembers only the selected maintenance action after a valid confirmation', () => {
    component.preparar('asterisk_restart');
    component.noMostrarConfirmacion.set(true);
    component.confirmacion.set('wrong');
    component.ejecutar();
    const preferences = TestBed.inject(NoticePreferencesService);
    expect(preferences.isHidden('confirm.system.asterisk_restart')).toBe(false);
    component.confirmacion.set('RESTART ASTERISK');
    component.ejecutar();
    http.expectOne(`${environment.apiUrl}/system/control`).flush({
      status: 'success', message: 'OK',
      data: { action: 'asterisk_restart', recovered: false, scheduled: false, details: [], warnings: '' },
    });
    component.preparar('asterisk_restart');
    const request = http.expectOne(`${environment.apiUrl}/system/control`);
    expect(request.request.body)
      .toEqual({ action: 'asterisk_restart', confirmation: 'RESTART ASTERISK' });
    request.flush({ message: 'AMI unavailable' }, { status: 502, statusText: 'Bad Gateway' });
    expect(component.errorControl()).toBe('AMI unavailable');
    component.preparar('linux_reboot');
    expect(component.accion()).toBe('linux_reboot');
    expect(component.confirmacion()).toBe('');
    http.expectNone(`${environment.apiUrl}/system/control`);
  });

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
    TestBed.inject(NoticePreferencesService).hide('notice.system.diagnostics');
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
    fixture.detectChanges();
    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelector('.sys-error')?.textContent).toContain('Metrics unavailable');
    expect(element.textContent).toContain('AMI no responde');
    const warning = element.querySelector('.sys-advertencia')!;
    expect(warning.textContent).toContain('Error del arranque');
    expect(warning.closest('app-dismissible-notice')?.querySelector('input')).toBeNull();
  });
});
