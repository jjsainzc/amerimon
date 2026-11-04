import { HttpErrorResponse, HttpEventType } from '@angular/common/http';
import { Component, ElementRef, ViewChild, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe, DecimalPipe } from '@angular/common';
import { AudioManagementLabelPipe } from './audio-management-label.pipe';
import { AudioPlaybackMode, AudioTrack } from './audio-management.model';
import { AudioManagementService } from './audio-management.service';

interface AudioTrackDraft {
  playbackMode: AudioPlaybackMode;
  intervalMinutes: number;
}

@Component({
  selector: 'app-audio-management',
  standalone: true,
  imports: [AudioManagementLabelPipe, DatePipe, DecimalPipe, FormsModule],
  templateUrl: './audio-management.component.html',
  styleUrl: './audio-management.component.css',
})
export class AudioManagementComponent {
  private readonly service = inject(AudioManagementService);
  @ViewChild('audioFileInput') private audioFileInput?: ElementRef<HTMLInputElement>;

  readonly tracks = signal<AudioTrack[]>([]);
  readonly drafts = signal<Record<number, AudioTrackDraft>>({});
  readonly loading = signal(false);
  readonly saving = signal(false);
  readonly savingTrackId = signal<number | null>(null);
  readonly uploadProgress = signal<number | null>(null);
  readonly playingId = signal<number | null>(null);
  readonly pendingDelete = signal<AudioTrack | null>(null);
  readonly deleting = signal(false);
  readonly error = signal('');
  readonly notice = signal('');

  name = '';
  file: File | null = null;

  constructor() {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.error.set('');
    this.service.list().subscribe({
      next: tracks => {
        this.tracks.set(tracks);
        this.drafts.set(Object.fromEntries(tracks.map(track => [
          track.id,
          {
            playbackMode: track.playback_mode,
            intervalMinutes: track.interval_minutes || 60,
          },
        ])));
        this.loading.set(false);
      },
      error: error => this.fail(error, 'AUDIO_LOAD_ERROR'),
    });
  }

  selectFile(event: Event): void {
    const input = event.target as HTMLInputElement;
    this.file = input.files?.[0] ?? null;
  }

  upload(): void {
    if (!this.file || !this.name.trim() || this.saving()) {
      return;
    }
    this.saving.set(true);
    this.error.set('');
    this.notice.set('');
    this.uploadProgress.set(null);
    this.service.upload({
      name: this.name.trim(),
      playbackMode: 'broadcast_once',
      intervalMinutes: 60,
      file: this.file,
    }).subscribe({
      next: event => {
        if (event.type === HttpEventType.UploadProgress) {
          if (event.total) {
            this.uploadProgress.set(Math.round((event.loaded / event.total) * 100));
          }
          return;
        }
        if (event.type !== HttpEventType.Response || !event.body?.data) {
          return;
        }
        const track = event.body.data;
        this.tracks.update(tracks => [track, ...tracks]);
        this.name = '';
        this.file = null;
        this.resetFileInput();
        this.notice.set('AUDIO_UPLOAD_SUCCESS');
        this.saving.set(false);
        this.uploadProgress.set(null);
      },
      error: error => {
        this.uploadProgress.set(null);
        this.fail(error, 'AUDIO_UPLOAD_ERROR');
      },
    });
  }

  changeMode(track: AudioTrack, event: Event): void {
    const select = event.target as HTMLSelectElement;
    const playbackMode = select.value;
    if (playbackMode !== 'broadcast_once' && playbackMode !== 'broadcast_silence') {
      return;
    }
    this.updateDraft(track, { playbackMode });
  }

  changeInterval(track: AudioTrack, event: Event): void {
    const input = event.target as HTMLInputElement;
    this.updateDraft(track, { intervalMinutes: Number(input.value) });
  }

  draftFor(track: AudioTrack): AudioTrackDraft {
    return this.drafts()[track.id] ?? {
      playbackMode: track.playback_mode,
      intervalMinutes: track.interval_minutes || 60,
    };
  }

