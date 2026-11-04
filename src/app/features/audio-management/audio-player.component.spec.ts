import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { AudioPlayerComponent } from './audio-player.component';
import { AudioLevelMeterComponent } from './audio-level-meter.component';

describe('Reusable audio level meters', () => {
  let fixture: ComponentFixture<AudioPlayerComponent>;
  const connect = vi.fn();
  const resume = vi.fn();
  const close = vi.fn();
  let amplitude = 0.5;

  beforeEach(() => {
    vi.useFakeTimers();
    amplitude = 0.5;
    resume.mockReset().mockResolvedValue(undefined);
    close.mockReset().mockResolvedValue(undefined);
    connect.mockReset();
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {});
    vi.stubGlobal('AudioContext', class {
      destination = {};
      createAnalyser() {
        return {
          fftSize: 1024, connect,
          getFloatTimeDomainData: (data: Float32Array) => data.fill(amplitude),
        };
      }
      createMediaElementSource() { return { connect }; }
      resume = resume;
      close = close;
    });
    TestBed.configureTestingModule({ imports: [AudioPlayerComponent, AudioLevelMeterComponent] });
  });

  afterEach(() => {
    fixture?.destroy();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  function create() {
    fixture = TestBed.createComponent(AudioPlayerComponent);
    fixture.componentRef.setInput('src', 'blob:test');
    fixture.detectChanges();
    vi.spyOn(fixture.nativeElement.querySelector('audio'), 'paused', 'get').mockReturnValue(false);
    return fixture.componentInstance;
  }

  it('measures real PCM RMS and resets on pause and end without duplicating the audio graph', async () => {
    const component = create();
    await component.startMeter();
    await vi.advanceTimersByTimeAsync(50);
    expect(component.level()).toBeCloseTo(-6.02, 2);
    amplitude = 0;
    await vi.advanceTimersByTimeAsync(50);
    expect(component.level()).toBe(-60);
    amplitude = 0.25;
    await vi.advanceTimersByTimeAsync(50);
    expect(component.level()).toBeCloseTo(-12.04, 2);
    fixture.nativeElement.querySelector('audio').dispatchEvent(new Event('pause'));
    expect(component.level()).toBe(-60);
    await component.startMeter();
    expect(connect).toHaveBeenCalledTimes(2);
    fixture.nativeElement.querySelector('audio').dispatchEvent(new Event('ended'));
    await vi.advanceTimersByTimeAsync(100);
    expect(component.level()).toBe(-60);
    fixture.destroy();
    expect(close).toHaveBeenCalledOnce();
  });

  it('does not restart metering after a pending resume is cancelled', async () => {
    const component = create();
    let finish!: () => void;
    resume.mockReturnValue(new Promise<void>(resolve => { finish = resolve; }));
    const starting = component.startMeter();
    component.stopMeter();
    finish();
    await starting;
    await vi.advanceTimersByTimeAsync(100);
    expect(component.level()).toBe(-60);
  });

  it('reports an unavailable audio context explicitly', async () => {
    vi.stubGlobal('AudioContext', undefined);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const component = create();
    await component.startMeter();
    expect(component.error()).toBe('AUDIO_LEVEL_ERROR');
    expect(component.level()).toBe(-60);
  });

  it('clamps the accessible meter range and rejects nonfinite input', () => {
    const meter = TestBed.createComponent(AudioLevelMeterComponent);
    for (const [value, expected] of [[-80, -60], [-20, -20], [3, 0], [NaN, -60]]) {
      meter.componentRef.setInput('dbfs', value);
      meter.detectChanges();
      expect(meter.nativeElement.querySelector('[role="meter"]').getAttribute('aria-valuenow')).toBe(String(expected));
    }
    meter.destroy();
  });
});
