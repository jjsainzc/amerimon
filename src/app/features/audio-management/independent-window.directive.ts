import { DOCUMENT } from '@angular/common';
import { Directive, HostListener, inject, input, output } from '@angular/core';

@Directive({
  selector: '[independentWindow]',
  standalone: true,
})
export class IndependentWindowDirective {
  readonly independentWindow = input.required<string>();
  readonly windowBlocked = output<void>();
  private readonly document = inject(DOCUMENT);

  @HostListener('click')
  open(): void {
    const url = new URL(this.independentWindow(), this.document.baseURI);
    if (url.origin !== this.document.location.origin) {
      this.windowBlocked.emit();
      return;
    }
    const popup = this.document.defaultView?.open('about:blank', '_blank', 'popup,width=1200,height=850,resizable=yes,scrollbars=yes');
    if (!popup) {
      this.windowBlocked.emit();
      return;
    }
    popup.opener = null;
    popup.location.replace(url.href);
  }
}
