import { HttpClient } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { AuthService } from '../../core/services/auth.service';
import { TranslationService } from '../../core/services/translation.service';
import { TerminalComponent } from './terminal.component';

const terminalMock = vi.hoisted(() => ({
  instance: null as {
    emitData: (data: string) => void;
    write: (data: string) => void;
    writes: string[];
    resetCount: number;
  } | null
}));

vi.mock('xterm', () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    writes: string[] = [];
    resetCount = 0;
    private dataHandler?: (data: string) => void;
    constructor() {
      terminalMock.instance = this;
    }
    open(): void {}
    writeln(data: string): void { this.writes.push(`${data}\r\n`); }
    onData(handler: (data: string) => void): { dispose(): void } {
      this.dataHandler = handler;
      return { dispose() {} };
    }
    emitData(data: string): void { this.dataHandler?.(data); }
    clear(): void { this.writes.length = 0; }
    reset(): void {
      this.resetCount++;
      this.writes.length = 0;
    }
    write(data: string): void { this.writes.push(data); }
    resize(cols: number, rows: number): void { this.cols = cols; this.rows = rows; }
    dispose(): void {}
  }
}));

class FakeWebSocket {
  static readonly OPEN = 1;
  readyState = 0;
  binaryType = '';
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string | ArrayBuffer>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  readonly sent: string[] = [];

  constructor(readonly url: string) {
    fakeSocket = this;
  }

  send(data: string): void { this.sent.push(data); }
  close(code = 1000): void {
    this.readyState = 3;
    this.onclose?.(new CloseEvent('close', { code }));
  }
  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }
}

let fakeSocket: FakeWebSocket | undefined;

