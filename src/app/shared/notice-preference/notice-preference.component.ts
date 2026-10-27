import { Component, input, output } from '@angular/core';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

@Component({
  selector: 'app-notice-preference',
  standalone: true,
  imports: [TranslatePipe],
  template: `
    <label>
      <input #checkbox type="checkbox" [checked]="checked()" [disabled]="disabled()"
        (change)="checkedChange.emit(checkbox.checked)" />
      {{ 'NOTICE_DO_NOT_SHOW_AGAIN' | translate }}
    </label>
  `,
  styles: `
    :host { display: block; margin-block: .65rem; }
    label { display: flex; align-items: flex-start; gap: .5rem; font-size: .85rem; cursor: pointer; }
    input { flex: 0 0 auto; margin: .15rem 0 0; width: auto; }
  `,
})
export class NoticePreferenceComponent {
  readonly checked = input(false);
  readonly disabled = input(false);
  readonly checkedChange = output<boolean>();
}
