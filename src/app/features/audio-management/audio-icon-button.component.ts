import { AfterContentChecked, Component, ElementRef, inject, input } from '@angular/core';

const icons = {
  refresh: 'M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 6M4 12l2 6a7 7 0 0 0 12-1',
  upload: 'M12 16V3m-5 5 5-5 5 5M4 16v5h16v-5',
  save: 'M4 3h13l4 4v14H3V3h1m3 0v7h10V3M7 21v-7h10v7',
  enable: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M8 12l3 3 5-6',
  disable: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M8 12h8',
  listen: 'M4 14v-3a8 8 0 0 1 16 0v3M4 12H2v8h4v-8H4m16 0h2v8h-4v-8h2',
  broadcast: 'M5 8a10 10 0 0 0 0 8M2 5a14 14 0 0 0 0 14M19 8a10 10 0 0 1 0 8m3-11a14 14 0 0 1 0 14M9 7l7 5-7 5V7',
  delete: 'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7',
  record: 'M12 4a8 8 0 1 0 0 16 8 8 0 0 0 0-16M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8',
  pause: 'M7 4h3v16H7V4m7 0h3v16h-3V4',
  resume: 'M6 3l15 9L6 21V3',
  stop: 'M5 5h14v14H5V5',
  library: 'M4 3v18m5-18v18m5-18v18m3-17 4 16M8 12h8m-4-4v8',
  cancel: 'M5 5l14 14M5 19 19 5',
} as const;

@Component({
  selector: 'button[audioIcon]',
  standalone: true,
  template: `
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path class="action-icon" [attr.d]="icons[audioIcon()]" />
      <path class="spinner" d="M12 3a9 9 0 1 1-9 9" />
    </svg>
    <span class="label"><ng-content /></span>
  `,
  styles: `
    :host { display: inline-flex; align-items: center; justify-content: center; gap: .4rem; min-width: 2.5rem; min-height: 2.5rem; }
    svg { width: 1.15rem; height: 1.15rem; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; flex-shrink: 0; }
    .label { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .spinner { display: none; }
    :host([aria-busy="true"]) .label { position: static; width: auto; height: auto; margin: 0; clip-path: none; white-space: normal; }
    :host([aria-busy="true"]) .action-icon { display: none; }
    :host([aria-busy="true"]) .spinner { display: block; }
    :host([aria-busy="true"]) svg { animation: audio-spin 1s linear infinite; }
    @keyframes audio-spin { to { transform: rotate(360deg); } }
    @media (prefers-reduced-motion: reduce) { :host([aria-busy="true"]) svg { animation: none; } }
  `,
})
export class AudioIconButtonComponent implements AfterContentChecked {
  readonly audioIcon = input.required<keyof typeof icons>();
  protected readonly icons = icons;
  private readonly element = inject<ElementRef<HTMLButtonElement>>(ElementRef);

  ngAfterContentChecked(): void {
    this.element.nativeElement.title = this.element.nativeElement.textContent?.trim() ?? '';
  }
}
