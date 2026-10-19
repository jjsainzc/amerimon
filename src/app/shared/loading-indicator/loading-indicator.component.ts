import { Component, input } from '@angular/core';

@Component({
  selector: 'app-loading-indicator',
  standalone: true,
  templateUrl: './loading-indicator.component.html',
  styleUrl: './loading-indicator.component.css'
})
export class LoadingIndicatorComponent {
  readonly message = input.required<string>();
  readonly layout = input<'centered' | 'inline' | 'compact'>('centered');
}
