import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { vi } from 'vitest';
import { TranslationService } from '../../core/services/translation.service';
import { NetCalculatorPopupComponent } from './net-calculator-popup.component';

describe('NetCalculatorPopupComponent', () => {
  let component: NetCalculatorPopupComponent;
  let fixture: ComponentFixture<NetCalculatorPopupComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [NetCalculatorPopupComponent],
      providers: [{
        provide: TranslationService,
        useValue: { currentLang: signal('es'), revision: signal(0), translate: (key: string) => key },
      }],
    }).compileComponents();
  });

  beforeEach(() => {
    fixture = TestBed.createComponent(NetCalculatorPopupComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('copies the edited IP and each calculated address without applying or closing', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      const close = vi.fn();
      const select = vi.fn();
      component.closeModal.subscribe(close);
      component.selectNetwork.subscribe(select);
      fixture.componentRef.setInput('isOpen', true);
      fixture.detectChanges();
      const input: HTMLInputElement = fixture.nativeElement.querySelector('#ip-input');
      input.value = '192.0.2.37';
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      fixture.nativeElement.querySelector('.clipboard-copy').click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('192.0.2.37');
      component.calculateSubnet();
      fixture.detectChanges();
      const buttons: NodeListOf<HTMLButtonElement> = fixture.nativeElement.querySelectorAll('.fav-lista .clipboard-copy');
      expect(buttons.length).toBe(3);
      for (const [i, expected] of ['192.0.2.0', '255.255.255.0', '192.0.2.255'].entries()) {
        buttons[i].click();
        await fixture.whenStable();
        expect(writeText).toHaveBeenLastCalledWith(expected);
      }
      expect(close).not.toHaveBeenCalled();
      expect(select).not.toHaveBeenCalled();
    } finally {
      Reflect.deleteProperty(navigator, 'clipboard');
    }
  });
});
