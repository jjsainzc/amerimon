import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnDestroy, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe, DecimalPipe } from '@angular/common';
import { Subscription, timer, exhaustMap, filter, finalize } from 'rxjs';
import { AudioManagementLabelPipe } from './audio-management-label.pipe';
import { AudioManagementService } from './audio-management.service';
import { AudioRecording } from './audio-management.model';
import { AudioLevelMeterComponent } from './audio-level-meter.component';
import { AudioPlayerComponent } from './audio-player.component';
import { AudioIconButtonComponent } from './audio-icon-button.component';

@Component({
  selector: 'app-audio-recordings',
  standalone: true,
  imports: [FormsModule, DatePipe, DecimalPipe, AudioManagementLabelPipe, AudioLevelMeterComponent, AudioPlayerComponent, AudioIconButtonComponent],
  templateUrl: './audio-recordings.component.html',
  styleUrl: './audio-management.component.css',
})
export class AudioRecordingsComponent implements OnDestroy {
  private readonly service = inject(AudioManagementService);
  private poll?: Subscription;
  private previewRequest?: Subscription;
  private destroyed = false;
  readonly recordings = signal<AudioRecording[]>([]);
  readonly busy = this.service.busy;
  readonly action = signal<{ type: string; id?: number } | null>(null);
  readonly refreshing = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly pendingDelete = signal<AudioRecording | null>(null);
  readonly preview = signal<{ id: number; url: string } | null>(null);
  name = '';

  constructor() {
    this.startPolling();
  }

  private startPolling(foreground = false): void {
    if (this.destroyed) { return; }
    this.poll?.unsubscribe();
    this.poll = timer(0, 500).pipe(
      filter(tick => this.hasActive() || tick % 4 === 0),
      exhaustMap(tick => this.service.listRecordings(foreground && tick === 0).pipe(
        finalize(() => this.refreshing.set(false)),
      )),
    ).subscribe({
      next: recordings => this.recordings.set(recordings),
      error: error => this.setError(error, 'AUDIO_RECORD_LOAD_ERROR'),
    });
  }

  refresh(): void {
    if (this.busy()) { return; }
    this.error.set('');
    this.startPolling(true);
    this.refreshing.set(true);
  }

  isActive(recording: AudioRecording): boolean {
    return ['starting', 'recording', 'paused'].includes(recording.state);
  }

  hasActive(): boolean {
    return this.recordings().some(recording => this.isActive(recording));
  }

  stateLabel(recording: AudioRecording): string {
    if (recording.state === 'recording' && recording.auto_paused) {
      return 'AUDIO_RECORD_WAITING';
    }
    return `AUDIO_RECORD_STATE_${recording.state.toUpperCase()}`;
  }

  start(): void {
    if (this.busy() || this.hasActive() || !this.name.trim()) { return; }
    this.begin('start');
    this.service.startRecording(this.name.trim()).subscribe({
      next: () => {
        this.name = '';
        this.complete('AUDIO_RECORD_STARTED');
      },
      error: error => {
        this.fail(error, 'AUDIO_RECORD_START_ERROR');
        this.startPolling();
      },
    });
  }

  change(recording: AudioRecording, action: 'pause' | 'resume' | 'stop'): void {
    if (this.busy()) { return; }
    this.begin(action, recording.id);
    this.service.changeRecording(recording.id, action).subscribe({
      next: () => this.complete(action === 'stop' ? 'AUDIO_RECORD_SAVED' : 'AUDIO_RECORD_UPDATED'),
      error: error => {
        this.fail(error, 'AUDIO_RECORD_CONTROL_ERROR');
        this.startPolling();
      },
    });
  }

  listen(recording: AudioRecording): void {
    if (this.busy()) { return; }
    this.begin('listen', recording.id);
    this.clearPreview();
    this.error.set('');
    this.previewRequest = this.service.recordingFile(recording.id).subscribe({
      next: blob => {
        this.action.set(null);
        this.preview.set({ id: recording.id, url: URL.createObjectURL(blob) });
      },
      error: error => {
        this.fail(error, 'AUDIO_RECORD_FILE_ERROR');
      },
    });
  }

  addToLibrary(recording: AudioRecording): void {
    if (this.busy()) { return; }
    this.begin('library', recording.id);
    this.service.addRecordingToLibrary(recording.id).subscribe({
      next: () => {
        this.service.libraryChanged.next();
        this.complete('AUDIO_RECORD_ADDED');
      },
      error: error => this.fail(error, 'AUDIO_RECORD_LIBRARY_ERROR'),
    });
  }

  remove(): void {
    const recording = this.pendingDelete();
    if (!recording || this.busy()) { return; }
    this.begin('delete', recording.id);
    this.service.deleteRecording(recording.id).subscribe({
      next: () => {
        if (this.preview()?.id === recording.id) { this.clearPreview(); }
        this.pendingDelete.set(null);
        this.complete('AUDIO_RECORD_DELETED');
      },
      error: error => this.fail(error, 'AUDIO_RECORD_DELETE_ERROR'),
    });
  }

  processing(type: string, id?: number): boolean {
    return this.action()?.type === type && this.action()?.id === id;
  }

  private begin(type: string, id?: number): void {
    this.action.set({ type, id });
    this.error.set('');
    this.notice.set('');
  }

  private complete(notice: string): void {
    this.action.set(null);
    this.notice.set(notice);
    this.startPolling();
  }

  private fail(error: unknown, fallback: string): void {
    this.setError(error, fallback);
    this.action.set(null);
  }

  private setError(error: unknown, fallback: string): void {
    const body: unknown = error instanceof HttpErrorResponse ? error.error : undefined;
    const code = typeof body === 'object' && body !== null && 'code' in body ? body.code : undefined;
    this.error.set(typeof code === 'string' && code.startsWith('AUDIO_') ? code : fallback);
  }

  private clearPreview(): void {
    this.previewRequest?.unsubscribe();
    const preview = this.preview();
    if (preview) { URL.revokeObjectURL(preview.url); }
    this.preview.set(null);
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    this.poll?.unsubscribe();
    this.clearPreview();
  }
}
