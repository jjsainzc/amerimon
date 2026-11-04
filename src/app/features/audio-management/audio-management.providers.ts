import { EnvironmentProviders, makeEnvironmentProviders } from '@angular/core';
import { AUDIO_MANAGEMENT_TRANSLATE, AudioManagementTranslate } from './audio-management.i18n';
import { AudioManagementService } from './audio-management.service';
import { AUDIO_MANAGEMENT_API_URL } from './audio-management.tokens';

export function provideAudioManagement(
  apiUrl: string,
  translate: AudioManagementTranslate = key => key,
): EnvironmentProviders {
  return makeEnvironmentProviders([
    { provide: AUDIO_MANAGEMENT_API_URL, useValue: apiUrl.replace(/\/$/, '') },
    { provide: AUDIO_MANAGEMENT_TRANSLATE, useValue: translate },
    AudioManagementService,
  ]);
}
