import { Injectable, signal } from '@angular/core';

export type CampoOrden = 'protocolo' | 'nodo' | 'callsign' | 'informacion' | 'ubicacion' | 'enlace' | 'direccion' | 'actividad' | 'conectado' | 'modo';

@Injectable({ providedIn: 'root' })
export class AllstarlinkViewStateService {
  readonly filtroBusqueda = signal('');
  readonly servidorSeleccionado = signal('todos');
  readonly campoOrden = signal<CampoOrden>('actividad');
  readonly direccionOrden = signal<'asc' | 'desc'>('desc');
  readonly intervaloMs = signal(15_000);
  readonly intervaloSeleccionadoManualmente = signal(false);
  readonly panelAdminAbierto = signal(false);
  readonly tablasContraidas = signal<ReadonlySet<string>>(new Set());
}
