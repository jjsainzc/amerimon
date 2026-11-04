# Angular audio-management feature

This standalone feature provides an administrator-facing audio library without depending on Amerimon's router, environment, translation service, or authentication implementation. The host application supplies its API URL, translations, and route-level authorization. Track settings and enabled state are stored by the backend, not in the browser session: playback scheduling continues after the admin signs out or closes the page and remains active until an administrator disables or deletes the track. A one-time broadcast remains one playback by design.

## Install

Import `AudioManagementComponent` and call `provideAudioManagement` from the host application's route:

```ts
{
  path: 'admin/audio',
  canActivate: [hostAdminGuard],
  providers: [
    provideAudioManagement('/api/audio'),
    {
      provide: AUDIO_MANAGEMENT_TRANSLATE,
      useFactory: () => {
        const i18n = inject(HostTranslationService);
        return (key: string) => i18n.translate(key);
      },
    },
  ],
  loadComponent: () => import('./audio-management').then(m => m.AudioManagementComponent),
}
```

`provideAudioManagement` accepts the base URL for the audio endpoints (ending in `/audio` in this API). Its default translator returns the key; applications should provide `AUDIO_MANAGEMENT_TRANSLATE` and translations for the `AUDIO_*` keys in the component template.

## API contract

The feature expects:

- `GET {baseUrl}/tracks` returning `{ status, data: AudioTrack[] }`
- `POST {baseUrl}/tracks` with multipart fields `name`, `playback_mode`, `interval_minutes` (for silence mode), and `file`
- `PATCH {baseUrl}/tracks/{id}` with editable track fields
- `DELETE {baseUrl}/tracks/{id}`
- `POST {baseUrl}/tracks/{id}/play`

The host remains responsible for authentication/authorization, API interceptors, and navigation. The backend accepts 8-bit or 16-bit, 8 kHz mono PCM WAV and caps uploads at 25 MiB. It converts 8-bit audio to 16-bit PCM because that is the format expected by Asterisk's WAV playback.

The component uses translation keys for every label, confirmation, success notice, and error. The host can provide `AUDIO_MANAGEMENT_TRANSLATE` directly or pass a translator to `provideAudioManagement`. Backend errors include a stable `code` (`AUDIO_ERROR_*`) as well as a diagnostic `message`; the component displays the translated code and never assumes that backend text matches the host application's language.

The library intentionally shows disabled tracks so they can be activated later. Supported playback modes are `broadcast_once` and `broadcast_silence`; change a track's mode and interval in its library card, then save the changes explicitly. A due `broadcast_silence` track remains pending while any monitored node is active and plays once all monitored nodes are silent. Scheduled playback is serialized using a persistent reservation based on the verified WAV duration. Deleting a track is explicit and removes its file and schedule. Existing `on_connection` tracks are migrated to `broadcast_once` and remain available for administrator-triggered playback.