  hasUnsavedChanges(track: AudioTrack): boolean {
    const draft = this.draftFor(track);
    return draft.playbackMode !== track.playback_mode ||
      (draft.playbackMode === 'broadcast_silence' && draft.intervalMinutes !== (track.interval_minutes || 60));
  }

  canSaveSettings(track: AudioTrack): boolean {
    const draft = this.draftFor(track);
    return this.hasUnsavedChanges(track) &&
      (draft.playbackMode !== 'broadcast_silence' ||
        (Number.isInteger(draft.intervalMinutes) && draft.intervalMinutes >= 1 && draft.intervalMinutes <= 10080));
  }

  saveSettings(track: AudioTrack): void {
    if (!this.canSaveSettings(track) || this.savingTrackId() !== null) {
      return;
    }
    const draft = this.draftFor(track);
    this.savingTrackId.set(track.id);
    this.error.set('');
    this.notice.set('');
    this.service.update(track.id, {
      playback_mode: draft.playbackMode,
      ...(draft.playbackMode === 'broadcast_silence' ? { interval_minutes: draft.intervalMinutes } : {}),
    }).subscribe({
      next: () => {
        this.savingTrackId.set(null);
        this.notice.set('AUDIO_UPDATE_SUCCESS');
        this.load();
      },
      error: error => {
        this.savingTrackId.set(null);
        this.fail(error, 'AUDIO_UPDATE_ERROR');
      },
    });
  }

  toggle(track: AudioTrack): void {
    this.updateTrack(track, { enabled: !track.enabled });
  }

  play(track: AudioTrack): void {
    this.playingId.set(track.id);
    this.error.set('');
    this.notice.set('');
    this.service.play(track.id).subscribe({
      next: () => {
        this.notice.set('AUDIO_PLAY_REQUESTED');
        this.playingId.set(null);
      },
      error: error => this.fail(error, 'AUDIO_PLAY_ERROR'),
    });
  }

  remove(track: AudioTrack): void {
    this.error.set('');
    this.pendingDelete.set(track);
  }

  cancelDelete(): void {
    if (!this.deleting()) {
      this.pendingDelete.set(null);
    }
  }

  confirmDelete(): void {
    const track = this.pendingDelete();
    if (!track || this.deleting()) {
      return;
    }
    this.error.set('');
    this.deleting.set(true);
    this.service.delete(track.id).subscribe({
      next: () => {
        this.tracks.update(tracks => tracks.filter(item => item.id !== track.id));
        this.notice.set('AUDIO_DELETE_SUCCESS');
        this.pendingDelete.set(null);
        this.deleting.set(false);
      },
      error: error => {
        this.deleting.set(false);
        this.fail(error, 'AUDIO_DELETE_ERROR');
      },
    });
  }

  modeLabel(mode: AudioPlaybackMode): string {
    switch (mode) {
      case 'broadcast_once': return 'AUDIO_MODE_ONCE';
      case 'broadcast_silence': return 'AUDIO_MODE_SILENCE';
    }
  }

  private updateTrack(track: AudioTrack, patch: Partial<AudioTrack>): void {
    this.error.set('');
    this.notice.set('');
    this.service.update(track.id, patch).subscribe({
      next: () => {
        this.notice.set('AUDIO_UPDATE_SUCCESS');
        this.load();
      },
      error: error => {
        this.load();
        this.fail(error, 'AUDIO_UPDATE_ERROR');
      },
    });
  }

  private updateDraft(track: AudioTrack, patch: Partial<AudioTrackDraft>): void {
    this.drafts.update(drafts => ({
      ...drafts,
      [track.id]: { ...this.draftFor(track), ...patch },
    }));
    this.error.set('');
    this.notice.set('');
  }

  private fail(error: unknown, fallback: string): void {
    const code = error instanceof HttpErrorResponse ? error.error?.code : undefined;
    this.error.set(typeof code === 'string' && code.startsWith('AUDIO_ERROR_') ? code : fallback);
    this.loading.set(false);
    this.saving.set(false);
    this.deleting.set(false);
    this.playingId.set(null);
    this.savingTrackId.set(null);
  }

  private resetFileInput(): void {
    if (this.audioFileInput) {
      this.audioFileInput.nativeElement.value = '';
    }
  }
}
