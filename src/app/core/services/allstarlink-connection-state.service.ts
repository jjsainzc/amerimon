import { Injectable, signal } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class AllstarlinkConnectionStateService {
  readonly nodoLocal = signal('');
  readonly nodoRemoto = signal('');
  readonly modo = signal<'connect' | 'monitor' | 'localmonitor'>('connect');
  readonly permanente = signal(false);
  readonly desconectarAntes = signal(false);
  readonly timeout = signal(15);
}
