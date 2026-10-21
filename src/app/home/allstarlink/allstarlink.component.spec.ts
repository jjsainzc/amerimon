import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AllstarlinkComponent } from './allstarlink.component';
import { environment } from '../../core/environment';
import { vi } from 'vitest';

const nodesUrl = `${environment.apiUrl}/allstarlink/nodes`;
const tablas = ['616265', '700001'].map(nodo_local => ({
  nodo_local, servidor_conectado: false, nodos_disponibles: []
}));

function descubrirServidores(http: HttpTestingController): void {
  http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
  http.expectOne(`${nodesUrl}?discovery=true`).flush({
    status: 'success',
    data: { count: 0, nodo_principal: '616265', tablas, resultados: [] }
  });
}

function respuestaServidor(local: string, conectado = true) {
  return {
    status: 'success',
    data: {
      count: conectado ? 1 : 0,
      tablas: [{ nodo_local: local, servidor_conectado: conectado, nodos_disponibles: [] }],
      resultados: conectado ? [{ nodo_local: local, nodo: '1964', protocolo: 'iax', enlace: 'ESTABLISHED' }] : []
    }
  };
}

describe('Allstarlink server selection', () => {
  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      imports: [AllstarlinkComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()]
    });
  });

  it('keeps private connections under their own servers without adding server options', async () => {
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    fixture.componentInstance.intervaloMs.set(0);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    descubrirServidores(http);
    http.expectOne(`${nodesUrl}?local_node=616265`).flush({
      status: 'success',
      data: {
        count: 3,
        nodo_principal: '616265',
        tablas: ['616265', '700001'].map(nodo_local => ({
          nodo_local, servidor_conectado: true, nodos_disponibles: []
        })),
        resultados: [
          { nodo_local: '616265', nodo: '1964', protocolo: 'iax', enlace: 'ESTABLISHED' },
          { nodo_local: '616265', nodo: '1998', protocolo: 'iax', enlace: 'ESTABLISHED' },
          { nodo_local: '700001', nodo: '1964', protocolo: 'iax', enlace: 'ESTABLISHED' }
        ]
      }
    });
    http.expectOne(`${nodesUrl}?local_node=700001`).flush(respuestaServidor('700001'));
    await fixture.whenStable();
    fixture.detectChanges();
    const element = fixture.nativeElement as HTMLElement;
    const selector = element.querySelector<HTMLSelectElement>('select[aria-label="Mostrar servidor"]')!;
    expect(Array.from(selector.options, option => option.value)).toEqual(['todos', '616265', '700001']);
    const main = element.querySelector('[aria-label="Conexiones del nodo principal 616265"]')!;
    const remote = element.querySelector('[aria-label="Conexiones del nodo remoto 700001"]')!;
    expect(element.querySelectorAll('.allstarlink-grupo')).toHaveLength(2);
    expect(main.textContent).toContain('1964');
    expect(main.textContent).toContain('1998');
    expect(remote.textContent).toContain('1964');
    selector.value = '700001';
    selector.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(element.querySelectorAll('.allstarlink-grupo')).toHaveLength(1);
    expect(element.querySelector('.allstarlink-grupo')?.getAttribute('aria-label'))
      .toBe('Conexiones del nodo remoto 700001');
    http.verify();
    fixture.destroy();
  });

  it('renders the primary node while another server is still waiting and allows refreshing', () => {
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    const component = fixture.componentInstance;
    component.intervaloMs.set(0);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    descubrirServidores(http);
    const remote = http.expectOne(`${nodesUrl}?local_node=700001`);
    http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265'));
    fixture.detectChanges();

    const element = fixture.nativeElement as HTMLElement;
    expect(component.cargando()).toBe(false);
    expect(component.error()).toBe('');
    expect(element.querySelector('[aria-label="Conexiones del nodo principal 616265"]')?.textContent).toContain('1964');
    expect(element.querySelector('[aria-label="Conexiones del nodo remoto 700001"]')?.textContent)
      .toContain('Conectando en segundo plano');
    expect(element.querySelector<HTMLButtonElement>('.allstarlink-actualizar')?.disabled).toBe(false);

    component.cargar();
    descubrirServidores(http);
    http.expectNone(`${nodesUrl}?local_node=700001`);
    http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265'));
    remote.flush(respuestaServidor('700001'));
    http.verify();
    fixture.destroy();
  });

  it('retries a failing secondary independently until it connects, even with polling disabled', () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    try {
      const component = fixture.componentInstance;
      component.intervaloMs.set(0);
      fixture.detectChanges();
      const http = TestBed.inject(HttpTestingController);
      descubrirServidores(http);
      http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265'));
      http.expectOne(`${nodesUrl}?local_node=700001`).flush(respuestaServidor('700001', false));
      fixture.detectChanges();
      expect(component.error()).toBe('');
      expect(component.erroresServidor()['700001']).toContain('700001');
      expect((fixture.nativeElement as HTMLElement).querySelector('.allstarlink > .allstarlink-error')).toBeNull();

      vi.advanceTimersByTime(5000);
      http.expectNone(`${nodesUrl}?local_node=616265`);
      http.expectOne(`${nodesUrl}?local_node=700001`).flush(
        { message: 'Servidor remoto no disponible' }, { status: 503, statusText: 'Unavailable' }
      );
      expect(component.error()).toBe('');
      vi.advanceTimersByTime(5000);
      http.expectOne(`${nodesUrl}?local_node=700001`).flush(respuestaServidor('700001'));
      expect(component.grupos().every(grupo => grupo.servidorConectado)).toBe(true);
      expect(component.erroresServidor()['700001']).toBeUndefined();
      vi.advanceTimersByTime(5000);
      http.expectNone(`${nodesUrl}?local_node=700001`);
      http.verify();
    } finally {
      fixture.destroy();
      vi.useRealTimers();
    }
  });

  it('shows only the primary connection failure globally and clears it after recovery', () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    try {
      const component = fixture.componentInstance;
      component.intervaloMs.set(0);
      fixture.detectChanges();
      const http = TestBed.inject(HttpTestingController);
      descubrirServidores(http);
      http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265', false));
      http.expectOne(`${nodesUrl}?local_node=700001`).flush(respuestaServidor('700001'));
      fixture.detectChanges();
      expect(component.error()).toContain('616265');
      expect(component.nodos().map(nodo => nodo.nodo_local)).toEqual(['700001']);
      expect((fixture.nativeElement as HTMLElement).querySelector('.allstarlink > .allstarlink-error')?.textContent)
        .toContain('616265');
      vi.advanceTimersByTime(5000);
      http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265'));
      expect(component.error()).toBe('');
      http.verify();
    } finally {
      fixture.destroy();
      vi.useRealTimers();
    }
  });

  it('times out a slow secondary without affecting primary data and cancels retries on destroy', () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    try {
      const component = fixture.componentInstance;
      component.intervaloMs.set(0);
      fixture.detectChanges();
      const http = TestBed.inject(HttpTestingController);
      descubrirServidores(http);
      const remote = http.expectOne(`${nodesUrl}?local_node=700001`);
      http.expectOne(`${nodesUrl}?local_node=616265`).flush(respuestaServidor('616265'));
      vi.advanceTimersByTime(30_000);
      expect(remote.cancelled).toBe(true);
      expect(component.error()).toBe('');
      expect(component.erroresServidor()['700001']).toContain('no respondió a tiempo');
      expect(component.nodos()).toHaveLength(1);
      fixture.destroy();
      vi.advanceTimersByTime(5000);
      http.expectNone(`${nodesUrl}?local_node=700001`);
      http.verify();
    } finally {
      fixture.destroy();
      vi.useRealTimers();
    }
  });

  it('queues a forced refresh only for servers already being queried and cancels active requests on destroy', () => {
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    const component = fixture.componentInstance;
    component.intervaloMs.set(0);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    descubrirServidores(http);
    const primary = http.expectOne(`${nodesUrl}?local_node=616265`);
    const remote = http.expectOne(`${nodesUrl}?local_node=700001`);
    component.cargar(true);
    descubrirServidores(http);
    http.expectNone(`${nodesUrl}?local_node=700001`);
    primary.flush(respuestaServidor('616265'));
    remote.flush(respuestaServidor('700001'));
    const refreshedPrimary = http.expectOne(`${nodesUrl}?local_node=616265`);
    const refreshedRemote = http.expectOne(`${nodesUrl}?local_node=700001`);
    fixture.destroy();
    expect(refreshedPrimary.cancelled).toBe(true);
    expect(refreshedRemote.cancelled).toBe(true);
    http.verify();
  });
});