describe('TerminalComponent', () => {
  let fixture: ComponentFixture<TerminalComponent>;
  let component: TerminalComponent;

  it('enables settings save only after changes and resets after saving', async () => {
    createComponent();
    await fixture.whenStable();
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('.secondary-button');
    expect(save.disabled).toBe(true);
    const original = component.settings.host;
    const host: HTMLInputElement = fixture.nativeElement.querySelector('input[name="host"]');
    host.value = '192.0.2.10';
    host.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    host.value = original;
    host.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    component.settings.host = '192.0.2.11';
    await component.saveSettings();
    fixture.changeDetectorRef.markForCheck();
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    component.settings.username = 'operator';
    component.password = 'new-password';
    await component.saveSettings();
    fixture.detectChanges();
    expect(component.password).toBe('');
    expect(save.disabled).toBe(true);
  });

  beforeEach(async () => {
    localStorage.clear();
    vi.stubGlobal('ResizeObserver', class {
      observe(): void {}
      disconnect(): void {}
    });
    await TestBed.configureTestingModule({
      imports: [TerminalComponent],
      providers: [
        { provide: AuthService, useValue: { getToken: () => 'admin-token' } },
        {
          provide: HttpClient,
          useValue: {
            post: vi.fn((url: string) => of(
              url.endsWith('/credentials/get') ? { found: false } : { status: 'ok' }
            ))
          }
        },
        {
          provide: TranslationService,
          useValue: {
            translate: (key: string, params?: Record<string, string | number>) =>
              key + (params ? ` ${JSON.stringify(params)}` : ''),
            revision: () => 0
          }
        }
      ]
    }).compileComponents();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    terminalMock.instance = null;
    fakeSocket = undefined;
    localStorage.clear();
  });

  function createComponent(): void {
    fixture = TestBed.createComponent(TerminalComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  }

  it('defaults to the backend host so SSH does not need to be exposed publicly', () => {
    createComponent();
    expect(component).toBeTruthy();
    expect(component.settings.host).toBe('127.0.0.1');
    expect(component.settings.port).toBe(22);
  });

  it('restores saved connection settings without persisting credentials in browser storage', () => {
    localStorage.setItem('amerimon.ssh-terminal.settings', JSON.stringify({
      host: 'server.example',
      port: 2222,
      username: 'operator',
      authMethod: 'publickey'
    }));
    createComponent();

    expect(component.settings.host).toBe('server.example');
    expect(component.settings.port).toBe(2222);
    component.password = 'temporary-password';
    component.keyPassphrase = 'temporary-passphrase';
    void component.saveSettings();

    const saved = JSON.parse(localStorage.getItem('amerimon.ssh-terminal.settings') ?? '{}');
    expect(saved.username).toBe('operator');
    expect(saved.password).toBeUndefined();
    expect(saved.keyPassphrase).toBeUndefined();
  });

  it('prompts for missing login and password inside the terminal without echoing the password', async () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.settings.username = '';

    component.connect();
    const socket = fakeSocket;
    expect(socket).toBeDefined();
    if (!socket) {
      return;
    }
    socket.open();
    expect(terminalMock.instance?.writes.join('')).toContain('SSH_LOGIN_PROMPT');

    terminalMock.instance?.emitData('operator\r');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(terminalMock.instance?.writes.join('')).toContain('SSH_PASSWORD_PROMPT');
    terminalMock.instance?.emitData('s3cret\r');

    expect(socket.sent).toHaveLength(1);
    expect(JSON.parse(socket.sent[0])).toMatchObject({
      type: 'connect',
      username: 'operator',
      password: 's3cret'
    });
    expect(terminalMock.instance?.writes.join('')).not.toContain('s3cret');
  });

  it('explains an unexpected WebSocket close instead of reporting a normal disconnect', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.connect();
    fakeSocket?.close(1006);

    expect(component.status()).toContain('SSH_WEBSOCKET_CLOSED_UNEXPECTEDLY');
    expect(component.status()).toContain('1006');
  });

  it('reports a normal shell exit as disconnected without a WebSocket error', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.connect();
    fakeSocket?.close(1000);

    expect(component.connectionState()).toBe('disconnected');
    expect(component.status()).toBe('SSH_DISCONNECTED');
    expect(terminalMock.instance?.writes.join('')).not.toContain('SSH_WEBSOCKET_CLOSED_UNEXPECTEDLY');
  });

  it('clears the terminal when disconnect is requested', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.settings.username = 'operator';
    component.connect();
    const socket = fakeSocket;
    socket?.open();
    socket?.onmessage?.({ data: 'terminal output' } as MessageEvent<string>);

    expect(terminalMock.instance?.writes.join('')).toContain('terminal output');
    component.disconnect();

    expect(terminalMock.instance?.writes.join('')).not.toContain('terminal output');
    expect(terminalMock.instance?.writes.join('')).toContain('\x1b[3J\x1b[2J\x1b[H');
    expect(terminalMock.instance?.resetCount).toBe(1);
    expect(socket?.onmessage).toBeNull();
    expect(component.status()).toBe('SSH_DISCONNECTED');
  });

  it('resets the terminal even when the WebSocket is already closed', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.connect();
    fakeSocket?.close();
    terminalMock.instance?.write('old terminal scrollback');

    component.disconnect();

    expect(terminalMock.instance?.writes.join('')).not.toContain('old terminal scrollback');
    expect(terminalMock.instance?.writes.join('')).toContain('\x1b[3J\x1b[2J\x1b[H');
    expect(terminalMock.instance?.resetCount).toBe(1);
  });

  it('shows and hides the SSH password with an accessible control', () => {
    createComponent();
    const passwordInput = fixture.nativeElement.querySelector(
      '#sshPassword'
    ) as HTMLInputElement;
    const passwordToggle = fixture.nativeElement.querySelector(
      '[aria-label="SSH_SHOW_SECRET"]'
    ) as HTMLButtonElement;

    expect(passwordInput.type).toBe('password');
    passwordToggle.click();
    fixture.detectChanges();
    expect(passwordInput.type).toBe('text');
    expect(passwordToggle.getAttribute('aria-pressed')).toBe('true');
  });

  it('shows and hides the key passphrase with an accessible control', () => {
    localStorage.setItem('amerimon.ssh-terminal.settings', JSON.stringify({
      host: 'server.example',
      port: 2222,
      username: 'operator',
      authMethod: 'publickey'
    }));
    createComponent();
    const passphraseInput = fixture.nativeElement.querySelector(
      '#sshKeyPassphrase'
    ) as HTMLInputElement;
    const passphraseToggle = fixture.nativeElement.querySelector(
      '[aria-label="SSH_SHOW_SECRET"]'
    ) as HTMLButtonElement;

    expect(passphraseInput.type).toBe('password');
    passphraseToggle.click();
    fixture.detectChanges();
    expect(passphraseInput.type).toBe('text');
  });

  it('requires explicit confirmation before trusting an unknown SSH host key', async () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.settings.username = 'operator';
    component.password = 'secret';
    component.connect();
    const socket = fakeSocket;
    expect(socket).toBeDefined();
    if (!socket) {
      return;
    }

    socket.open();
    await new Promise((resolve) => setTimeout(resolve, 0));
    socket.onmessage?.({
      data: JSON.stringify({
        type: 'host-key-required',
        host: '127.0.0.1:22',
        algorithm: 'ssh-ed25519',
        fingerprint: 'SHA256:example'
      })
    } as MessageEvent<string>);

    expect(terminalMock.instance?.writes.join('')).toContain('SHA256:example');
    expect(terminalMock.instance?.writes.join('')).toContain('SSH_HOST_KEY_CONFIRM_PROMPT');
    terminalMock.instance?.emitData('yes\r');
    expect(JSON.parse(socket.sent[1])).toEqual({ type: 'trust-host-key', trust: true });
  });

  it('stores the selected private key only through the encrypted backend endpoint', async () => {
    vi.stubGlobal('WebSocket', FakeWebSocket);
    createComponent();
    component.settings.username = 'operator';
    component.settings.authMethod = 'publickey';
    component.keyPassphrase = 'private-key-passphrase';
    const file = {
      size: 32,
      text: vi.fn().mockResolvedValue('private-key-material')
    };
    const input = {
      files: [file],
      value: 'C:\\fakepath\\id_ed25519'
    };
    await component.selectPrivateKey({ target: input } as unknown as Event);
    expect(input.value).toBe('');
    expect(component['privateKeySelected']).toBe(true);

    await component.saveSettings();

    const saved = JSON.parse(localStorage.getItem('amerimon.ssh-terminal.settings') ?? '{}');
    expect(saved.privateKey).toBeUndefined();
    const http = TestBed.inject(HttpClient);
    expect(vi.mocked(http.post)).toHaveBeenCalledWith(
      '/mon/terminal/credentials/save',
      expect.objectContaining({
        privateKey: 'private-key-material',
        keyPassphrase: 'private-key-passphrase',
        username: 'operator'
      })
    );
    expect(component['privateKey']).toBe('');
    expect(component.keyPassphrase).toBe('');
  });
});
