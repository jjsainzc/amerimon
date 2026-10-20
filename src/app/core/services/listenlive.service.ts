import { Injectable, inject, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { environment } from '../environment';
import { AuthService } from './auth.service';
import { ListenLiveSessionsService } from './listenlive-sessions.service';

/**
 * Escucha en vivo de una extensión PJSIP.
 * 1) Crea una sesión y POST {apiUrl}/listen-live/start {ext, uuid} para iniciar Asterisk.
 * 2) WS {wsUrl}/listen-live/ws?uuid=&token= recibe PCM SLIN16 a 8 kHz en tramos de 320 bytes.
 */
@Injectable({
  providedIn: 'root'
})
export class ListenLiveService {
  private readonly http = inject(HttpClient);
  private readonly auth = inject(AuthService);
  private readonly sessions = inject(ListenLiveSessionsService);

  private ws?: WebSocket;
  private audioCtx?: AudioContext;
  private nextStartTime = 0;
  private activeSessionId: number | null = null;

  public isListening = signal<boolean>(false);
  public connecting = signal<boolean>(false);
  public error = signal<string | null>(null);
  public asteriskStatus = signal<'checking' | 'connected' | 'disconnected' | 'unavailable'>('checking');
  public audioLevelDb = signal(-60);
  public sessionRevision = signal(0);

  /** Comprueba la conexión con Asterisk y distingue desconexión de indisponibilidad HTTP. */
  public async checkAsteriskConnection(): Promise<void> {
    this.asteriskStatus.set('checking');
    try {
      const response = await firstValueFrom(
        this.http.get<{ connected: boolean }>(`${environment.apiUrl}/listen-live/status`)
      );
      this.asteriskStatus.set(response.connected ? 'connected' : 'disconnected');
    } catch (error) {
      this.asteriskStatus.set(error instanceof HttpErrorResponse && error.status === 503 ? 'disconnected' : 'unavailable');
    }
  }

  /**
   * Detiene la escucha previa, registra la sesión e inicia audio de la extensión por WebSocket.
   * Crea el contexto de audio antes del primer await para conservar el gesto del usuario.
   */
  public async startListening(ext: string): Promise<void> {
    const previousStop = this.stopListening();
    this.error.set(null);
    this.connecting.set(true);

    const uuid = this.crearUUID();

    // Se crea antes del await para conservar el gesto del usuario (política de autoplay).
    const ctx = new (window.AudioContext || (window as any).webkitAudioContext)({ sampleRate: 8000 });
    this.audioCtx = ctx;
    void ctx.resume();
    await previousStop;

    try {
      const session = await this.sessions.crear();
      this.activeSessionId = session.listenliveid;
      await firstValueFrom(
        this.http.post(`${environment.apiUrl}/listen-live/start`, { ext, uuid, listenliveid: session.listenliveid })
      );
    } catch (e: any) {
      await this.stopListening();
      this.error.set(e?.error?.message ?? 'No se pudo iniciar la escucha');
      return;
    }

    const token = this.auth.getToken();
    let wsUrl = `${environment.wsUrl}/listen-live/ws?uuid=${encodeURIComponent(uuid)}`;
    if (token) {
      wsUrl += `&token=${encodeURIComponent(token)}`;
    }

    let ws: WebSocket;
    try {
      ws = new WebSocket(wsUrl);
    } catch {
      await this.stopListening();
      this.error.set('No se pudo abrir la conexión de audio');
      return;
    }
    ws.binaryType = 'arraybuffer';
    this.ws = ws;

    await new Promise<void>((resolve) => {
      ws.onopen = () => {
        this.connecting.set(false);
        this.isListening.set(true);
        resolve();
      };

      ws.onmessage = (event: MessageEvent) => {
        if (this.ws !== ws) { return; }
        try {
          this.playChunk(event.data as ArrayBuffer);
        } catch {
          this.error.set('No se pudo procesar el audio recibido');
          void this.stopListening().finally(resolve);
        }
      };

      ws.onerror = () => {
        if (this.ws !== ws) { return; }
        this.error.set('Error en la conexión de audio');
        void this.stopListening().finally(() => resolve());
      };

      ws.onclose = () => {
        if (this.ws === ws) {
          void this.stopListening().finally(resolve);
        } else {
          resolve();
        }
      };
    });
  }

  /** Convierte PCM de 16 bits a audio mono de 8 kHz, actualiza el medidor y programa su reproducción. */
  private playChunk(arrayBuffer: ArrayBuffer) {
    if (!this.audioCtx || arrayBuffer.byteLength < 2) return;

    const pcm16 = new Int16Array(arrayBuffer, 0, arrayBuffer.byteLength >> 1);
    const float32 = new Float32Array(pcm16.length);
    let sumaCuadrados = 0;

    for (let i = 0; i < pcm16.length; i++) {
      float32[i] = pcm16[i] / 32768;
      sumaCuadrados += float32[i] * float32[i];
    }

    const rms = Math.sqrt(sumaCuadrados / float32.length);
    const dbfs = rms > 0 ? 20 * Math.log10(rms) : -60;
    this.audioLevelDb.set(Math.max(-60, Math.min(0, dbfs)));

    const audioBuffer = this.audioCtx.createBuffer(1, float32.length, 8000);
    audioBuffer.getChannelData(0).set(float32);

    const source = this.audioCtx.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(this.audioCtx.destination);

    // Colchón inicial para absorber jitter de red.
    const now = this.audioCtx.currentTime;
    if (this.nextStartTime < now) {
      this.nextStartTime = now + 0.1;
    }

    source.start(this.nextStartTime);
    this.nextStartTime += audioBuffer.duration;
  }

  /** Libera el WebSocket y el audio, reinicia indicadores y cierra el registro de sesión activo. */
  public async stopListening(): Promise<void> {
    const sessionId = this.activeSessionId;
    this.activeSessionId = null;
    const ws = this.ws;
    this.ws = undefined;
    ws?.close();
    this.audioCtx?.close();
    this.audioCtx = undefined;
    this.isListening.set(false);
    this.connecting.set(false);
    this.audioLevelDb.set(-60);
    this.nextStartTime = 0;

    if (sessionId !== null) {
      try {
        await this.sessions.cerrar(sessionId);
        this.sessionRevision.update(revision => revision + 1);
      } catch {
        this.error.set('La escucha se detuvo, pero no se pudo cerrar su registro');
      }
    }
  }

  /** Genera un identificador con formato UUID v4, con alternativa si Web Crypto no está disponible. */
  private crearUUID(): string {
    const cryptoAPI = globalThis.crypto;
    if (typeof cryptoAPI?.randomUUID === 'function') {
      return cryptoAPI.randomUUID();
    }

    const bytes = new Uint8Array(16);
    try {
      cryptoAPI?.getRandomValues(bytes);
    } catch {
      for (let index = 0; index < bytes.length; index++) {
        bytes[index] = Math.floor(Math.random() * 256);
      }
    }
    if (!cryptoAPI?.getRandomValues) {
      for (let index = 0; index < bytes.length; index++) {
        bytes[index] = Math.floor(Math.random() * 256);
      }
    }

    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
}
