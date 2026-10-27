import { AfterViewInit, DestroyRef, Directive, ElementRef, Renderer2, effect, inject, input } from '@angular/core';
import { TranslationService } from '../../core/services/translation.service';

@Directive({ selector: '[appCopy]', standalone: true })
export class CopyValueDirective implements AfterViewInit {
  readonly appCopy = input<string | null | undefined>('');
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly renderer = inject(Renderer2);
  private readonly destroyRef = inject(DestroyRef);
  private readonly translation = inject(TranslationService);
  private button?: HTMLButtonElement;
  private status?: HTMLSpanElement;
  private resetTimer?: ReturnType<typeof setTimeout>;
  private destroyed = false;

  constructor() {
    effect(() => {
      this.translation.currentLang();
      this.translation.revision();
      const value = this.appCopy()?.trim() ?? '';
      if (this.button) {
        this.button.disabled = !value;
        this.button.setAttribute('aria-label', this.translation.translate('SHARED_COPY_VALUE_LABEL', { value }));
      }
      this.clearStatus();
    });
    this.destroyRef.onDestroy(() => {
      this.destroyed = true;
      if (this.resetTimer) { clearTimeout(this.resetTimer); }
    });
  }

  ngAfterViewInit(): void {
    const button = this.renderer.createElement('button') as HTMLButtonElement;
    this.button = button;
    this.renderer.addClass(this.host.nativeElement, 'clipboard-value');
    this.renderer.addClass(button, 'clipboard-copy');
    button.type = 'button';
    button.title = this.translation.translate('SHARED_COPY_TITLE');
    button.setAttribute('aria-label', this.translation.translate('SHARED_COPY_VALUE_LABEL', {
      value: this.appCopy()?.trim() ?? ''
    }));
    button.disabled = !this.appCopy()?.trim();
    const icon = this.renderer.createElement('i');
    this.renderer.addClass(icon, 'fa-regular');
    this.renderer.addClass(icon, 'fa-copy');
    this.renderer.setAttribute(icon, 'aria-hidden', 'true');
    this.renderer.appendChild(button, icon);
    const status = this.renderer.createElement('span') as HTMLSpanElement;
    this.status = status;
    status.className = 'clipboard-status';
    status.setAttribute('role', 'status');
    status.setAttribute('aria-live', 'polite');
    this.renderer.appendChild(this.host.nativeElement, button);
    this.renderer.appendChild(this.host.nativeElement, status);
    const unlisten = this.renderer.listen(button, 'click', (event: MouseEvent) => {
      event.preventDefault();
      event.stopPropagation();
      void this.copy();
    });
    this.destroyRef.onDestroy(unlisten);
  }

  private clearStatus(): void {
    if (this.resetTimer) { clearTimeout(this.resetTimer); }
    if (this.status) { this.status.textContent = ''; }
    if (this.button) { this.button.title = this.translation.translate('SHARED_COPY_TITLE'); }
  }

  private async copy(): Promise<void> {
    const value = this.appCopy()?.trim();
    if (!value || !this.button) { return; }
    let message: string;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        this.copyLegacy(value);
      }
      message = this.translation.translate('SHARED_COPY_SUCCESS');
    } catch {
      message = this.translation.translate('SHARED_COPY_FAILURE');
    }
    if (this.destroyed || this.appCopy()?.trim() !== value) { return; }
    this.clearStatus();
    this.button.title = message;
    if (this.status) { this.status.textContent = message; }
    this.resetTimer = setTimeout(() => this.clearStatus(), 5000);
  }

  private copyLegacy(value: string): void {
    const active = document.activeElement;
    const input = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement ? active : null;
    const start = input?.selectionStart;
    const end = input?.selectionEnd;
    const direction = input?.selectionDirection ?? undefined;
    const selection = document.getSelection();
    const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, i) => selection.getRangeAt(i).cloneRange()) : [];
    const textarea = document.createElement('textarea');
    textarea.value = value;
    textarea.readOnly = true;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    try {
      textarea.select();
      if (!document.execCommand('copy')) { throw new Error('SHARED_CLIPBOARD_UNAVAILABLE'); }
    } finally {
      textarea.remove();
      if (active instanceof HTMLElement) { active.focus({ preventScroll: true }); }
      if (input && start != null && end != null) { input.setSelectionRange(start, end, direction); }
      if (selection) {
        selection.removeAllRanges();
        for (const range of ranges) { selection.addRange(range); }
      }
    }
  }
}
