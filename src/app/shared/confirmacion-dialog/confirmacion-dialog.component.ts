import { Component, EventEmitter, HostListener, Input, Output } from '@angular/core';

@Component({
  selector: 'app-confirmacion-dialog',
  standalone: true,
  templateUrl: './confirmacion-dialog.component.html',
  styleUrl: './confirmacion-dialog.component.css'
})
export class ConfirmacionDialogComponent {
  @Input() open = false;
  @Input() title = 'Confirmar acción';
  @Input() message = '';
  @Input() confirmLabel = 'Confirmar';
  @Input() showDeleteIcon = false;
  @Input() busy = false;
  @Output() confirmed = new EventEmitter<void>();
  @Output() cancelled = new EventEmitter<void>();

  /** Cancela el diálogo abierto con Escape e impide la acción predeterminada de esa tecla. */
  @HostListener('document:keydown.escape', ['$event'])
  onEscape(event: Event): void {
    if (!this.open) { return; }
    event.preventDefault();
    this.cancelled.emit();
  }

  /** Cancela solo al pulsar directamente el fondo, no el contenido del diálogo. */
  onBackdropClick(event: MouseEvent): void {
    if (event.target === event.currentTarget) { this.cancelled.emit(); }
  }
}
