import { Pipe, PipeTransform, inject } from '@angular/core';
import { AUDIO_MANAGEMENT_TRANSLATE } from './audio-management.i18n';

@Pipe({
  name: 'audioLabel',
  standalone: true,
})
export class AudioManagementLabelPipe implements PipeTransform {
  private readonly translate = inject(AUDIO_MANAGEMENT_TRANSLATE);

  transform(key: string): string {
    return this.translate(key);
  }
}
