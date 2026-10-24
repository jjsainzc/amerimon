import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { CopyValueDirective } from './copy-value.directive';

@Component({
  imports: [CopyValueDirective],
  template: `<form (submit)="submitted = true">
    <span class="clipboard-field" [appCopy]="value()" (click)="selected = true">
      <input [value]="value()" (input)="value.set($any($event.target).value)">
    </span>
    <span [appCopy]="value()"><a href="/example">Nodo</a></span>
  </form>`,
})
class TestHost {
  readonly value = signal('616262');
  submitted = false;
  selected = false;
}

describe('CopyValueDirective', () => {
  const writeText = vi.fn<Clipboard['writeText']>();

  beforeEach(() => {
    writeText.mockReset().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    Reflect.deleteProperty(navigator, 'clipboard');
  });

  function setup() {
    const fixture = TestBed.createComponent(TestHost);
    fixture.detectChanges();
    const element: HTMLElement = fixture.nativeElement;
    const button = element.querySelector<HTMLButtonElement>('.clipboard-copy')!;
    return { fixture, element, button };
  }

  it('copies the latest edited value, reports success and does not submit or select', async () => {
    const { fixture, element, button } = setup();
    const input = element.querySelector('input')!;
    input.value = '700001';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button.click();
    await fixture.whenStable();
    expect(writeText).toHaveBeenCalledWith('700001');
    expect(element.querySelector('[role="status"]')!.textContent).toBe('Copiado');
    expect(button.type).toBe('button');
    expect(fixture.componentInstance.submitted).toBe(false);
    expect(fixture.componentInstance.selected).toBe(false);
    expect(element.querySelector('a button')).toBeNull();
  });

  it('keeps an explicit error when permission is denied', async () => {
    writeText.mockRejectedValue(new DOMException('denied', 'NotAllowedError'));
    const { fixture, element, button } = setup();
    button.click();
    await fixture.whenStable();
    expect(element.textContent).toContain('No se pudo copiar');
    expect(element.textContent).not.toContain('Copiado');
  });

  it('supports keyboard-generated activation through a labelled native button', async () => {
    const { fixture, button } = setup();
    expect(button.getAttribute('aria-label')).toBe('Copiar 616262 al portapapeles');
    button.focus();
    expect(document.activeElement).toBe(button);
    button.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 0 }));
    await fixture.whenStable();
    expect(writeText).toHaveBeenCalledWith('616262');
    expect(fixture.componentInstance.submitted).toBe(false);
  });

  it('does not show stale success if the field changes during an asynchronous copy', async () => {
    let resolveCopy!: () => void;
    writeText.mockReturnValue(new Promise<void>(resolve => { resolveCopy = resolve; }));
    const { fixture, element, button } = setup();
    button.click();
    fixture.componentInstance.value.set('700002');
    fixture.detectChanges();
    resolveCopy();
    await fixture.whenStable();
    expect(element.querySelector('[role="status"]')!.textContent).toBe('');
  });

  it('disables empty values and clears stale feedback when the value changes', async () => {
    const { fixture, element, button } = setup();
    button.click();
    await fixture.whenStable();
    fixture.componentInstance.value.set(' ');
    fixture.detectChanges();
    expect(button.disabled).toBe(true);
    expect(element.querySelector('[role="status"]')!.textContent).toBe('');
    button.click();
    expect(writeText).toHaveBeenCalledTimes(1);
  });

  it('copies in HTTP without Clipboard API and restores the focused field', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    const execCommand = vi.fn(() => true);
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand });
    try {
      const { fixture, element, button } = setup();
      const input = element.querySelector('input')!;
      input.focus();
      input.setSelectionRange(1, 3);
      button.click();
      await fixture.whenStable();
      expect(execCommand).toHaveBeenCalledWith('copy');
      expect(document.activeElement).toBe(input);
      expect(input.selectionStart).toBe(1);
      expect(input.selectionEnd).toBe(3);
      expect(document.querySelector('textarea')).toBeNull();
      expect(element.textContent).toContain('Copiado');
    } finally {
      Reflect.deleteProperty(document, 'execCommand');
    }
  });

  it('reports unsuccessful legacy copy and removes its temporary element', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn(() => false) });
    try {
      const { fixture, element, button } = setup();
      button.click();
      await fixture.whenStable();
      expect(element.textContent).toContain('No se pudo copiar');
      expect(document.querySelector('textarea')).toBeNull();
    } finally {
      Reflect.deleteProperty(document, 'execCommand');
    }
  });
});
