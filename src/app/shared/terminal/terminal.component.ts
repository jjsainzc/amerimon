import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { AfterViewInit, Component, ElementRef, OnDestroy, ViewChild, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { Terminal } from 'xterm';
import { AuthService } from '../../core/services/auth.service';
import { environment } from '../../core/environment';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { TranslationService } from '../../core/services/translation.service';
import { FormChanges } from '../../core/utils/form-changes';
import { ConfirmacionDialogComponent } from '../confirmacion-dialog/confirmacion-dialog.component';

type SSHAuthMethod = 'password' | 'publickey';
type TerminalConnectionState = 'disconnected' | 'connecting' | 'connected';
type LoginPrompt = 'username' | 'password' | 'host-key' | null;

interface TerminalSettings {
  host: string;
  port: number;
  username: string;
  authMethod: SSHAuthMethod;
}

interface TerminalControlMessage {
  type: 'status' | 'error' | 'host-key-required';
  status?: string;
  message?: string;
  host?: string;
  algorithm?: string;
  fingerprint?: string;
}

interface StoredTerminalCredentials {
  found: boolean;
  password?: string;
  privateKey?: string;
  keyPassphrase?: string;
}

interface PendingCredentialSave {
  target: TerminalSettings;
  credentials: { password: string; privateKey: string; keyPassphrase: string };
}

const settingsStorageKey = 'amerimon.ssh-terminal.settings';
const hideGuidanceStorageKey = 'amerimon.ssh-terminal.hide-guidance';
const maxPrivateKeySize = 48 * 1024;

@Component({
  selector: 'app-terminal',
  standalone: true,
  imports: [FormsModule, TranslatePipe, ConfirmacionDialogComponent],
  templateUrl: './terminal.component.html',
  styleUrl: './terminal.component.scss'
})
export class TerminalComponent implements AfterViewInit, OnDestroy {
  @ViewChild('terminalContainer', { static: true }) private terminalContainer!: ElementRef<HTMLDivElement>;

  private readonly auth = inject(AuthService);
  private readonly http = inject(HttpClient);
  private readonly translations = inject(TranslationService);
  private readonly changes = new FormChanges();
  readonly savingSettings = signal(false);
  readonly replaceCredentialsConfirmation = signal(false);
  private readonly terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Consolas, "Courier New", monospace',
    theme: {
      background: '#101820',
      foreground: '#e8eef2',
      cursor: '#e8eef2'
    }
  });
  private socket?: WebSocket;
  private resizeObserver?: ResizeObserver;
  private loginPrompt: LoginPrompt = null;
  private promptBuffer = '';
  private readonly onWindowResize = () => this.resizeTerminal();

  readonly connectionState = signal<TerminalConnectionState>('disconnected');
  readonly terminalVisible = signal(false);
  readonly status = signal('');
  readonly showGuidance = signal(localStorage.getItem(hideGuidanceStorageKey) !== 'true');

  settings: TerminalSettings = {
    host: '127.0.0.1',
    port: 22,
    username: '',
    authMethod: 'password'
  };
  password = '';
  keyPassphrase = '';
  showPassword = false;
  showKeyPassphrase = false;
  privateKey = '';
  privateKeySelected = false;
  privateKeyReading = false;
  private pendingCredentialSave: PendingCredentialSave | null = null;

  constructor() {
    this.loadSettings();
    this.changes.reset(this.settings);
  }

  hasSettingsChanges(): boolean {
    return this.changes.changed(this.settings) ||
      (this.settings.authMethod === 'password' ? this.password !== '' : this.privateKey !== '' || this.keyPassphrase !== '');
  }

  ngAfterViewInit(): void {
    this.terminal.open(this.terminalContainer.nativeElement);
    this.terminal.onData((data) => this.handleTerminalInput(data));

    if (typeof ResizeObserver !== 'undefined') {
      this.resizeObserver = new ResizeObserver(() => this.resizeTerminal());
      this.resizeObserver.observe(this.terminalContainer.nativeElement);
    } else {
      window.addEventListener('resize', this.onWindowResize);
    }
  }

  async saveSettings(): Promise<void> {
    if (this.savingSettings() || !this.hasSettingsChanges()) { return; }
    if (this.privateKeyReading) {
      this.status.set(this.translations.translate('SSH_PRIVATE_KEY_READING'));
      return;
    }
    if (!this.isSettingsValid()) {
      this.status.set(this.translations.translate('SSH_SETTINGS_INVALID'));
      return;
    }
    try {
      localStorage.setItem(settingsStorageKey, JSON.stringify(this.settings));
      this.changes.reset(this.settings);
    } catch (error) {
      console.error('No se pudo guardar la configuración SSH.', error);
      this.status.set(this.translations.translate('SSH_CONFIG_SAVE_FAILED'));
      return;
    }

    const credentials = this.currentCredentials();
    if (!this.settings.username.trim()) {
      this.status.set(this.translations.translate('SSH_CONFIG_SAVED_NO_CREDENTIALS'));
      return;
    }
    if (!credentials) {
      const messageKey = this.settings.authMethod === 'publickey'
        ? 'SSH_PRIVATE_KEY_REQUIRED'
        : 'SSH_CONFIG_SAVED_NO_CREDENTIALS';
      this.status.set(this.translations.translate(messageKey));
      return;
    }
    this.savingSettings.set(true);
    try {
      await this.storeCredentials({
        target: this.credentialTarget(),
        credentials
      }, false);
    } finally {
      this.savingSettings.set(false);
    }
  }

  async confirmCredentialReplacement(): Promise<void> {
    const pending = this.pendingCredentialSave;
    if (!pending || this.savingSettings()) { return; }
    this.savingSettings.set(true);
    try {
      await this.storeCredentials(pending, true);
    } finally {
      this.savingSettings.set(false);
    }
  }

  cancelCredentialReplacement(): void {
    if (this.savingSettings()) { return; }
    this.pendingCredentialSave = null;
    this.replaceCredentialsConfirmation.set(false);
    this.status.set(this.translations.translate('SSH_CREDENTIALS_REPLACE_CANCELLED'));
  }

  async selectPrivateKey(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    this.privateKey = '';
    this.privateKeySelected = false;
    this.privateKeyReading = true;
    this.status.set(this.translations.translate('SSH_PRIVATE_KEY_READING'));
    try {
      if (file.size > maxPrivateKeySize) {
        this.status.set(this.translations.translate('SSH_PRIVATE_KEY_TOO_LARGE'));
        return;
      }
      const content = await file.text();
      if (!content.trim()) {
        this.status.set(this.translations.translate('SSH_PRIVATE_KEY_READ_FAILED'));
        return;
      }
      this.privateKey = content;
      this.privateKeySelected = true;
      this.status.set('');
    } catch (error) {
      console.error('No se pudo leer la clave privada seleccionada.', error);
      this.privateKey = '';
      this.privateKeySelected = false;
      this.status.set(this.translations.translate('SSH_PRIVATE_KEY_READ_FAILED'));
    } finally {
      this.privateKeyReading = false;
      input.value = '';
    }
  }

  connect(): void {
    if (this.connectionState() !== 'disconnected') {
      return;
    }
    if (!this.isSettingsValid()) {
      this.status.set(this.translations.translate('SSH_SETTINGS_INVALID'));
      return;
    }
    const token = this.auth.getToken();
    if (!token) {
      this.status.set(this.translations.translate('SSH_ADMIN_SESSION_REQUIRED'));
      return;
    }

    this.terminal.clear();
    this.terminalVisible.set(true);
    this.connectionState.set('connecting');
    this.status.set(this.translations.translate('SSH_CONNECTING_STATUS'));

    const socketUrl = `${environment.wsUrl}/terminal?token=${encodeURIComponent(token)}`;
    try {
      this.socket = new WebSocket(socketUrl);
    } catch (error) {
      console.error('No se pudo abrir la conexión WebSocket SSH.', error);
      this.connectionState.set('disconnected');
      this.status.set(this.translations.translate('SSH_WEBSOCKET_OPEN_FAILED'));
      return;
    }

    const socket = this.socket;
    socket.binaryType = 'arraybuffer';
    socket.onopen = () => {
      this.promptBuffer = '';
      if (!this.settings.username.trim()) {
        this.showLoginPrompt('username');
      } else {
        this.continueLogin(socket);
      }
    };
    socket.onmessage = (event: MessageEvent<string | ArrayBuffer>) => {
      if (event.data instanceof ArrayBuffer) {
        this.terminal.write(new Uint8Array(event.data));
      } else {
        this.handleSocketMessage(event.data);
      }
    };
    socket.onerror = (error) => {
      console.error('Error en la conexión WebSocket SSH.', error);
      this.status.set(this.translations.translate('SSH_WEBSOCKET_ERROR'));
    };
    socket.onclose = (event: CloseEvent) => {
      this.password = '';
      this.keyPassphrase = '';
      this.privateKey = '';
      this.privateKeySelected = false;
      this.privateKeyReading = false;
      this.loginPrompt = null;
      this.promptBuffer = '';
      this.connectionState.set('disconnected');
      if (this.status().startsWith(this.translations.translate('SSH_ERROR_PREFIX'))) {
        this.terminal.writeln(this.status());
      } else if ((event.code === 1000 || event.code === 1001) &&
                 this.status() !== this.translations.translate('SSH_DISCONNECTING')) {
        this.status.set(this.translations.translate('SSH_DISCONNECTED'));
      } else if (event.code === 1006 && this.status() !== this.translations.translate('SSH_DISCONNECTING')) {
        const message = this.translations.translate('SSH_WEBSOCKET_CLOSED_UNEXPECTEDLY', { code: event.code });
        this.status.set(message);
        this.terminal.writeln(`\r\n${message}`);
      } else if (this.status() !== this.translations.translate('SSH_DISCONNECTING')) {
        this.status.set(this.translations.translate('SSH_CONNECTION_CLOSED'));
      }
      this.socket = undefined;
    };
  }

  disconnect(): void {
    const socket = this.socket;
    this.socket = undefined;
    this.connectionState.set('disconnected');
    this.loginPrompt = null;
    this.promptBuffer = '';
    this.clearCredentials();
    this.status.set(this.translations.translate('SSH_DISCONNECTED'));
    if (socket) socket.onmessage = null;
    this.terminal.reset();
    this.terminal.clear();
    this.terminal.write('\x1b[3J\x1b[2J\x1b[H');
    socket?.close();
  }

  ngOnDestroy(): void {
    this.resizeObserver?.disconnect();
    window.removeEventListener('resize', this.onWindowResize);
    this.socket?.close();
    this.terminal.dispose();
  }

  setGuidanceHidden(event: Event): void {
    const hidden = (event.target as HTMLInputElement).checked;
    this.showGuidance.set(!hidden);
    try {
      if (hidden) {
        localStorage.setItem(hideGuidanceStorageKey, 'true');
      } else {
        localStorage.removeItem(hideGuidanceStorageKey);
      }
    } catch (error) {
      console.error('No se pudo guardar la preferencia de explicaciones.', error);
    }
  }

  private handleSocketMessage(data: string): void {
    let control: TerminalControlMessage | undefined;
    try {
      const parsed: unknown = JSON.parse(data);
      if (typeof parsed === 'object' && parsed !== null) {
        const value = parsed as Record<string, unknown>;
        if (value['type'] === 'status' && typeof value['status'] === 'string') {
          control = { type: 'status', status: value['status'] };
        } else if (value['type'] === 'error' && typeof value['message'] === 'string') {
          control = { type: 'error', message: value['message'] };
        } else if (value['type'] === 'host-key-required' &&
                   typeof value['host'] === 'string' &&
                   typeof value['algorithm'] === 'string' &&
                   typeof value['fingerprint'] === 'string') {
          control = {
            type: 'host-key-required',
            host: value['host'],
            algorithm: value['algorithm'],
            fingerprint: value['fingerprint']
          };
        }
      }
    } catch {
      this.terminal.write(data);
      return;
    }

    if (!control) {
      this.terminal.write(data);
      return;
    }
    if (control.type === 'host-key-required') {
      this.terminal.writeln(this.translations.translate('SSH_HOST_KEY_DETAILS', {
        host: control.host ?? '',
        algorithm: control.algorithm ?? '',
        fingerprint: control.fingerprint ?? ''
      }));
      this.showLoginPrompt('host-key');
    } else if (control.type === 'status' && control.status === 'connected') {
      this.connectionState.set('connected');
      this.status.set(this.translations.translate('SSH_CONNECTED_TO', {
        username: this.settings.username, host: this.settings.host, port: this.settings.port
      }));
      this.resizeTerminal();
    } else if (control.type === 'status' && control.status === 'disconnected') {
      this.connectionState.set('disconnected');
      this.status.set(this.translations.translate('SSH_DISCONNECTED'));
    } else if (control.type === 'error') {
      this.connectionState.set('disconnected');
      this.status.set(`${this.translations.translate('SSH_ERROR_PREFIX')} ${control.message}`);
      this.socket?.close();
    }
  }

  private handleTerminalInput(data: string): void {
    if (this.loginPrompt) {
      this.handleLoginPromptInput(data);
      return;
    }
    if (this.connectionState() === 'connected' && this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ type: 'input', data }));
    }
  }

  private handleLoginPromptInput(data: string): void {
    for (const character of data) {
      if (character === '\u0003') {
        this.terminal.write('^C\r\n');
        this.disconnect();
        return;
      }
      if (character === '\r' || character === '\n') {
        this.terminal.write('\r\n');
        this.finishLoginPrompt();
        if (!this.loginPrompt) {
          return;
        }
        continue;
      }
      if (character === '\u007f' || character === '\b') {
        if (this.promptBuffer.length > 0) {
          this.promptBuffer = Array.from(this.promptBuffer).slice(0, -1).join('');
          if (this.loginPrompt === 'username') {
            this.terminal.write('\b \b');
          }
        }
        continue;
      }
      if (character < ' ' || character === '\u007f') {
        continue;
      }
      if (this.promptBuffer.length >= (this.loginPrompt === 'username' ? 64 : 4096)) {
        continue;
      }
      this.promptBuffer += character;
      if (this.loginPrompt === 'username' || this.loginPrompt === 'host-key') {
        this.terminal.write(character);
      }
    }
  }

  private finishLoginPrompt(): void {
    const value = this.promptBuffer;
    this.promptBuffer = '';
    if (this.loginPrompt === 'username') {
      const username = value.trim();
      if (!username || /[ \t\r\n]/.test(username)) {
        this.terminal.writeln(this.translations.translate('SSH_USERNAME_REQUIRED'));
        this.showLoginPrompt('username');
        return;
      }
      this.settings.username = username;
      this.continueLogin(this.socket);
      return;
    }
    if (this.loginPrompt === 'password') {
      if (!value) {
        this.terminal.writeln(this.translations.translate('SSH_PASSWORD_REQUIRED'));
        this.showLoginPrompt('password');
        return;
      }
      this.password = value;
      this.loginPrompt = null;
      this.sendConnectionRequest(this.socket);
      return;
    }
    if (this.loginPrompt === 'host-key') {
      const trust = ['yes', 'y'].includes(value.trim().toLowerCase());
      const socket = this.socket;
      this.loginPrompt = null;
      this.promptBuffer = '';
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        this.status.set(this.translations.translate('SSH_WEBSOCKET_OPEN_FAILED'));
        return;
      }
      if (!trust) {
        this.terminal.writeln(this.translations.translate('SSH_HOST_KEY_REJECTED'));
      }
      socket.send(JSON.stringify({ type: 'trust-host-key', trust }));
    }
  }

  private async continueLogin(socket?: WebSocket): Promise<void> {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      this.status.set(this.translations.translate('SSH_WEBSOCKET_OPEN_FAILED'));
      return;
    }
    try {
      await this.loadSavedCredentials();
    } catch (error) {
      console.error('No se pudieron recuperar las credenciales SSH guardadas.', error);
      this.status.set(`${this.translations.translate('SSH_ERROR_PREFIX')} ${this.translations.translate('SSH_CREDENTIALS_LOAD_FAILED')}`);
      socket.close();
      return;
    }
    if (this.settings.authMethod === 'publickey' && !this.privateKey.trim()) {
      this.status.set(`${this.translations.translate('SSH_ERROR_PREFIX')} ${this.translations.translate('SSH_PRIVATE_KEY_REQUIRED')}`);
      socket.close();
      return;
    }
    if (this.settings.authMethod === 'password' && !this.password) {
      this.showLoginPrompt('password');
      return;
    }
    this.loginPrompt = null;
    this.sendConnectionRequest(socket);
  }

  private showLoginPrompt(prompt: Exclude<LoginPrompt, null>): void {
    this.loginPrompt = prompt;
    this.promptBuffer = '';
    const promptKey = prompt === 'username' ? 'SSH_LOGIN_PROMPT' :
      prompt === 'password' ? 'SSH_PASSWORD_PROMPT' : 'SSH_HOST_KEY_CONFIRM_PROMPT';
    this.terminal.write(this.translations.translate(promptKey));
  }

  private sendConnectionRequest(socket?: WebSocket): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      this.loginPrompt = null;
      this.status.set(this.translations.translate('SSH_WEBSOCKET_OPEN_FAILED'));
      return;
    }
    socket.send(JSON.stringify({
      type: 'connect',
      ...this.settings,
      password: this.password,
      privateKey: this.privateKey,
      keyPassphrase: this.keyPassphrase,
      cols: this.terminal.cols,
      rows: this.terminal.rows
    }));
    this.clearCredentials();
  }

  private credentialTarget(): TerminalSettings {
    return {
      host: this.settings.host.trim(),
      port: this.settings.port,
      username: this.settings.username.trim(),
      authMethod: this.settings.authMethod
    };
  }

  private currentCredentials(): { password: string; privateKey: string; keyPassphrase: string } | null {
    if (this.settings.authMethod === 'password') {
      return this.password ? { password: this.password, privateKey: '', keyPassphrase: '' } : null;
    }
    return this.privateKey
      ? { password: '', privateKey: this.privateKey, keyPassphrase: this.keyPassphrase }
      : null;
  }

  private async storeCredentials(pending: PendingCredentialSave, replaceExisting: boolean): Promise<void> {
    try {
      await firstValueFrom(this.http.post<{ status: string }>(
        `${environment.apiUrl}/terminal/credentials/save`,
        { ...pending.target, ...pending.credentials, replaceExisting }
      ));
      this.pendingCredentialSave = null;
      this.replaceCredentialsConfirmation.set(false);
      this.clearCredentials();
      this.status.set(this.translations.translate('SSH_CREDENTIALS_SAVED'));
    } catch (error) {
      if (error instanceof HttpErrorResponse && error.status === 409 &&
          error.error?.code === 'terminal_credentials_exist' && !replaceExisting) {
        this.pendingCredentialSave = pending;
        this.replaceCredentialsConfirmation.set(true);
        return;
      }
      console.error('No se pudieron guardar las credenciales SSH cifradas.', error);
      this.status.set(this.translations.translate('SSH_CREDENTIALS_SAVE_FAILED'));
    }
  }

  private async loadSavedCredentials(): Promise<void> {
    const alreadyEntered = this.settings.authMethod === 'password'
      ? !!this.password
      : !!this.privateKey;
    if (alreadyEntered || !this.settings.username.trim()) {
      return;
    }
    const saved = await firstValueFrom(this.http.post<StoredTerminalCredentials>(
      `${environment.apiUrl}/terminal/credentials/get`,
      this.credentialTarget()
    ));
    if (!saved.found) {
      return;
    }
    this.clearCredentials();
    this.password = saved.password ?? '';
    this.privateKey = saved.privateKey ?? '';
    this.privateKeySelected = !!this.privateKey;
    this.keyPassphrase = saved.keyPassphrase ?? '';
  }

  private clearCredentials(): void {
    this.password = '';
    this.privateKey = '';
    this.privateKeySelected = false;
    this.keyPassphrase = '';
    this.showPassword = false;
    this.showKeyPassphrase = false;
    this.privateKeyReading = false;
  }

  private resizeTerminal(): void {
    const container = this.terminalContainer.nativeElement;
    if (container.clientWidth === 0 || container.clientHeight === 0) {
      return;
    }
    const cols = Math.max(2, Math.floor(container.clientWidth / 8.4));
    const rows = Math.max(2, Math.floor(container.clientHeight / 17));
    if (cols === this.terminal.cols && rows === this.terminal.rows) {
      return;
    }
    this.terminal.resize(cols, rows);
    if (this.connectionState() === 'connected' && this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ type: 'resize', cols, rows }));
    }
  }

  private isSettingsValid(): boolean {
    return !!this.settings.host.trim() && this.settings.host.trim().length <= 253 &&
      Number.isInteger(this.settings.port) && this.settings.port >= 1 && this.settings.port <= 65535 &&
      this.settings.username.trim().length <= 64 && !/[ \t\r\n]/.test(this.settings.username.trim()) &&
      (this.settings.authMethod === 'password' ||
       this.settings.authMethod === 'publickey');
  }

  private loadSettings(): void {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(settingsStorageKey) ?? 'null');
      if (typeof stored !== 'object' || stored === null) {
        return;
      }
      const candidate = stored as Partial<TerminalSettings>;
      if (typeof candidate.host === 'string' && typeof candidate.port === 'number' &&
          typeof candidate.username === 'string' &&
          (candidate.authMethod === 'password' || candidate.authMethod === 'publickey')) {
        this.settings = {
          host: candidate.host,
          port: candidate.port,
          username: candidate.username,
          authMethod: candidate.authMethod
        };
      } else {
        this.status.set(this.translations.translate('SSH_CONFIG_RESTORE_INVALID'));
      }
    } catch (error) {
      console.error('No se pudo leer la configuración SSH guardada.', error);
      this.status.set(this.translations.translate('SSH_CONFIG_RESTORE_FAILED'));
    }
  }
}
