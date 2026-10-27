import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TranslationService } from '../../core/services/translation.service';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';
import { DismissibleNoticeComponent } from './dismissible-notice.component';

@Component({
  imports: [DismissibleNoticeComponent],
  template: `
    <app-dismissible-notice noticeKey="notice.system.diagnostics" [alwaysShow]="error">
      <p role="alert">Diagnostic details</p>
    </app-dismissible-notice>
  `,
})
class TestHost {
  error = false;
}

describe('Dismissible notices', () => {
  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      imports: [TestHost],
      providers: [{
        provide: TranslationService,
        useValue: { revision: signal(0), translate: (key: string) => key },
      }],
    });
  });
  afterEach(() => localStorage.clear());

  it('hides only the selected warning and restores it on reset', () => {
    const fixture = TestBed.createComponent(TestHost);
    fixture.detectChanges();
    const checkbox = fixture.nativeElement.querySelector('input') as HTMLInputElement;
    checkbox.checked = true;
    checkbox.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')).toBeNull();
    const preferences = TestBed.inject(NoticePreferencesService);
    expect(preferences.isHidden('confirm.system.asterisk_restart')).toBe(false);
    preferences.reset();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')).not.toBeNull();
  });

  it('always shows error details without a suppression checkbox', () => {
    TestBed.inject(NoticePreferencesService).hide('notice.system.diagnostics');
    const fixture = TestBed.createComponent(TestHost);
    fixture.componentInstance.error = true;
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')?.textContent).toContain('Diagnostic details');
    expect(fixture.nativeElement.querySelector('input[type="checkbox"]')).toBeNull();
  });
});
