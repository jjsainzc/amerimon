import { Component, OnInit, OnDestroy, signal, inject, NgZone } from '@angular/core';

/**
 * Botón flotante reutilizable para volver al inicio.
 * Aparece al hacer scroll hacia abajo. Usa signals (modo zoneless).
 *
 * Por defecto registra el listener de 'scroll' en FASE DE CAPTURA a nivel de
 * documento, lo que detecta el scroll tanto de la ventana como de cualquier
 * contenedor interno (el evento scroll de elementos no burbujea, pero sí se
 * captura).
 *
 * Para contenido embebido en un iframe (mismo origen), el scroll ocurre en un
 * documento distinto, por lo que el listener del documento anfitrión no lo
 * detecta. En ese caso llama a engancharIframeEl(iframe) desde el evento
 * (load) del iframe y el componente se enganchará a su scroll interno.
 *
 * Uso: <app-scroll-top></app-scroll-top>
 *      <app-scroll-top #st></app-scroll-top>
 *      <iframe (load)="st.engancharIframeEl(frameEl)"></iframe>
 */
@Component({
  selector: 'app-scroll-top',
  template: `
    <button
      class="scroll-top-btn"
      [class.visible]="visible()"
      (click)="irArriba()"
      aria-label="Ir al principio">
      <i class="fas fa-caret-up"></i>
    </button>
  `,
  styles: [`
    .scroll-top-btn {
      position: fixed;
      bottom: 32px;
      right: 32px;
      width: 48px;
      height: 48px;
      border-radius: 50%;
      background: linear-gradient(135deg, #0077cc, #00bfff);
      color: #ffffff;
      font-size: 1.1rem;
      border: none;
      cursor: pointer;
      box-shadow: 0 4px 16px rgba(0, 119, 204, 0.4);
      opacity: 0;
      transform: translateY(20px);
      transition: opacity 0.3s ease, transform 0.3s ease;
      z-index: 999;
      pointer-events: none;
    }
    .scroll-top-btn.visible {
      opacity: 1;
      transform: translateY(0);
      pointer-events: auto;
    }
    .scroll-top-btn:hover {
      background: linear-gradient(135deg, #005fa3, #0099dd);
      box-shadow: 0 6px 20px rgba(0, 119, 204, 0.6);
      transform: translateY(-3px);
    }
  `]
})
export class ScrollTopComponent implements OnInit, OnDestroy {
  readonly visible = signal<boolean>(false);

  private ultimoScroller: Element | Window = window;
  private readonly zone = inject(NgZone);
  private readonly handler = (event: Event) => this.onScrollEvent(event);

  // Referencia a la ventana del iframe actualmente enganchada y su listener,
  // para poder desengancharlos correctamente al cambiar o destruir.
  private iframeWin: Window | null = null;
  private readonly iframeHandler = () => this.onIframeScroll();

  ngOnInit(): void {
    // Fase de captura (true) para atrapar scroll de cualquier contenedor.
    document.addEventListener('scroll', this.handler, true);
  }

  ngOnDestroy(): void {
    document.removeEventListener('scroll', this.handler, true);
    this.engancharIframe(null);
  }

  onScrollEvent(event: Event): void {
    const target = event.target;
    let y = 0;
    if (target === document || target === document.documentElement || target === document.body) {
      this.ultimoScroller = window;
      y = window.scrollY || document.documentElement.scrollTop || document.body.scrollTop || 0;
    } else if (target instanceof Element) {
      this.ultimoScroller = target;
      y = target.scrollTop;
    }
    // Actualiza dentro de la zona para asegurar el refresco (aunque signals
    // ya lo hacen, esto cubre el listener registrado fuera de Angular).
    this.zone.run(() => this.visible.set(y > 300));
  }

  irArriba(): void {
    // Si estamos controlando un iframe, prioriza su scroll interno.
    if (this.iframeWin) {
      this.iframeWin.scrollTo({ top: 0, behavior: 'smooth' });
      return;
    }
    const s = this.ultimoScroller;
    if (s instanceof Window) {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    } else {
      s.scrollTo({ top: 0, behavior: 'smooth' });
    }
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  /**
   * Engancha el scroll interno de un iframe (mismo origen).
   * Llamar desde el evento (load) del iframe para que se reenganche cada vez
   * que termina de cargar un documento (incluido al cambiar de contenido).
   */
  engancharIframeEl(iframe: HTMLIFrameElement | null): void {
    // Fuerza el reenganche aunque sea el mismo contentWindow (nuevo documento).
    this.engancharIframe(null);
    this.engancharIframe(iframe?.contentWindow ?? null);
  }

  /** Engancha (o desengancha, si win es null) el listener de scroll del iframe. */
  private engancharIframe(win: Window | null): void {
    if (this.iframeWin === win) { return; }
    // Limpia el listener anterior.
    if (this.iframeWin) {
      this.iframeWin.removeEventListener('scroll', this.iframeHandler);
    }
    this.iframeWin = win;
    // Reinicia visibilidad al cambiar de documento embebido.
    this.zone.run(() => this.visible.set(false));
    if (win) {
      win.addEventListener('scroll', this.iframeHandler);
    }
  }

  private onIframeScroll(): void {
    const win = this.iframeWin;
    if (!win) { return; }
    const y = win.scrollY || win.document.documentElement.scrollTop || 0;
    this.zone.run(() => this.visible.set(y > 300));
  }
}
