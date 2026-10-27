import { Component, inject, input } from '@angular/core';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';
import { NoticePreferenceComponent } from '../notice-preference/notice-preference.component';

@Component({
  selector: 'app-dismissible-notice',
  standalone: true,
  imports: [NoticePreferenceComponent],
  template: `
    @if (alwaysShow() || !preferences.isHidden(noticeKey())) {
      <ng-content />
      @if (!alwaysShow()) {
        <app-notice-preference (checkedChange)="hide($event)" />
      }
    }
  `,
})
export class DismissibleNoticeComponent {
  protected readonly preferences = inject(NoticePreferencesService);
  readonly noticeKey = input.required<string>();
  readonly alwaysShow = input(false);

  protected hide(checked: boolean): void {
    if (checked && !this.alwaysShow()) { this.preferences.hide(this.noticeKey()); }
  }
}
