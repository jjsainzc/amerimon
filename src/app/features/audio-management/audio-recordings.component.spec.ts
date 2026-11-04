import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { vi } from 'vitest';
import { AudioRecordingsComponent } from './audio-recordings.component';
import { AudioRecording } from './audio-management.model';
import { AudioManagementService } from './audio-management.service';
import { provideAudioManagement } from './audio-management.providers';
import es from '../../../../public/assets/i18n/es.json';
import en from '../../../../public/assets/i18n/en.json';

const baseUrl = '/api/audio/recordings';
const recording: AudioRecording = {
  id: 1, name: 'QSO test', node: '616265', state: 'recording', auto_paused: true, level_dbfs: -60,
  file_size: 44, duration_ms: 0, created_at: '2026-11-04T13:00:00Z', in_library: false,
};

describe('Reusable node audio recordings', () => {
  let fixture: ComponentFixture<AudioRecordingsComponent>;
  let http: HttpTestingController;
  const language = signal<'es' | 'en'>('es');

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {});
    language.set('es');
    const translations: Record<string, Record<string, string>> = { es, en };
    TestBed.configureTestingModule({
      imports: [AudioRecordingsComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(),
        provideAudioManagement('/api/audio', key => translations[language()][key] ?? key),
      ],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    fixture?.destroy();
    http.verify();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  async function create(records: AudioRecording[] = [recording]) {
    fixture = TestBed.createComponent(AudioRecordingsComponent);
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(0);
    http.expectOne(baseUrl).flush({ status: 'success', data: records });
    fixture.detectChanges();
    return fixture.componentInstance;
  }

  async function refresh(records: AudioRecording[]) {
    await vi.advanceTimersByTimeAsync(0);
    http.expectOne(baseUrl).flush({ status: 'success', data: records });
    fixture.detectChanges();
  }

  it('distinguishes automatic silence pause from manual pause and sends explicit resume', async () => {
    const component = await create();
    expect(fixture.nativeElement.textContent).toContain('Pausa automática: esperando audio');
    component.change(recording, 'pause');
    const pause = http.expectOne(`${baseUrl}/1`);
    expect(pause.request.body).toEqual({ action: 'pause' });
    pause.flush({});
    const paused: AudioRecording = { ...recording, state: 'paused', auto_paused: false };
    await refresh([paused]);
    expect(fixture.nativeElement.textContent).toContain('En pausa manual');
    expect(fixture.nativeElement.textContent).toContain('Continuar');
    component.change(paused, 'resume');
    const resume = http.expectOne(`${baseUrl}/1`);
    expect(resume.request.body).toEqual({ action: 'resume' });
    resume.flush({});
    await refresh([recording]);
  });

  it('updates the recording input meter every 500 ms, including during manual pause', async () => {
    await create([{ ...recording, level_dbfs: -24 }]);
    expect(fixture.nativeElement.querySelector('[role="meter"]').getAttribute('aria-valuenow')).toBe('-24');
    await vi.advanceTimersByTimeAsync(500);
    http.expectOne(baseUrl).flush({ data: [{ ...recording, state: 'paused', level_dbfs: -12 }] });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="meter"]').getAttribute('aria-valuenow')).toBe('-12');
  });

  it('starts on the server and never sends stop when the component is destroyed', async () => {
    const component = await create([]);
    component.name = 'QSO test';
    component.start();
    const start = http.expectOne(baseUrl);
    expect(start.request.method).toBe('POST');
    expect(start.request.body).toEqual({ name: 'QSO test' });
    start.flush({});
    await refresh([recording]);
    expect(component.hasActive()).toBe(true);
    component.ngOnDestroy();
    await vi.advanceTimersByTimeAsync(6000);
    http.expectNone(() => true);
  });

  it('downloads authenticated WAV for browser playback, not station broadcast, and revokes its URL', async () => {
    const createURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:recording-test');
    const revokeURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const saved: AudioRecording = { ...recording, state: 'completed', duration_ms: 1000 };
    const component = await create([saved]);
    component.listen(saved);
    fixture.detectChanges();
    const activeButton = fixture.nativeElement.querySelector('button[aria-busy="true"]');
    expect(activeButton.textContent).toContain(es.AUDIO_RECORD_PROCESSING);
    expect(fixture.nativeElement.querySelector('button[type="submit"]').textContent).toContain(es.AUDIO_RECORD_START);
    component.addToLibrary(saved);
    http.expectNone(`${baseUrl}/1/library`);
    const request = http.expectOne(`${baseUrl}/1/file`);
    expect(request.request.responseType).toBe('blob');
    request.flush(new Blob(['RIFF'], { type: 'audio/wav' }));
    fixture.detectChanges();
    expect(createURL).toHaveBeenCalledOnce();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('audio')?.getAttribute('src')).toBe('blob:recording-test');
    http.expectNone('/api/audio/tracks/1/play');
    component.ngOnDestroy();
    expect(revokeURL).toHaveBeenCalledWith('blob:recording-test');
  });

  it('copies a recording to the library and notifies the library without broadcasting', async () => {
    const saved: AudioRecording = { ...recording, state: 'completed', duration_ms: 1000 };
    const component = await create([saved]);
    const libraryChanged = vi.fn();
    const subscription = TestBed.inject(AudioManagementService).libraryChanged.subscribe(libraryChanged);
    component.addToLibrary(saved);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelectorAll('button[aria-busy="true"]').length).toBe(1);
    expect(fixture.nativeElement.querySelector('button[aria-busy="true"]').textContent).toContain(es.AUDIO_RECORD_PROCESSING);
    const request = http.expectOne(`${baseUrl}/1/library`);
    expect(request.request.method).toBe('POST');
    request.flush({});
    expect(libraryChanged).toHaveBeenCalledOnce();
    await refresh([{ ...saved, in_library: true }]);
    expect(fixture.nativeElement.textContent).toContain('En la biblioteca');
    http.expectNone('/api/audio/tracks/1/play');
    subscription.unsubscribe();
  });

  it('requires delete confirmation and translates recording labels and errors in English', async () => {
    language.set('en');
    const saved: AudioRecording = { ...recording, state: 'completed', duration_ms: 1000 };
    const component = await create([saved]);
    expect(fixture.nativeElement.textContent).toContain('Primary node recordings');
    component.pendingDelete.set(saved);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="dialog"]')?.textContent).toContain('Delete this recording?');
    http.expectNone(`${baseUrl}/1`);
    component.remove();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.audio-confirm-delete').textContent).toContain(en.AUDIO_DELETING);
    expect(fixture.nativeElement.querySelector('.audio-confirm-cancel').disabled).toBe(true);
    http.expectOne(`${baseUrl}/1`).flush(
      { code: 'AUDIO_RECORD_DELETE_ERROR' }, { status: 500, statusText: 'Error' },
    );
    fixture.detectChanges();
    expect(component.busy()).toBe(false);
    expect(fixture.nativeElement.querySelector('[role="dialog"]')?.textContent).toContain('Could not delete the recording.');
  });

  it('updates existing labels, meter and icon tooltips when the language changes', async () => {
    await create();
    expect(fixture.nativeElement.textContent).toContain(es.AUDIO_RECORD_TITLE);
    language.set('en');
    fixture.changeDetectorRef.markForCheck();
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain(en.AUDIO_RECORD_TITLE);
    expect(fixture.nativeElement.querySelector('button[audioIcon="record"]').title).toBe(en.AUDIO_RECORD_START);
    expect(fixture.nativeElement.querySelector('[role="meter"]').getAttribute('aria-label')).toBe(en.AUDIO_LEVEL_RECORDING);
    language.set('es');
    fixture.changeDetectorRef.markForCheck();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[audioIcon="record"]').title).toBe(es.AUDIO_RECORD_START);
  });
});
