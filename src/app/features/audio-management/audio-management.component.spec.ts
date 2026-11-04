import { HttpEventType, provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { AudioManagementComponent } from './audio-management.component';
import { AudioRecordingsComponent } from './audio-recordings.component';
import { AudioTrack } from './audio-management.model';
import { provideAudioManagement } from './audio-management.providers';
import { AUDIO_MANAGEMENT_MONITOR_URL } from './audio-management.tokens';
import { By } from '@angular/platform-browser';
import es from '../../../../public/assets/i18n/es.json';

const track: AudioTrack = {
  id: 1, name: 'Test', original_name: 'test.wav', playback_mode: 'broadcast_once',
  enabled: true, file_size: 1644, duration_ms: 100, sample_rate: 8000, bits_per_sample: 16,
  created_at: '2026-11-04T13:00:00Z',
};

describe('Audio management foreground actions', () => {
  let fixture: ComponentFixture<AudioManagementComponent>;
  let http: HttpTestingController;

  beforeEach(async () => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {});
    const labels: Record<string, string> = es;
    TestBed.configureTestingModule({
      imports: [AudioManagementComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(),
        provideAudioManagement('/api/audio', key => labels[key] ?? key),
        { provide: AUDIO_MANAGEMENT_MONITOR_URL, useValue: 'allstarlink' },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(AudioManagementComponent);
    fixture.detectChanges();
    http.expectOne('/api/audio/tracks').flush({ data: [track] });
    await vi.advanceTimersByTimeAsync(0);
    http.expectOne('/api/audio/recordings').flush({ data: [] });
    fixture.detectChanges();
  });

  afterEach(() => {
    fixture.destroy();
    http.verify();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  async function expectLocked() {
    fixture.detectChanges();
    await Promise.resolve();
    const buttons: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('button'));
    expect(buttons.every(button => button.disabled)).toBe(true);
    const inputs: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('input, select'));
    expect(inputs.every(input => input.disabled)).toBe(true);
  }

  it('shows upload progress only in the upload button and blocks recording and library actions', async () => {
    const component = fixture.componentInstance;
    component.name = 'Upload';
    component.file = new File(['RIFF'], 'test.wav');
    component.upload();
    const upload = http.expectOne('/api/audio/tracks');
    upload.event({ type: HttpEventType.UploadProgress, loaded: 2, total: 4 });
    await expectLocked();
    const button: HTMLButtonElement = fixture.nativeElement.querySelector('button[aria-busy="true"]');
    expect(button.textContent).toContain('50%');
    expect(fixture.nativeElement.querySelectorAll('.audio-upload-progress, .audio-upload-progress-label').length).toBe(0);
    const recordings = fixture.debugElement.query(By.directive(AudioRecordingsComponent)).componentInstance as AudioRecordingsComponent;
    recordings.name = 'Blocked';
    recordings.start();
    component.play(track);
    component.toggle(track);
    component.load();
    http.expectNone('/api/audio/recordings');
    http.expectNone('/api/audio/tracks/1/play');
    http.expectNone('/api/audio/tracks/1');
    upload.flush({ data: { ...track, id: 2 } });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]')).toBeNull();
  });

  it('keeps the active toggle label and lock through the library reload', async () => {
    const component = fixture.componentInstance;
    component.toggle(track);
    await expectLocked();
    expect(fixture.nativeElement.querySelector('.audio-state-toggle').textContent).toContain('Procesando');
    http.expectOne('/api/audio/tracks/1').flush({});
    await expectLocked();
    expect(fixture.nativeElement.querySelector('.audio-state-toggle').textContent).toContain('Procesando');
    expect(fixture.nativeElement.querySelector('.audio-page > header button').textContent).not.toContain(es.AUDIO_LOADING);
    http.expectOne('/api/audio/tracks').flush({ data: [{ ...track, enabled: false }] });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('.audio-state-toggle').textContent).toContain(es.AUDIO_ENABLE);
  });

  it('unlocks all controls after a playback error and keeps the error visible', async () => {
    const component = fixture.componentInstance;
    component.play(track);
    component.confirmPlayback();
    await expectLocked();
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_PLAYING);
    http.expectOne('/api/audio/tracks/1/play').flush({}, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain(es.AUDIO_PLAY_ERROR);
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]')).toBeNull();
  });

  it('keeps settings progress inside its save button until the refreshed settings arrive', async () => {
    const component = fixture.componentInstance;
    const select: HTMLSelectElement = fixture.nativeElement.querySelector('.audio-track-mode select');
    select.value = 'broadcast_silence';
    select.dispatchEvent(new Event('change'));
    component.saveSettings(track);
    await expectLocked();
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_SAVING_SETTINGS);
    http.expectOne('/api/audio/tracks/1').flush({});
    await expectLocked();
    expect(fixture.nativeElement.querySelectorAll('button[aria-busy="true"]').length).toBe(1);
    http.expectOne('/api/audio/tracks').flush({ data: [{ ...track, playback_mode: 'broadcast_silence', interval_minutes: 60 }] });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(component.hasUnsavedChanges(component.tracks()[0])).toBe(false);
  });

  it('keeps delete progress in the confirmation button and unlocks after deletion', async () => {
    const component = fixture.componentInstance;
    component.remove(track);
    component.confirmDelete();
    await expectLocked();
    expect(fixture.nativeElement.querySelector('.audio-confirm-delete').textContent).toContain(es.AUDIO_DELETING);
    component.cancelDelete();
    expect(component.pendingDelete()).toEqual(track);
    http.expectOne('/api/audio/tracks/1').flush({});
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('shows recording processing only in its start button and blocks the library', async () => {
    const component = fixture.componentInstance;
    const recordings = fixture.debugElement.query(By.directive(AudioRecordingsComponent)).componentInstance as AudioRecordingsComponent;
    recordings.name = 'Test recording';
    recordings.start();
    await expectLocked();
    expect(fixture.nativeElement.querySelectorAll('button[aria-busy="true"]').length).toBe(1);
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_RECORD_PROCESSING);
    component.play(track);
    http.expectNone('/api/audio/tracks/1/play');
    http.expectOne('/api/audio/recordings').flush({}, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]')).toBeNull();
  });

  it('uses distinct accessible icons for local preview and Asterisk broadcast', () => {
    const local: HTMLButtonElement = fixture.nativeElement.querySelector('button[audioIcon="listen"]');
    const broadcast: HTMLButtonElement = fixture.nativeElement.querySelector('button[audioIcon="broadcast"]');
    expect(local.title).toBe(es.AUDIO_LISTEN_LOCAL);
    expect(broadcast.title).toBe(es.AUDIO_PLAY_ASTERISK);
    for (const button of fixture.nativeElement.querySelectorAll('button[audioIcon]')) {
      expect(button.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
      expect(button.querySelector('.label')?.textContent.trim()).toBeTruthy();
      expect(button.title).toBe(button.querySelector('.label').textContent.trim());
    }
    broadcast.click();
    fixture.componentInstance.confirmPlayback();
    http.expectOne('/api/audio/tracks/1/play').flush({});
    http.expectNone('/api/audio/tracks/1/file');
  });

  it('previews disabled and scheduled tracks as authenticated blobs without broadcasting and releases URLs', async () => {
    const component = fixture.componentInstance;
    const createURL = vi.spyOn(URL, 'createObjectURL').mockReturnValueOnce('blob:first').mockReturnValueOnce('blob:second');
    const revokeURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    component.tracks.set([{ ...track, enabled: false, playback_mode: 'broadcast_silence' }]);
    fixture.detectChanges();
    const local: HTMLButtonElement = fixture.nativeElement.querySelector('button[audioIcon="listen"]');
    expect(local.disabled).toBe(false);
    expect(fixture.nativeElement.querySelector('button[audioIcon="broadcast"]')).toBeNull();
    local.click();
    const request = http.expectOne('/api/audio/tracks/1/file');
    expect(request.request.responseType).toBe('blob');
    await expectLocked();
    request.flush(new Blob(['RIFF'], { type: 'audio/wav' }));
    fixture.detectChanges();
    expect(createURL).toHaveBeenCalledOnce();
    expect(fixture.nativeElement.querySelector('app-audio-player audio').getAttribute('src')).toBe('blob:first');
    http.expectNone('/api/audio/tracks/1/play');
    component.listen(track);
    expect(revokeURL).toHaveBeenCalledWith('blob:first');
    http.expectOne('/api/audio/tracks/1/file').flush(new Blob(['RIFF']));
    fixture.detectChanges();
    fixture.destroy();
    expect(revokeURL).toHaveBeenCalledWith('blob:second');
  });

  it('unlocks on a preview failure and displays a translated error', async () => {
    const component = fixture.componentInstance;
    component.listen(track);
    await expectLocked();
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').title).toBe(es.AUDIO_LOADING);
    http.expectOne('/api/audio/tracks/1/file').flush(new Blob(['error']), { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(component.previewLoadingId()).toBeNull();
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain(es.AUDIO_PREVIEW_ERROR);
  });

  it('offers the monitor window from audio management without navigating the page', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const button: HTMLButtonElement = fixture.nativeElement.querySelector('button[audioIcon="monitor"]');
    expect(button.title).toBe(es.AUDIO_OPEN_MONITOR_WINDOW);
    button.click();
    fixture.detectChanges();
    expect(open).toHaveBeenCalledOnce();
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain(es.AUDIO_WINDOW_BLOCKED);
  });

  it('saves the selected bounds before broadcasting and restores them on reopening', () => {
    const component = fixture.componentInstance;
    component.play(track);
    http.expectNone('/api/audio/tracks/1/play');
    component.playbackComplete = false;
    component.playbackStartMS = 20;
    component.playbackEndMS = 80;
    component.confirmPlayback();
    const save = http.expectOne('/api/audio/tracks/1');
    expect(save.request.body).toEqual({ start_ms: 20, end_ms: 80 });
    http.expectNone('/api/audio/tracks/1/play');
    save.flush({});
    const request = http.expectOne('/api/audio/tracks/1/play');
    expect(request.request.body).toEqual({});
    request.flush({});
    expect(component.pendingPlayback()).toBeNull();
    component.play(component.tracks()[0]);
    expect(component.playbackComplete).toBe(false);
    expect(component.playbackStartMS).toBe(20);
    expect(component.playbackEndMS).toBe(80);
    component.playbackStartMS = 90;
    component.playbackEndMS = 80;
    component.confirmPlayback();
    http.expectNone('/api/audio/tracks/1/play');
  });

  it('saves and resets scheduled bounds without transmitting or changing intervals', async () => {
    const component = fixture.componentInstance;
    const scheduled: AudioTrack = { ...track, playback_mode: 'broadcast_silence', interval_minutes: 15, start_ms: 20, end_ms: 80 };
    component.tracks.set([scheduled]);
    component.editPlaybackSelection(scheduled);
    fixture.detectChanges();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.audio-range input').value).toBe('20');
    const ranges: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('.audio-range input'));
    expect(ranges.map(input => Number(input.value))).toEqual([20, 80]);
    component.changePlaybackStart(95);
    expect(component.playbackStartMS).toBe(79);
    component.changePlaybackEnd(0);
    expect(component.playbackEndMS).toBe(80);
    component.resetPlaybackSelection();
    expect(component.playbackComplete).toBe(true);
    expect(component.playbackStartMS).toBe(0);
    expect(component.playbackEndMS).toBe(100);
    component.confirmPlayback(false);
    await expectLocked();
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_SAVING_SETTINGS);
    const request = http.expectOne('/api/audio/tracks/1');
    expect(request.request.body).toEqual({ start_ms: 0, end_ms: 0 });
    request.flush({});
    http.expectNone('/api/audio/tracks/1/play');
    expect(component.tracks()[0].interval_minutes).toBe(15);
    component.editPlaybackSelection(component.tracks()[0]);
    expect(component.playbackComplete).toBe(true);
  });

  it('keeps unsaved bounds and reports save errors without transmitting', () => {
    const component = fixture.componentInstance;
    component.play(track);
    component.playbackComplete = false;
    component.playbackStartMS = 20;
    component.playbackEndMS = 80;
    component.confirmPlayback();
    http.expectOne('/api/audio/tracks/1').flush({}, { status: 500, statusText: 'Error' });
    http.expectNone('/api/audio/tracks/1/play');
    expect(component.pendingPlayback()).not.toBeNull();
    expect(component.tracks()[0].end_ms).toBeUndefined();
    expect(component.error()).toBe('AUDIO_UPDATE_ERROR');
    expect(component.busy()).toBe(false);
  });

  it('marks saved fragments on the card with bounds and duration, and removes the badge after reset', () => {
    const component = fixture.componentInstance;
    component.tracks.set([{ ...track, start_ms: 20, end_ms: 80 }]);
    fixture.detectChanges();
    const badge: HTMLElement = fixture.nativeElement.querySelector('.audio-fragment-badge');
    expect(badge.textContent).toContain(es.AUDIO_FRAGMENT_SELECTED);
    expect(badge.textContent).toContain('0.02 s - 0.08 s');
    expect(badge.textContent).toContain('0.06 s');
    component.tracks.set([{ ...track, start_ms: 0, end_ms: 0 }]);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.audio-fragment-badge')).toBeNull();
    expect(fixture.nativeElement.querySelector('.audio-track').textContent).toContain(es.AUDIO_PLAY_COMPLETE);
  });

  it('accepts precise start and end in seconds and keeps the timeline synchronized', async () => {
    const component = fixture.componentInstance;
    component.play({ ...track, duration_ms: 55120 });
    component.playbackComplete = false;
    fixture.detectChanges();
    await fixture.whenStable();
    const inputs: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('.audio-range-values input'));
    inputs[0].value = '5.125';
    inputs[0].dispatchEvent(new Event('input'));
    inputs[1].value = '26.902';
    inputs[1].dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    expect(component.playbackStartMS).toBe(5125);
    expect(component.playbackEndMS).toBe(26902);
    const ranges: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('.audio-range input'));
    expect(ranges.map(input => Number(input.value))).toEqual([5125, 26902]);
    expect(component.validPlaybackSegment()).toBe(true);
    component.setPlaybackSeconds('end', null);
    expect(component.validPlaybackSegment()).toBe(false);
    component.confirmPlayback(false);
    http.expectNone('/api/audio/tracks/1');
  });

  it('previews complete audio inside the dialog without saving or broadcasting and cleans up on cancel', () => {
    const component = fixture.componentInstance;
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:selection');
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    component.play(track);
    component.listenSelection();
    const request = http.expectOne('/api/audio/tracks/1/file');
    expect(component.selectionLoading()).toBe(true);
    request.flush(new Blob(['RIFF'], { type: 'audio/wav' }));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="dialog"] app-audio-player audio').src).toBe('blob:selection');
    http.expectNone('/api/audio/tracks/1/play');
    http.expectNone('/api/audio/tracks/1');
    component.closePlaybackSelection();
    fixture.detectChanges();
    expect(revoke).toHaveBeenCalledWith('blob:selection');
    expect(component.pendingPlayback()).toBeNull();
  });

  it('hides a stale selection player when bounds change and reports loading errors', () => {
    const component = fixture.componentInstance;
    component.play(track);
    component.playbackComplete = false;
    component.playbackStartMS = 20;
    component.playbackEndMS = 80;
    component.selectionPreview.set({ url: 'blob:selection', start: 20, end: 80 });
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="dialog"] app-audio-player')).not.toBeNull();
    component.changePlaybackStart(30);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="dialog"] app-audio-player')).toBeNull();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    component.listenSelection();
    http.expectOne('/api/audio/tracks/1/file').flush(new Blob(['error']), { status: 500, statusText: 'Error' });
    expect(component.selectionLoading()).toBe(false);
    expect(component.error()).toBe('AUDIO_PREVIEW_ERROR');
    http.expectNone('/api/audio/tracks/1/play');
  });

  it('downloads authenticated WAV with its own progress and revokes the temporary URL', async () => {
    const component = fixture.componentInstance;
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:download');
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function(this: HTMLAnchorElement) {
      expect(this.download).toBe('Test.wav');
      expect(this.href).toBe('blob:download');
    });
    component.download({ ...track, enabled: false });
    const request = http.expectOne('/api/audio/tracks/1/file');
    expect(request.request.responseType).toBe('blob');
    await expectLocked();
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_DOWNLOADING);
    request.flush(new Blob(['RIFF'], { type: 'audio/wav' }));
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(click).toHaveBeenCalledOnce();
    http.expectNone('/api/audio/tracks/1/play');
    await vi.advanceTimersByTimeAsync(1000);
    expect(revoke).toHaveBeenCalledWith('blob:download');
  });

  it('reports download errors and clears button progress', async () => {
    const component = fixture.componentInstance;
    component.download(track);
    http.expectOne('/api/audio/tracks/1/file').flush(new Blob(['error']), { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(component.downloadingId()).toBeNull();
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain(es.AUDIO_DOWNLOAD_ERROR);
  });
});
