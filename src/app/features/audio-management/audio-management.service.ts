import { HttpClient, HttpEvent } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { Observable, Subject, defer, finalize, map } from 'rxjs';
import { AudioRecording, AudioTrack, AudioTrackUpload } from './audio-management.model';
import { AUDIO_MANAGEMENT_API_URL } from './audio-management.tokens';

interface AudioApiResponse<T> {
  status: string;
  data: T;
  message?: string;
}

@Injectable()
export class AudioManagementService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = inject(AUDIO_MANAGEMENT_API_URL);
  private readonly tracksUrl = `${this.apiUrl}/tracks`;
  readonly libraryChanged = new Subject<void>();
  private readonly pending = signal(0);
  readonly busy = computed(() => this.pending() > 0);

  private track<T>(request: Observable<T>): Observable<T> {
    return defer(() => {
      this.pending.update(count => count + 1);
      return request.pipe(finalize(() => this.pending.update(count => count - 1)));
    });
  }

  listRecordings(foreground = false): Observable<AudioRecording[]> {
    const request = this.http.get<AudioApiResponse<AudioRecording[]>>(`${this.apiUrl}/recordings`).pipe(map(response => response.data));
    return foreground ? this.track(request) : request;
  }

  startRecording(name: string) {
    return this.track(this.http.post(`${this.apiUrl}/recordings`, { name }));
  }

  changeRecording(id: number, action: 'pause' | 'resume' | 'stop') {
    return this.track(this.http.patch(`${this.apiUrl}/recordings/${id}`, { action }));
  }

  recordingFile(id: number) {
    return this.track(this.http.get(`${this.apiUrl}/recordings/${id}/file`, { responseType: 'blob' }));
  }

  deleteRecording(id: number) {
    return this.track(this.http.delete(`${this.apiUrl}/recordings/${id}`));
  }

  addRecordingToLibrary(id: number) {
    return this.track(this.http.post(`${this.apiUrl}/recordings/${id}/library`, {}));
  }

  list(): Observable<AudioTrack[]> {
    return this.track(this.http.get<AudioApiResponse<AudioTrack[]>>(this.tracksUrl).pipe(map(response => response.data)));
  }

  upload(audio: AudioTrackUpload): Observable<HttpEvent<AudioApiResponse<AudioTrack>>> {
    const body = new FormData();
    body.set('name', audio.name);
    body.set('playback_mode', audio.playbackMode);
    if (audio.playbackMode === 'broadcast_silence') {
      body.set('interval_minutes', String(audio.intervalMinutes));
    }
    body.set('file', audio.file);
    return this.track(this.http.post<AudioApiResponse<AudioTrack>>(this.tracksUrl, body, {
      observe: 'events',
      reportProgress: true,
    }));
  }

  update(id: number, patch: Partial<Pick<AudioTrack, 'name' | 'playback_mode' | 'enabled' | 'interval_minutes'>>): Observable<void> {
    return this.track(this.http.patch<void>(`${this.tracksUrl}/${id}`, patch));
  }

  delete(id: number): Observable<void> {
    return this.track(this.http.delete<void>(`${this.tracksUrl}/${id}`));
  }

  play(id: number): Observable<void> {
    return this.track(this.http.post<void>(`${this.tracksUrl}/${id}/play`, {}));
  }

  trackFile(id: number): Observable<Blob> {
    return this.track(this.http.get(`${this.tracksUrl}/${id}/file`, { responseType: 'blob' }));
  }
}
