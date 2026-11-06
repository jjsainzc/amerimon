import { Component, DestroyRef, inject, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { environment } from '../../../core/environment';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';

interface Section {
  section: string;
  values: Record<string, string>;
  password_set: boolean;
}
interface Config {
  path: string;
  astnode: string;
  revision: string;
  loaded: boolean;
  sections: Section[];
}
interface Port {
  protocol: string;
  port: number;
  server?: string;
  reachable?: boolean;
  listening?: boolean;
}

@Component({
  selector: 'app-echolink',
  standalone: true,
  imports: [FormsModule, TranslatePipe, PasswordFieldComponent, AdminVolverComponent],
  templateUrl: './echolink.component.html',
  styles: `
    :host { display: block; min-width: 0; }
    .echolink { box-sizing: border-box; width: 100%; max-width: 65rem; min-width: 0; margin: auto; padding: 1rem; color: #dce5eb; overflow-wrap: anywhere; }
    fieldset { min-inline-size: 0; border: 1px solid #506673; border-radius: 6px; margin: 1rem 0; padding: 1rem; }
    .fields { display: grid; grid-template-columns: repeat(auto-fit,minmax(min(220px,100%),1fr)); gap: 1rem; }
    .message-field { grid-column: 1 / -1; }
    label { display: grid; min-width: 0; gap: .4rem; }
    input, select, textarea { box-sizing: border-box; min-width: 0; max-width: 100%; padding: .5rem; border: 1px solid #506673; border-radius: 4px; background: #14212a; color: inherit; }
    .password-field { width: 100%; max-width: 28rem; }
    button { padding: .6rem; border: 1px solid #506673; border-radius: 4px; background: #16616d; color: inherit; cursor: pointer; }
    button:disabled { opacity: .5; cursor: not-allowed; }
    .actions { display: flex; gap: .6rem; flex-wrap: wrap; margin: 1rem 0; }
    .error { color: #ffaaa4; } .notice { color: #9cddc2; }
    .check { display: flex; align-items: center; gap: .5rem; }
    .module-status { display: flex; align-items: center; gap: .7rem; padding: 1rem; border: 2px solid; border-radius: 6px; font-size: 1.1rem; }
    .module-loaded { color: #b5f5ce; background: #123525; border-color: #54c98b; }
    .module-unloaded { color: #ffd0ca; background: #421f24; border-color: #ed8278; }
    .status-dot { flex: 0 0 .75rem; height: .75rem; border-radius: 50%; background: currentColor; }
  `,
})
export class EchoLinkComponent {
  private readonly http = inject(HttpClient);
  private readonly destroyRef = inject(DestroyRef);
  private readonly url = `${environment.apiUrl}/echolink`;
  readonly config = signal<Config | null>(null);
  readonly busy = signal(false);
  readonly locating = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly ports = signal<Port[]>([]);
  selected = '';
  values: Record<string, string> = {};
  readonly password = signal('');
  persist = false;
  pending: 'enable' | 'disable' | 'reload' | null = null;
  readonly groups = [
    { label: 'ECHOLINK_IDENTITY', keys: ['call', 'name', 'qth', 'email', 'node'] },
    { label: 'ECHOLINK_RADIO', keys: ['lat', 'lon', 'freq', 'tone', 'power', 'height', 'gain', 'dir'] },
    { label: 'ECHOLINK_CONNECTIONS', keys: ['maxstns', 'rtcptimeout', 'recfile', 'context', 'server1', 'server2', 'server3', 'server4', 'message'] },
  ];

  constructor() { this.load(); }

  showPassword(visible: boolean): void {
    const config = this.config();
    if (!visible || this.password() || !config || this.busy() ||
        !config.sections.find(section => section.section === this.selected)?.password_set) { return; }
    const selected = this.selected;
    this.busy.set(true);
    this.error.set('');
    this.http.get<{ password: string }>(`${this.url}/password`, {
      params: { section: selected, revision: config.revision },
    }).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: result => {
        if (this.selected === selected && !this.password()) { this.password.set(result.password); }
        this.busy.set(false);
      },
      error: error => this.fail(error, 'ECHOLINK_PASSWORD_ERROR'),
    });
  }

  useBrowserLocation(): void {
    if (!this.config() || this.busy()) { return; }
    this.error.set('');
    this.notice.set('');
    if (window.isSecureContext === false || !navigator.geolocation) {
      this.error.set('ECHOLINK_LOCATION_UNAVAILABLE');
      return;
    }
    const values = this.values;
    this.busy.set(true);
    this.locating.set(true);
    navigator.geolocation.getCurrentPosition(
      position => {
        if (this.destroyRef.destroyed) { return; }
        this.busy.set(false);
        this.locating.set(false);
        if (this.values !== values) { return; }
        const { latitude, longitude } = position.coords;
        if (!Number.isFinite(latitude) || !Number.isFinite(longitude) ||
            Math.abs(latitude) > 90 || Math.abs(longitude) > 180) {
          this.error.set('ECHOLINK_LOCATION_FAILED');
          return;
        }
        this.values['lat'] = latitude.toFixed(6);
        this.values['lon'] = longitude.toFixed(6);
        this.notice.set('ECHOLINK_LOCATION_READY');
      },
      error => {
        if (this.destroyRef.destroyed) { return; }
        this.busy.set(false);
        this.locating.set(false);
        this.error.set(error.code === 1 ? 'ECHOLINK_LOCATION_DENIED'
          : error.code === 3 ? 'ECHOLINK_LOCATION_TIMEOUT' : 'ECHOLINK_LOCATION_FAILED');
      },
      { enableHighAccuracy: true, maximumAge: 0, timeout: 10_000 },
    );
  }

  load(): void {
    if (this.busy()) { return; }
    this.busy.set(true);
    this.error.set('');
    this.http.get<Config>(`${this.url}/config`).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: config => {
        this.config.set(config);
        this.select(config.sections.some(s => s.section === this.selected) ? this.selected : config.sections[0].section);
        this.busy.set(false);
      },
      error: error => this.fail(error, 'ECHOLINK_READ_ERROR'),
    });
  }

  select(name: string): void {
    const section = this.config()?.sections.find(s => s.section === name);
    if (!section) { return; }
    this.selected = name;
    this.values = { ...section.values };
    delete this.values['astnode'];
    this.password.set('');
  }

  save(): void {
    const config = this.config();
    if (!config || this.busy()) { return; }
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    this.http.put<{ revision: string }>(`${this.url}/config`, {
      section: this.selected, astnode: config.astnode, revision: config.revision,
      values: this.values, password: this.password(),
    }).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: result => {
        this.config.set({ ...config, revision: result.revision, sections: config.sections.map(s =>
          s.section === this.selected ? { ...s, values: { ...this.values }, password_set: s.password_set || !!this.password() } : s) });
        this.password.set('');
        this.notice.set('ECHOLINK_SAVED');
        this.busy.set(false);
      },
      error: error => this.fail(error, 'ECHOLINK_SAVE_ERROR'),
    });
  }

  apply(): void {
    if (!this.pending || this.busy()) { return; }
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    this.http.post<{ loaded: boolean }>(`${this.url}/activate`, {
      action: this.pending, persist: this.pending === 'reload' ? false : this.persist,
      confirmation: 'APPLY ECHOLINK',
    }).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: result => {
        this.config.update(config => config ? { ...config, loaded: result.loaded } : config);
        this.pending = null;
        this.busy.set(false);
        this.notice.set('ECHOLINK_APPLIED');
      },
      error: error => this.fail(error, 'ECHOLINK_ACTIVATE_ERROR'),
    });
  }

  checkPorts(): void {
    if (this.busy()) { return; }
    this.busy.set(true);
    this.error.set('');
    this.ports.set([]);
    this.http.get<{ results: Port[] }>(`${this.url}/ports`).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: result => { this.ports.set(result.results); this.busy.set(false); },
      error: error => this.fail(error, 'ECHOLINK_PORTS_ERROR'),
    });
  }

  private fail(error: unknown, fallback: string): void {
    const code = error instanceof HttpErrorResponse ? error.error?.code : null;
    this.error.set(typeof code === 'string' && code.startsWith('ECHOLINK_') ? code : fallback);
    this.busy.set(false);
  }
}
