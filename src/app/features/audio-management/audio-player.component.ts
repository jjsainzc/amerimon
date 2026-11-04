import { Component, ElementRef, OnDestroy, ViewChild, input, signal } from '@angular/core';
import { AudioLevelMeterComponent } from './audio-level-meter.component';
import { AudioManagementLabelPipe } from './audio-management-label.pipe';

@Component({
  selector: 'app-audio-player',
  standalone: true,
  imports: [AudioLevelMeterComponent, AudioManagementLabelPipe],
  template: `
    <audio #audio controls [src]="src()" [attr.aria-label]="label()" (play)="startMeter()"
      (pause)="stopMeter()" (ended)="stopMeter()" (error)="playbackError()"></audio>
    <app-audio-level-meter [dbfs]="level()" [label]="'AUDIO_LEVEL_PLAYBACK' | audioLabel" />
    @if (error()) { <p class="error" role="alert">{{ error() | audioLabel }}</p> }
  `,
  styles: `
    :host { display: block; grid-column: 1 / -1; width: 100%; }
    audio { width: 100%; margin-bottom: .5rem; }
    .error { color: #ffaaa4; }
  `,
})
export class AudioPlayerComponent implements OnDestroy {
  readonly src = input.required<string>();
  readonly label = input('');
  readonly level = signal(-60);
  readonly error = signal('');
  @ViewChild('audio', { static: true }) private audio!: ElementRef<HTMLAudioElement>;
  private context?: AudioContext;
  private analyser?: AnalyserNode;
  private samples?: Float32Array<ArrayBuffer>;
  private interval?: ReturnType<typeof setInterval>;
  private destroyed = false;
  private generation = 0;

  async startMeter(): Promise<void> {
    this.error.set('');
    this.stopMeter();
    const generation = this.generation;
    try {
      if (!this.context) {
        this.context = new AudioContext();
        this.analyser = this.context.createAnalyser();
        this.analyser.fftSize = 1024;
        this.samples = new Float32Array(this.analyser.fftSize);
        const source = this.context.createMediaElementSource(this.audio.nativeElement);
        source.connect(this.analyser);
        this.analyser.connect(this.context.destination);
      }
      await this.context.resume();
      if (this.destroyed || generation !== this.generation || this.audio.nativeElement.paused) { return; }
      this.interval = setInterval(() => {
        if (!this.analyser || !this.samples) { return; }
        this.analyser.getFloatTimeDomainData(this.samples);
        const power = this.samples.reduce((sum, sample) => sum + sample * sample, 0) / this.samples.length;
        this.level.set(power > 0 ? Math.max(-60, Math.min(0, 10 * Math.log10(power))) : -60);
      }, 50);
    } catch (error) {
      console.error('Audio playback meter failed', error);
      this.error.set('AUDIO_LEVEL_ERROR');
    }
  }

  stopMeter(): void {
    this.generation++;
    clearInterval(this.interval);
    this.interval = undefined;
    this.level.set(-60);
  }

  playbackError(): void {
    this.stopMeter();
    this.error.set('AUDIO_RECORD_FILE_ERROR');
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    this.stopMeter();
    this.audio.nativeElement.pause();
    if (this.context) {
      void this.context.close().catch(error => console.error('Audio playback context close failed', error));
    }
  }
}
