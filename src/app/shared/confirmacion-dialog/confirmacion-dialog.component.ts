import { Component, DestroyRef, EventEmitter, Input, OnChanges, Output, SimpleChanges, inject } from '@angular/core';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';
import { NoticePreferenceComponent } from '../notice-preference/notice-preference.component';

@Component({
  selector: 'app-confirmacion-dialog',
  standalone: true,
  imports: [TranslatePipe, NoticePreferenceComponent],
  templateUrl: './confirmacion-dialog.component.html',
  styleUrl: './confirmacion-dialog.component.css'
})
export class ConfirmacionDialogComponent implements OnChanges {
  protected readonly preferences = inject(NoticePreferencesService);
  private readonly destroyRef = inject(DestroyRef);
  private generation = 0;
  @Input() noticeKey = '';
  protected doNotShow = false;
  @Input() open = false;
  @Input() title = '';
  @Input() message = '';
  @Input() confirmLabel = '';
  @Input() showDeleteIcon = false;
  @Input() busy = false;
  @Output() confirmed = new EventEmitter<void>();
  @Output() cancelled = new EventEmitter<void>();

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['open'] || changes['noticeKey']) {
      const generation = ++this.generation;
      this.doNotShow = false;
      if (this.open && !this.busy && this.preferences.isHidden(this.noticeKey)) {
        const key = this.noticeKey;
        queueMicrotask(() => {
          if (!this.destroyRef.destroyed && this.generation === generation && this.open && !this.busy && this.noticeKey === key &&
              this.preferences.isHidden(key)) {
            this.confirmed.emit();
          }
        });
      }
    }
  }

  protected confirm(): void {
    if (this.busy) { return; }
    if (this.doNotShow) { this.preferences.hide(this.noticeKey); }
    this.confirmed.emit();
  }
}
