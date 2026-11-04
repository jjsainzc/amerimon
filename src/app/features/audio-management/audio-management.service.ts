import { HttpClient, HttpEvent } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';
import { AudioTrack, AudioTrackUpload } from './audio-management.model';
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

  list(): Observable<AudioTrack[]> {
    return this.http.get<AudioApiResponse<AudioTrack[]>>(this.tracksUrl).pipe(map(response => response.data));
  }

  upload(audio: AudioTrackUpload): Observable<HttpEvent<AudioApiResponse<AudioTrack>>> {
    const body = new FormData();
    body.set('name', audio.name);
    body.set('playback_mode', audio.playbackMode);
    if (audio.playbackMode === 'broadcast_silence') {
      body.set('interval_minutes', String(audio.intervalMinutes));
    }
    body.set('file', audio.file);
    return this.http.post<AudioApiResponse<AudioTrack>>(this.tracksUrl, body, {
      observe: 'events',
      reportProgress: true,
    });
  }

  update(id: number, patch: Partial<Pick<AudioTrack, 'name' | 'playback_mode' | 'enabled' | 'interval_minutes'>>): Observable<void> {
    return this.http.patch<void>(`${this.tracksUrl}/${id}`, patch);
  }

  delete(id: number): Observable<void> {
    return this.http.delete<void>(`${this.tracksUrl}/${id}`);
  }

  play(id: number): Observable<void> {
    return this.http.post<void>(`${this.tracksUrl}/${id}/play`, {});
  }
}
