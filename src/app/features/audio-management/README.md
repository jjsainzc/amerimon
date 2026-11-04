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

`audioLabel` is impure so existing labels and tooltips are translated again when the host language changes or dictionaries finish loading. For signal-based hosts, the supplied translator must read the language/dictionary revision signal to notify the view; Amerimon's adapter reads `TranslationService.revision()`. This keeps the feature independent of the host translation service.

## API contract

The feature expects:

- `GET {baseUrl}/tracks` returning `{ status, data: AudioTrack[] }`
- `POST {baseUrl}/tracks` with multipart fields `name`, `playback_mode`, `interval_minutes` (for silence mode), and `file`
- `PATCH {baseUrl}/tracks/{id}` with editable track fields
- `DELETE {baseUrl}/tracks/{id}`
- `POST {baseUrl}/tracks/{id}/play`
- `GET {baseUrl}/tracks/{id}/file` returning an authenticated WAV blob for local listening
- `GET {baseUrl}/recordings` returning `{ status, data: AudioRecording[] }`
- `POST {baseUrl}/recordings` with `{ name }`, confirmed after server-side PCM starts arriving
- `PATCH {baseUrl}/recordings/{id}` with `{ action: 'pause' | 'resume' | 'stop' }`
- `GET {baseUrl}/recordings/{id}/file` returning an authenticated WAV blob
- `DELETE {baseUrl}/recordings/{id}`
- `POST {baseUrl}/recordings/{id}/library` creating an independent track copy without broadcasting it

The host remains responsible for authentication/authorization, API interceptors, and navigation. The backend accepts 8-bit or 16-bit, 8 kHz mono PCM WAV and caps uploads at 25 MiB. It converts 8-bit audio to 16-bit PCM because that is the format expected by Asterisk's WAV playback.

The component uses translation keys for every label, confirmation, success notice, and error. The host can provide `AUDIO_MANAGEMENT_TRANSLATE` directly or pass a translator to `provideAudioManagement`. Backend errors include a stable `code` (`AUDIO_ERROR_*`) as well as a diagnostic `message`; the component displays the translated code and never assumes that backend text matches the host application's language.

The library intentionally shows disabled tracks so they can be activated later. Supported playback modes are `broadcast_once` and `broadcast_silence`; change a track's mode and interval in its library card, then save the changes explicitly. A due `broadcast_silence` track remains pending while any monitored node is active and plays once all monitored nodes are silent. Scheduled playback is serialized using a persistent reservation based on the verified WAV duration. Deleting a track is explicit and removes its file and schedule. Existing `on_connection` tracks are migrated to `broadcast_once` and remain available for administrator-triggered playback.

Foreground requests share a service-scoped busy state across the library and recordings. While an action is pending, other action buttons and editable fields are disabled. Progress text (including upload percentage) appears only inside the initiating button with `aria-busy`; errors and completion notices remain separate. Background recording polling does not lock the screen, and a recording in progress does not prevent its pause/stop controls from being used.

Action buttons use inline SVG icons with translated tooltips and visually hidden accessible labels. Pending actions show a spinner and their process text inside the button. The headphones button previews any library track locally, including disabled or scheduled tracks, using the reusable player and signal meter. It only downloads the authenticated WAV and never sends an AMI command or changes a schedule. The distinct broadcast icon retains manual Asterisk playback for enabled one-time tracks. Preview URLs are revoked when changing tracks, deleting the previewed track, or leaving the screen.

## Server-side node recordings

`AudioRecordingsComponent` is included in the main screen and can also be imported separately with the same providers. It captures the primary node listening feed, not the browser microphone. The backend reuses the EAGI listening pipeline, records without a browser WebSocket, and persists recordings in a private directory beside SQLite. Start, manual pause, resume, stop, browser-only playback, confirmed deletion, and copying into the broadcast library are supported. Copying does not transmit audio and deleting a recording does not delete its library copy.

Automatic silence removal uses approximately −40 dBFS PCM RMS, a 200 ms pre-roll, and a 500 ms release margin. Manual pause has priority over automatic resumption. The server caps each recording at 25 MiB and finalizes when the limit is reached. Closing the component only stops polling and revokes browser blob URLs; it does not stop capture. If EAGI disconnects, recording is interrupted; after a service restart, persisted PCM is recovered into a playable WAV without automatically resuming capture.

All recording requests must use the host's authenticated HTTP client/interceptors. Playback downloads a blob rather than embedding an unauthenticated server URL. The child notifies `AudioManagementService.libraryChanged` after a copy, refreshing the library; polling is disposed on component destruction. Supply translations for `AUDIO_RECORD_*` as well as the existing audio keys.

### Signal meters

Active recordings expose `level_dbfs` in the listing API: PCM RMS in dBFS, floored at -60, with a 500 ms maximum hold and a one-second stale timeout. It measures the incoming feed even during manual pause, not the retained WAV. The screen polls every 500 ms while recording (every two seconds when idle); levels are transient and require no database migration.

Browser playback of saved recordings uses `AudioPlayerComponent` and Web Audio RMS sampling every 50 ms. It does not represent Asterisk's broadcast output or the physical speaker volume. Pause/end reset the meter, and destroying the player closes its audio context. `AudioLevelMeterComponent` can also be reused separately with `dbfs` and `label` inputs. Both components are exported and require `AUDIO_LEVEL_*` translations. No microphone permission or additional dependency is needed.
