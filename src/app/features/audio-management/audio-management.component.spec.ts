import { HttpEventType, provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { AudioManagementComponent } from './audio-management.component';
import { AudioRecordingsComponent } from './audio-recordings.component';
import { AudioTrack } from './audio-management.model';
import { provideAudioManagement } from './audio-management.providers';
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
});
