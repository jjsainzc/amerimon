import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { TranslationService } from '../../core/services/translation.service';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';
import { ConfirmacionDialogComponent } from './confirmacion-dialog.component';

describe('Confirmation preferences', () => {
  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      imports: [ConfirmacionDialogComponent],
      providers: [{
        provide: TranslationService,
        useValue: { revision: signal(0), translate: (key: string) => key },
      }],
    });
  });
  afterEach(() => localStorage.clear());

  function create() {
    const fixture = TestBed.createComponent(ConfirmacionDialogComponent);
    fixture.componentRef.setInput('noticeKey', 'confirm.favorite.delete');
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    return fixture;
  }

  it('remembers the checkbox only after confirmation and skips later prompts', async () => {
    const fixture = create();
    const confirmed = vi.fn();
    fixture.componentInstance.confirmed.subscribe(confirmed);
    const element = fixture.nativeElement as HTMLElement;
    const checkbox = element.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
    checkbox.checked = true;
    checkbox.dispatchEvent(new Event('change'));
    expect(TestBed.inject(NoticePreferencesService).isHidden('confirm.favorite.delete')).toBe(false);
    element.querySelector<HTMLButtonElement>('.confirmacion-confirmar')!.click();
    expect(confirmed).toHaveBeenCalledOnce();
    expect(TestBed.inject(NoticePreferencesService).isHidden('confirm.favorite.delete')).toBe(true);

    fixture.componentRef.setInput('open', false);
    fixture.detectChanges();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    await fixture.whenStable();
    expect(element.querySelector('[role="alertdialog"]')).toBeNull();
    expect(confirmed).toHaveBeenCalledTimes(2);
  });

  it('does not persist cancellation and resets the checkbox when reopened', () => {
    const fixture = create();
    const element = fixture.nativeElement as HTMLElement;
    const checkbox = element.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
    checkbox.checked = true;
    checkbox.dispatchEvent(new Event('change'));
    element.querySelector<HTMLButtonElement>('.confirmacion-cancelar')!.click();
    fixture.componentRef.setInput('open', false);
    fixture.detectChanges();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    expect(TestBed.inject(NoticePreferencesService).isHidden('confirm.favorite.delete')).toBe(false);
    expect(element.querySelector<HTMLInputElement>('input[type="checkbox"]')!.checked).toBe(false);
  });

  it('shows prompts again immediately after reset', async () => {
    const preferences = TestBed.inject(NoticePreferencesService);
    preferences.hide('confirm.favorite.delete');
    const fixture = create();
    await fixture.whenStable();
    preferences.reset();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it('does not execute a stale skipped confirmation after the dialog is reopened', async () => {
    TestBed.inject(NoticePreferencesService).hide('confirm.favorite.delete');
    const fixture = create();
    const confirmed = vi.fn();
    fixture.componentInstance.confirmed.subscribe(confirmed);
    fixture.componentRef.setInput('open', false);
    fixture.detectChanges();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    await fixture.whenStable();
    expect(confirmed).toHaveBeenCalledOnce();
  });
});
