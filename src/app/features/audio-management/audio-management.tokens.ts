import { InjectionToken } from '@angular/core';

export const AUDIO_MANAGEMENT_API_URL = new InjectionToken<string>('AUDIO_MANAGEMENT_API_URL');
export const AUDIO_MANAGEMENT_MONITOR_URL = new InjectionToken<string | null>('AUDIO_MANAGEMENT_MONITOR_URL', {
  factory: () => null,
});
