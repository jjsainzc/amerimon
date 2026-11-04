import { Component, computed, input } from '@angular/core';

@Component({
  selector: 'app-audio-level-meter',
  standalone: true,
  template: `
    <div class="heading"><span>{{ label() }}</span><span>{{ level().toFixed(0) }} dBFS</span></div>
    <div class="meter" role="meter" [attr.aria-label]="label()" aria-valuemin="-60" aria-valuemax="0"
      [attr.aria-valuenow]="level()" [attr.aria-valuetext]="level().toFixed(0) + ' dBFS'">
      <div class="fill" [style.clip-path]="'inset(0 ' + (-level() / 60 * 100) + '% 0 0)'"></div>
    </div>
    <div class="scale" aria-hidden="true"><span>-60</span><span>-40</span><span>-20</span><span>0 dBFS</span></div>
  `,
  styles: `
    :host { display: block; grid-column: 1 / -1; width: 100%; }
    .heading, .scale { display: flex; justify-content: space-between; gap: .5rem; color: #c4d2da; font-size: .75rem; }
    .meter { height: .75rem; margin: .3rem 0; overflow: hidden; border: 1px solid #506673; border-radius: 3px; background: #14212a; }
    .fill { height: 100%; background: linear-gradient(to right, #31bb78 0%, #31bb78 66%, #f5c34b 85%, #f36760 100%); }
  `,
})
export class AudioLevelMeterComponent {
  readonly dbfs = input(-60);
  readonly label = input('');
  readonly level = computed(() => Number.isFinite(this.dbfs()) ? Math.max(-60, Math.min(0, this.dbfs())) : -60);
}
