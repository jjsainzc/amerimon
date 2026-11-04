import { InjectionToken } from '@angular/core';

export type AudioManagementTranslate = (key: string) => string;

export const AUDIO_MANAGEMENT_TRANSLATE = new InjectionToken<AudioManagementTranslate>(
  'AUDIO_MANAGEMENT_TRANSLATE',
  { factory: () => key => key },
);
