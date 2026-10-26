import { Component, input, output } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

@Component({
  selector: 'app-about-modal',
  standalone: true,
  imports: [CommonModule, TranslatePipe],
  templateUrl: './about-modal.component.html',
  styleUrls: ['./about-modal.component.css']
})
export class AboutModalComponent {
  readonly isOpen = input<boolean>(false);
  readonly closed = output<void>();

  close(): void {
    this.closed.emit();
  }
}
