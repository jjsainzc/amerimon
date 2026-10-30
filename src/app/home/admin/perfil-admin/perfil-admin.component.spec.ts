import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { NoticePreferencesService } from '../../../core/services/notice-preferences.service';
import { TranslationService } from '../../../core/services/translation.service';
import { environment } from '../../../core/environment';
import { PerfilAdminComponent } from './perfil-admin.component';

describe('Admin profile settings', () => {
  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      imports: [PerfilAdminComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: TranslationService, useValue: { revision: signal(0), translate: (key: string) => key } },
      ],
    });
  });
  afterEach(() => localStorage.clear());

  it('enables save only for changed profile settings, not notice preferences', async () => {
    const fixture = TestBed.createComponent(PerfilAdminComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    const url = `${environment.apiUrl}/site-settings`;
    http.expectOne(url).flush({ title: 'Amerimon', subtitle: '', weather_location: '' });
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(true);
    const title: HTMLInputElement = fixture.nativeElement.querySelector('input[name="tituloCabecera"]');
    title.value = 'New title';
    title.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    title.value = 'Amerimon';
    title.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
    http.expectNone(url);
    http.verify();
  });

  it('restores all notices without submitting header settings or erasing credentials', () => {
    const preferences = TestBed.inject(NoticePreferencesService);
    preferences.hide('confirm.asterisk.connect');
    preferences.hide('notice.system.diagnostics');
    localStorage.setItem('amerimon.admin.user', 'admin');
    const fixture = TestBed.createComponent(PerfilAdminComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`).flush({
      title: 'Amerimon', subtitle: '', asterisk_version: '',
    });
    fixture.detectChanges();
    const button = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>)
      .find(button => button.textContent?.includes('NOTICE_RESET'))!;
    button.click();
    fixture.detectChanges();
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(false);
    expect(preferences.isHidden('notice.system.diagnostics')).toBe(false);
    expect(localStorage.getItem('amerimon.admin.user')).toBe('admin');
    expect(fixture.nativeElement.textContent).toContain('NOTICE_RESET_DONE');
    http.verify();
  });

  it('keeps the reset button available when header settings cannot be loaded', () => {
    const preferences = TestBed.inject(NoticePreferencesService);
    preferences.hide('confirm.asterisk.connect');
    const fixture = TestBed.createComponent(PerfilAdminComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/site-settings`).flush(
      { message: 'Unavailable' }, { status: 503, statusText: 'Service Unavailable' },
    );
    fixture.detectChanges();
    const button = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>)
      .find(button => button.textContent?.includes('NOTICE_RESET'))!;
    expect(button.type).toBe('button');
    button.click();
    fixture.detectChanges();
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(false);
    expect(fixture.nativeElement.textContent).toContain('Unavailable');
    expect(fixture.nativeElement.textContent).toContain('NOTICE_RESET_DONE');
    http.verify();
  });
});
