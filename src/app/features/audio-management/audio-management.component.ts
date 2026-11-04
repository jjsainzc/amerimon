import { HttpErrorResponse, HttpEventType } from '@angular/common/http';
import { Component, DestroyRef, ElementRef, OnDestroy, ViewChild, inject, signal } from '@angular/core';
import { Subscription, from, map, of, switchMap, tap } from 'rxjs';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DatePipe, DecimalPipe } from '@angular/common';
import { AudioManagementLabelPipe } from './audio-management-label.pipe';
import { AudioPlaybackMode, AudioTrack } from './audio-management.model';
import { AudioManagementService } from './audio-management.service';
import { AudioRecordingsComponent } from './audio-recordings.component';
import { AudioPlayerComponent } from './audio-player.component';
import { AudioIconButtonComponent } from './audio-icon-button.component';
import { IndependentWindowDirective } from './independent-window.directive';
import { AUDIO_MANAGEMENT_MONITOR_URL } from './audio-management.tokens';
import { downloadAudioBlob } from './audio-download';
import { audioSegmentBlob } from './audio-segment';

interface AudioTrackDraft {
  playbackMode: AudioPlaybackMode;
  intervalMinutes: number;
}

@Component({
  selector: 'app-audio-management',
  standalone: true,
  imports: [AudioManagementLabelPipe, DatePipe, DecimalPipe, FormsModule, AudioRecordingsComponent, AudioPlayerComponent, AudioIconButtonComponent, IndependentWindowDirective],
  templateUrl: './audio-management.component.html',
  styleUrl: './audio-management.component.css',
})
export class AudioManagementComponent implements OnDestroy {
  private readonly service = inject(AudioManagementService);
  private readonly destroyRef = inject(DestroyRef);
  readonly monitorUrl = inject(AUDIO_MANAGEMENT_MONITOR_URL);
  @ViewChild('audioFileInput') private audioFileInput?: ElementRef<HTMLInputElement>;

  readonly tracks = signal<AudioTrack[]>([]);
  readonly drafts = signal<Record<number, AudioTrackDraft>>({});
  readonly loading = signal(false);
  readonly busy = this.service.busy;
  readonly togglingId = signal<number | null>(null);
  readonly saving = signal(false);
  readonly savingTrackId = signal<number | null>(null);
  readonly uploadProgress = signal<number | null>(null);
  readonly playingId = signal<number | null>(null);
  readonly pendingDelete = signal<AudioTrack | null>(null);
  readonly deleting = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly preview = signal<{ id: number; url: string } | null>(null);
  readonly previewLoadingId = signal<number | null>(null);
  readonly downloadingId = signal<number | null>(null);
  readonly pendingPlayback = signal<AudioTrack | null>(null);
  readonly savingSelection = signal(false);
  readonly selectionPreview = signal<{ url: string; start: number; end: number } | null>(null);
  readonly selectionLoading = signal(false);
  private selectionRequest?: Subscription;
  playbackComplete = true;
  playbackStartMS = 0;
  playbackEndMS = 0;
  private previewRequest?: Subscription;

  name = '';
  file: File | null = null;

  constructor() {
    this.load();
    this.service.libraryChanged.pipe(takeUntilDestroyed()).subscribe(() => this.fetchTracks());
  }

  load(): void {
    if (this.busy()) { return; }
    this.fetchTracks();
  }

