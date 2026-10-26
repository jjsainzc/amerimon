import { DOCUMENT } from '@angular/common';
import { Component, EventEmitter, Input, Output, inject } from '@angular/core';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

@Component({
  selector: 'app-print-button',
  standalone: true,
  imports: [TranslatePipe],
  templateUrl: './print-button.component.html',
  styleUrl: './print-button.component.css'
})
export class PrintButtonComponent {
  @Input() label = '';
  @Input() disabled = false;
  @Output() readonly beforePrint = new EventEmitter<void>();

  private readonly document = inject(DOCUMENT);

  /** Notifica la preparación del reporte y abre la impresión en el siguiente turno del navegador. */
  imprimir(): void {
    const windowRef = this.document.defaultView;
    if (this.disabled || !windowRef) { return; }

    this.beforePrint.emit();
    windowRef.setTimeout(() => windowRef.print(), 0);
  }
}