  private fetchTracks(): void {
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
        this.savingTrackId.set(null);
        this.togglingId.set(null);
      },
      error: error => this.fail(error, 'AUDIO_LOAD_ERROR'),
    });
  }

  selectFile(event: Event): void {
    if (this.busy()) { return; }
    const input = event.target as HTMLInputElement;
    this.file = input.files?.[0] ?? null;
  }

  upload(): void {
    if (!this.file || !this.name.trim() || this.busy()) {
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
    if (this.busy()) { return; }
    const select = event.target as HTMLSelectElement;
    const playbackMode = select.value;
    if (playbackMode !== 'broadcast_once' && playbackMode !== 'broadcast_silence') {
      return;
    }
    this.updateDraft(track, { playbackMode });
  }

  changeInterval(track: AudioTrack, event: Event): void {
    if (this.busy()) { return; }
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
    if (!this.canSaveSettings(track) || this.busy()) {
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
        this.notice.set('AUDIO_UPDATE_SUCCESS');
        this.fetchTracks();
      },
      error: error => {
        this.savingTrackId.set(null);
        this.fail(error, 'AUDIO_UPDATE_ERROR');
      },
    });
  }

  toggle(track: AudioTrack): void {
    if (this.busy() || this.hasUnsavedChanges(track)) { return; }
    this.togglingId.set(track.id);
    this.updateTrack(track, { enabled: !track.enabled });
  }

  play(track: AudioTrack): void {
    if (this.busy() || !track.enabled || track.playback_mode !== 'broadcast_once') { return; }
    this.editPlaybackSelection(track);
  }

  editPlaybackSelection(track: AudioTrack): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    this.error.set('');
    this.playbackComplete = !track.end_ms;
    this.playbackStartMS = track.start_ms ?? 0;
    this.playbackEndMS = track.end_ms || track.duration_ms;
    this.pendingPlayback.set(track);
  }

  resetPlaybackSelection(): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    this.playbackComplete = true;
    this.playbackStartMS = 0;
    this.playbackEndMS = this.pendingPlayback()?.duration_ms ?? 0;
  }

  selectionPreviewMatches(): boolean {
    const preview = this.selectionPreview();
    return !!preview && preview.start === (this.playbackComplete ? 0 : this.playbackStartMS) &&
      preview.end === (this.playbackComplete ? this.pendingPlayback()?.duration_ms : this.playbackEndMS);
  }

  listenSelection(): void {
    const track = this.pendingPlayback();
    if (!track || this.busy() || this.selectionLoading() || (!this.playbackComplete && !this.validPlaybackSegment())) { return; }
    this.clearSelectionPreview();
    this.clearPreview();
    const start = this.playbackComplete ? 0 : this.playbackStartMS;
    const end = this.playbackComplete ? track.duration_ms : this.playbackEndMS;
    const complete = this.playbackComplete;
    this.error.set('');
    this.selectionLoading.set(true);
    this.selectionRequest = this.service.trackFile(track.id).pipe(
      switchMap(blob => complete ? of(blob) : from(blob.arrayBuffer()).pipe(
        map(buffer => audioSegmentBlob(buffer, start, end)),
      )),
    ).subscribe({
      next: blob => {
        this.selectionLoading.set(false);
        this.selectionPreview.set({ url: URL.createObjectURL(blob), start, end });
      },
      error: error => {
        this.selectionLoading.set(false);
        console.error('Audio selection preview failed', error);
        this.fail(error, 'AUDIO_PREVIEW_ERROR');
      },
    });
  }

  closePlaybackSelection(): void {
    if (this.busy() || this.selectionLoading()) { return; }
    this.clearSelectionPreview();
    this.pendingPlayback.set(null);
  }

  private clearSelectionPreview(): void {
    this.selectionRequest?.unsubscribe();
    const preview = this.selectionPreview();
    if (preview) { URL.revokeObjectURL(preview.url); }
    this.selectionPreview.set(null);
    this.selectionLoading.set(false);
  }

  changePlaybackStart(value: number): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    this.playbackStartMS = Math.max(0, Math.min(value, this.playbackEndMS - 1));
  }

  changePlaybackEnd(value: number): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    this.playbackEndMS = Math.min(this.pendingPlayback()?.duration_ms ?? 0, Math.max(value, this.playbackStartMS + 1));
  }

  setPlaybackSeconds(bound: 'start' | 'end', value: number | null): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    const milliseconds = value !== null && Number.isFinite(value) ? Math.round(value * 1000) : -1;
    if (bound === 'start') {
      this.playbackStartMS = milliseconds;
    } else {
      this.playbackEndMS = milliseconds;
    }
  }

  changePlaybackComplete(complete: boolean): void {
    if (this.busy()) { return; }
    this.clearSelectionPreview();
    this.playbackComplete = complete;
  }

  validPlaybackSegment(): boolean {
    const track = this.pendingPlayback();
    return !!track && Number.isInteger(this.playbackStartMS) && Number.isInteger(this.playbackEndMS) &&
      this.playbackStartMS >= 0 && this.playbackEndMS > this.playbackStartMS && this.playbackEndMS <= track.duration_ms;
  }

  confirmPlayback(emit = true): void {
    const track = this.pendingPlayback();
    if (!track || this.busy() || (!this.playbackComplete && !this.validPlaybackSegment())) { return; }
    if (emit && (!track.enabled || track.playback_mode !== 'broadcast_once')) { return; }
    this.playingId.set(emit ? track.id : null);
    this.savingSelection.set(!emit);
    this.error.set('');
    this.notice.set('');
    const selection = this.playbackComplete ? { start_ms: 0, end_ms: 0 } : {
      start_ms: this.playbackStartMS, end_ms: this.playbackEndMS,
    };
    const changed = selection.start_ms !== (track.start_ms ?? 0) || selection.end_ms !== (track.end_ms ?? 0);
    let selectionSaved = !changed;
    const saved = changed ? this.service.update(track.id, selection).pipe(tap(() => {
      selectionSaved = true;
      this.pendingPlayback.set({ ...track, ...selection });
      this.tracks.update(tracks => tracks.map(item => item.id === track.id ? { ...item, ...selection } : item));
    })) : of(undefined);
    saved.pipe(switchMap(() => emit ? this.service.play(track.id) : of(undefined))).subscribe({
      next: () => {
        this.clearSelectionPreview();
        this.pendingPlayback.set(null);
        this.notice.set(emit ? 'AUDIO_PLAY_REQUESTED' : 'AUDIO_UPDATE_SUCCESS');
        this.playingId.set(null);
        this.savingSelection.set(false);
      },
      error: error => {
        this.savingSelection.set(false);
        this.fail(error, selectionSaved && emit ? 'AUDIO_PLAY_ERROR' : 'AUDIO_UPDATE_ERROR');
      },
    });
  }

  listen(track: AudioTrack): void {
    if (this.busy()) { return; }
    this.clearPreview();
    this.error.set('');
    this.notice.set('');
    this.previewLoadingId.set(track.id);
    this.previewRequest = this.service.trackFile(track.id).subscribe({
      next: blob => {
        this.previewLoadingId.set(null);
        this.preview.set({ id: track.id, url: URL.createObjectURL(blob) });
      },
      error: error => {
        this.previewLoadingId.set(null);
        this.fail(error, 'AUDIO_PREVIEW_ERROR');
      },
    });
  }

  private clearPreview(): void {
    this.previewRequest?.unsubscribe();
    const preview = this.preview();
    if (preview) { URL.revokeObjectURL(preview.url); }
    this.preview.set(null);
  }

  download(track: AudioTrack): void {
    if (this.busy()) { return; }
    this.error.set('');
    this.downloadingId.set(track.id);
    this.service.trackFile(track.id).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: blob => {
        downloadAudioBlob(blob, track.name);
        this.downloadingId.set(null);
      },
      error: error => {
        this.downloadingId.set(null);
        this.fail(error, 'AUDIO_DOWNLOAD_ERROR');
      },
    });
  }

  ngOnDestroy(): void {
    this.clearSelectionPreview();
    this.clearPreview();
  }

  remove(track: AudioTrack): void {
    if (this.busy()) { return; }
    this.error.set('');
    this.pendingDelete.set(track);
  }

  cancelDelete(): void {
    if (!this.busy()) {
      this.pendingDelete.set(null);
    }
  }

  confirmDelete(): void {
    const track = this.pendingDelete();
    if (!track || this.busy()) {
      return;
    }
    this.error.set('');
    this.deleting.set(true);
    this.service.delete(track.id).subscribe({
      next: () => {
        if (this.preview()?.id === track.id) { this.clearPreview(); }
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
        this.fetchTracks();
      },
      error: error => {
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
    this.togglingId.set(null);
  }

  private resetFileInput(): void {
    if (this.audioFileInput) {
      this.audioFileInput.nativeElement.value = '';
    }
  }
}
