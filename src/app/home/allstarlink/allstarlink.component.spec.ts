import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AllstarlinkComponent } from './allstarlink.component';
import { environment } from '../../core/environment';
import { vi } from 'vitest';
import { signal } from '@angular/core';
import { AuthService } from '../../core/services/auth.service';

const nodesUrl = `${environment.apiUrl}/allstarlink/nodes`;
const tablas = ['616265', '700001'].map(nodo_local => ({
  nodo_local, servidor_conectado: false, nodos_disponibles: []
}));

describe('Allstarlink connection target safety', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [AllstarlinkComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: AuthService, useValue: { esAdmin: signal(true) } }
      ]
    });
  });

  async function crearMonitor() {
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    const component = fixture.componentInstance;
    component.intervaloMs.set(0);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/asterisk/commands`).flush({
      status: 'success',
      data: [{ id: 'restart', label: 'Reiniciar', requires_remote_node: false, supports_permanent: false }]
    });
    http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
    http.expectOne(`${nodesUrl}?discovery=true`).flush({
      status: 'success',
      data: {
        nodo_principal: '616262',
        tablas: ['56502', '59201', '616262'].map(nodo_local => ({
          nodo_local, servidor_conectado: true, nodos_disponibles: []
        })),
        resultados: []
      }
    });
    for (const local of ['56502', '59201', '616262']) {
      http.expectOne(`${nodesUrl}?local_node=${local}`).flush(respuestaServidor(local));
    }
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, component, http, element: fixture.nativeElement as HTMLElement };
  }

  it('displays the actual local target after options arrive asynchronously, in both panels', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      const selector = element.querySelector<HTMLSelectElement>('.allstarlink-conexion select')!;
      expect(component.nodoLocal()).toBe('616262');
      expect(selector.value).toBe('616262');
      expect(selector.selectedOptions[0].textContent?.trim()).toBe('616262');
      component.panelAdminAbierto.set(true);
      fixture.detectChanges();
      await fixture.whenStable();
      const admin = element.querySelector<HTMLSelectElement>('#allstarlink-admin-contenido select[name="local_node"]')!;
      expect(admin.value).toBe('616262');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('confirms mute and unmute only after AMI acceptance and keeps failures unknown', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      const node = {
        nodo_local: '616262', nodo: '1964', protocolo: 'iax', enlace: 'ESTABLISHED',
        audio: { available: true, muted: null, connection_id: 'connection-1' }
      };
      component.nodos.set([node]);
      fixture.detectChanges();
      const button = element.querySelector<HTMLButtonElement>('.allstarlink-audio')!;
      expect(button.disabled).toBe(false);
      expect(button.classList.contains('audio-desconocido')).toBe(true);
      button.click();
      http.expectNone(request => request.method === 'POST');
      component.confirmarAccion();
      const mute = http.expectOne(`${environment.apiUrl}/asterisk/connections/audio`);
      expect(mute.request.body).toEqual({
        local_node: '616262', remote_node: '1964', connection_id: 'connection-1', muted: true
      });
      expect(component.nodos()[0].audio?.muted).toBeNull();
      mute.flush({ status: 'success', data: { available: true, muted: true, connection_id: 'connection-1' } });
      fixture.detectChanges();
      expect(button.classList.contains('muteado')).toBe(true);
      expect(button.getAttribute('aria-pressed')).toBe('true');
      button.click();
      component.confirmarAccion();
      const unmute = http.expectOne(`${environment.apiUrl}/asterisk/connections/audio`);
      expect(unmute.request.body.muted).toBe(false);
      unmute.flush({ status: 'success', data: { available: true, muted: false, connection_id: 'connection-1' } });
      fixture.detectChanges();
      expect(button.classList.contains('muteado')).toBe(false);
      button.click();
      component.confirmarAccion();
      http.expectOne(`${environment.apiUrl}/asterisk/connections/audio`).flush(
        { message: 'MuteAudio rechazado' }, { status: 502, statusText: 'Bad Gateway' }
      );
      expect(component.nodos()[0].audio?.muted).toBeNull();
      expect(component.resultadoPopup()?.error).toBe(true);
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('disables mute without support and never exposes it on remote servers', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      component.nodos.set([
        { nodo_local: '616262', nodo: '1964', protocolo: 'iax', audio: { available: false, muted: null, reason: 'MuteAudio no disponible' } },
        { nodo_local: '56502', nodo: '1998', protocolo: 'iax', audio: { available: true, muted: false, connection_id: 'remote' } }
      ]);
      fixture.detectChanges();
      const buttons = element.querySelectorAll<HTMLButtonElement>('.allstarlink-audio');
      expect(buttons.length).toBe(1);
      expect(buttons[0].disabled).toBe(true);
      expect(buttons[0].title).toBe('MuteAudio no disponible');
      component.alternarAudio(component.nodos()[1]);
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('preserves every connection parameter when the view is destroyed and recreated', async () => {
    const primera = await crearMonitor();
    primera.component.nodoLocal.set('56502');
    primera.component.nodoRemotoConexion.set('46818');
    primera.component.modoConexion.set('localmonitor');
    primera.component.permanenteConexion.set(true);
    primera.component.desconectarAntesConexion.set(true);
    primera.component.timeoutConexion.set(42);
    primera.fixture.destroy();

    const { fixture, component, http, element } = await crearMonitor();
    try {
      expect(component.nodoLocal()).toBe('56502');
      expect(component.nodoRemotoConexion()).toBe('46818');
      expect(component.modoConexion()).toBe('localmonitor');
      expect(component.permanenteConexion()).toBe(true);
      expect(component.desconectarAntesConexion()).toBe(true);
      expect(component.timeoutConexion()).toBe(42);
      expect(element.querySelector<HTMLSelectElement>('.allstarlink-conexion select[name="local_node"]')!.value).toBe('56502');
      expect(element.querySelector<HTMLInputElement>('.allstarlink-nodo-fila input')!.value).toBe('46818');
      expect(element.querySelector<HTMLInputElement>('.allstarlink-desconectar-antes input')!.checked).toBe(true);
      expect(component.conectando()).toBe(false);
      expect(component.confirmacionPendiente()).toBeNull();
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('falls back to the available primary server if the remembered server no longer exists', async () => {
    const primera = await crearMonitor();
    primera.component.nodoLocal.set('999999');
    primera.component.nodoRemotoConexion.set('46818');
    primera.fixture.destroy();
    const { fixture, component, http } = await crearMonitor();
    try {
      expect(component.nodoLocal()).toBe('616262');
      expect(component.nodoRemotoConexion()).toBe('46818');
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('sends 46818 to the selected 56502 and freezes all confirmed parameters', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      const selector = element.querySelector<HTMLSelectElement>('.allstarlink-conexion select')!;
      selector.value = '56502';
      selector.dispatchEvent(new Event('change'));
      component.nodoRemotoConexion.set('46818');
      component.modoConexion.set('localmonitor');
      fixture.detectChanges();
      element.querySelector<HTMLFormElement>('.allstarlink-conexion form')!.dispatchEvent(
        new Event('submit', { cancelable: true })
      );
      expect(component.confirmacionPendiente()?.mensaje)
        .toBe('Conectar el nodo 46818 a 56502 en modo Local Monitor (temporal).');
      http.expectNone(request => request.method === 'POST');
      component.nodoLocal.set('616262');
      component.nodoRemotoConexion.set('99999');
      component.modoConexion.set('connect');
      component.permanenteConexion.set(true);
      component.timeoutConexion.set(60);
      component.confirmarAccion();
      const request = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(request.request.body).toEqual({
        local_node: '56502', remote_node: '46818', action: 'localmonitor',
        permanent: false, timeout_seconds: 15
      });
      request.flush({ status: 'error', message: 'Respuesta simulada; no se ejecutan conexiones reales.' });
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('does not execute a cancelled connection', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.conectarNodo();
      component.cancelarConfirmacion();
      component.confirmarAccion();
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('binds the disconnect checkbox and freezes it with the selected server at confirmation', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      const checkbox = element.querySelector<HTMLInputElement>('.allstarlink-desconectar-antes input')!;
      expect(checkbox.checked).toBe(false);
      checkbox.checked = true;
      checkbox.dispatchEvent(new Event('change'));
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.modoConexion.set('localmonitor');
      component.timeoutConexion.set(42);
      component.conectarNodo();
      expect(component.confirmacionPendiente()?.mensaje).toContain('Se desconectarán todos los nodos del servidor 56502.');
      http.expectNone(request => request.method === 'POST');
      component.desconectarAntesConexion.set(false);
      component.nodoLocal.set('616262');
      component.nodoRemotoConexion.set('99999');
      component.modoConexion.set('connect');
      component.permanenteConexion.set(true);
      component.timeoutConexion.set(5);
      component.confirmarAccion();
      expect(component.conexionesPendientes()).toEqual([]);
      const lista = respuestaServidor('56502');
      lista.data.resultados = [
        { nodo_local: '56502', nodo: '1964', protocolo: 'iax', enlace: 'ESTABLISHED' },
        { nodo_local: '56502', nodo: '1998', protocolo: 'iax', enlace: 'ESTABLISHED' },
        { nodo_local: '616262', nodo: '8888', protocolo: 'iax', enlace: 'ESTABLISHED' }
      ];
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(lista);
      for (const remoto of ['1964', '1998']) {
        const desconexion = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
        expect(desconexion.request.body).toEqual({
          local_node: '56502', remote_node: remoto, action: 'disconnect', permanent: false
        });
        desconexion.flush({ status: 'success', data: { command: 'disconnect', lines: [] } });
      }
      http.expectNone(request => request.method === 'POST');
      lista.data.resultados = [];
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(lista);
      const request = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(request.request.body).toEqual({
        local_node: '56502', remote_node: '46818', action: 'localmonitor',
        permanent: false, timeout_seconds: 42
      });
      request.flush({ status: 'error', message: 'No se confirmó la desconexión previa.' });
      expect(component.resultadoConexion()?.error).toBe(true);
      expect(component.conexionesPendientes()).toEqual([]);
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('stops before connecting when an individual disconnection fails', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.confirmarAccion();
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502'));
      const desconexion = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(desconexion.request.body.action).toBe('disconnect');
      desconexion.flush({ status: 'error', message: 'Desconexión rechazada.' });
      http.expectNone(request => request.method === 'POST');
      expect(component.resultadoConexion()).toEqual({ texto: 'Desconexión rechazada.', error: true });
      expect(component.conectando()).toBe(false);
      expect(component.conexionesPendientes()).toEqual([]);
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('waits for an empty list before connecting with permanent mode preserved', async () => {
    const { fixture, component, http } = await crearMonitor();
    vi.useFakeTimers();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.modoConexion.set('monitor');
      component.permanenteConexion.set(true);
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.confirmarAccion();
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502'));
      http.expectOne(`${environment.apiUrl}/asterisk/commands`).flush({ status: 'success' });
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502'));
      http.expectNone(request => request.method === 'POST');
      expect(component.desconectandoAntesConexion()).toBe(true);
      await vi.advanceTimersByTimeAsync(500);
      const vacia = respuestaServidor('56502');
      vacia.data.resultados = [];
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(vacia);
      const conexion = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(conexion.request.body).toEqual({
        local_node: '56502', remote_node: '46818', action: 'monitor',
        permanent: true, timeout_seconds: 15
      });
      conexion.flush({ status: 'success', data: { command: 'connect', lines: [] } });
      expect(component.desconectandoAntesConexion()).toBe(false);
      http.verify();
    } finally {
      fixture.destroy();
      vi.useRealTimers();
    }
  });

  it('does not treat an unreachable server as an empty list', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.confirmarAccion();
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502', false));
      http.expectNone(request => request.method === 'POST');
      expect(component.resultadoConexion()?.error).toBe(true);
      expect(component.conectando()).toBe(false);
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('connects without sending disconnect commands when the selected server is already empty', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.confirmarAccion();
      const vacia = respuestaServidor('56502');
      vacia.data.resultados = [];
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(vacia);
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(vacia);
      const conexion = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(conexion.request.body.action).toBe('connect');
      conexion.flush({ status: 'success', data: { command: 'connect', lines: [] } });
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('never connects if disconnection cannot be confirmed within the deadline', async () => {
    const { fixture, component, http } = await crearMonitor();
    vi.useFakeTimers();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.confirmarAccion();
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502'));
      http.expectOne(`${environment.apiUrl}/asterisk/commands`).flush({ status: 'success' });
      http.expectOne(`${nodesUrl}?local_node=56502`).flush(respuestaServidor('56502'));
      await vi.advanceTimersByTimeAsync(500);
      const pendiente = http.expectOne(`${nodesUrl}?local_node=56502`);
      await vi.advanceTimersByTimeAsync(14_500);
      expect(pendiente.cancelled).toBe(true);
      expect(component.conectando()).toBe(false);
      expect(component.resultadoConexion()?.error).toBe(true);
      expect(component.conexionesPendientes()).toEqual([]);
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
      vi.useRealTimers();
    }
  });

  it('does not disconnect anything when the replacement connection is cancelled', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoRemotoConexion.set('46818');
      component.desconectarAntesConexion.set(true);
      component.conectarNodo();
      component.cancelarConfirmacion();
      component.confirmarAccion();
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('keeps both selectors synchronized when options are replaced without changing the chosen node', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      component.panelAdminAbierto.set(true);
      fixture.detectChanges();
      await fixture.whenStable();
      const selector = element.querySelector<HTMLSelectElement>('.allstarlink-conexion select')!;
      const admin = element.querySelector<HTMLSelectElement>('#allstarlink-admin-contenido select[name="local_node"]')!;
      selector.value = '56502';
      selector.dispatchEvent(new Event('change'));
      await fixture.whenStable();
      expect(admin.value).toBe('56502');
      component.tablasLocales.update(tablas => [
        { nodo_local: '10000', servidor_conectado: true, nodos_disponibles: [] },
        ...tablas.slice().reverse()
      ]);
      fixture.detectChanges();
      await fixture.whenStable();
      expect(selector.value).toBe('56502');
      expect(admin.value).toBe('56502');
      expect(component.nodoLocal()).toBe('56502');
      admin.value = '59201';
      admin.dispatchEvent(new Event('change'));
      await fixture.whenStable();
      expect(component.nodoLocal()).toBe('59201');
      expect(selector.value).toBe('59201');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('executes the administrative operation on the node shown in its confirmation', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.ejecutarComando();
      expect(component.confirmacionPendiente()?.mensaje).toContain('56502');
      component.nodoLocal.set('616262');
      component.accionSeleccionada.set('other_command');
      component.confirmarAccion();
      const request = http.expectOne(`${environment.apiUrl}/asterisk/commands`);
      expect(request.request.body).toEqual({
        local_node: '56502', action: 'restart', permanent: false
      });
      request.flush({ status: 'success', data: { command: 'simulated', lines: [] } });
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('rejects a local node not present in the server list without offering confirmation', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('999999');
      component.nodoRemotoConexion.set('46818');
      component.conectarNodo();
      expect(component.confirmacionPendiente()).toBeNull();
      expect(component.resultadoPopup()?.error).toBe(true);
      expect(component.resultadoPopup()?.lineas.join(' ')).toContain('999999');
      http.expectNone(request => request.method === 'POST');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('blocks a confirmed connection if its local server is no longer available', async () => {
    const { fixture, component, http } = await crearMonitor();
    try {
      component.nodoLocal.set('56502');
      component.nodoRemotoConexion.set('46818');
      component.conectarNodo();
      component.tablasLocales.update(tablas => tablas.filter(tabla => tabla.nodo_local !== '56502'));
      component.nodos.update(nodos => nodos.filter(nodo => nodo.nodo_local !== '56502'));
      component.confirmarAccion();
      http.expectNone(request => request.method === 'POST');
      expect(component.resultadoPopup()?.error).toBe(true);
      expect(component.resultadoPopup()?.lineas.join(' ')).toContain('56502');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('clears every node table when server discovery returns an error response', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      component.cargar();
      http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
      http.expectOne(`${nodesUrl}?discovery=true`).flush({
        status: 'error',
        message: 'No se pudo cargar el listado de nodos.'
      });
      fixture.detectChanges();

      expect(component.tablasLocales()).toEqual([]);
      expect(component.nodos()).toEqual([]);
      expect(component.grupos()).toEqual([]);
      expect(element.querySelectorAll('.allstarlink-tabla')).toHaveLength(0);
      expect(element.querySelector('.allstarlink-error')?.textContent).toContain('No se pudo cargar');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });

  it('clears every node table when server discovery fails at the network level', async () => {
    const { fixture, component, http, element } = await crearMonitor();
    try {
      component.cargar();
      http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
      http.expectOne(`${nodesUrl}?discovery=true`).flush(
        { message: 'Servicio no disponible' },
        { status: 503, statusText: 'Unavailable' }
      );
      fixture.detectChanges();

      expect(component.tablasLocales()).toEqual([]);
      expect(component.nodos()).toEqual([]);
      expect(component.grupos()).toEqual([]);
      expect(element.querySelectorAll('.allstarlink-tabla')).toHaveLength(0);
      expect(element.querySelector('.allstarlink-error')?.textContent).toContain('Servicio no disponible');
      http.verify();
    } finally {
      fixture.destroy();
    }
  });
});

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

  it('puts the primary table first and shows remote metadata even before AMI responds', async () => {
    const fixture = TestBed.createComponent(AllstarlinkComponent);
    const component = fixture.componentInstance;
    component.intervaloMs.set(0);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
    http.expectOne(`${nodesUrl}?discovery=true`).flush({
      status: 'success',
      data: {
        nodo_principal: '700003',
        tablas: [
          { nodo_local: '700002', descripcion: 'Radio Madrid', hostname: 'radio.example.org', servidor_conectado: false, nodos_disponibles: [] },
          { nodo_local: '700001', descripcion: 'Radio local', ip: '192.0.2.10', servidor_conectado: false, nodos_disponibles: [] },
          { nodo_local: '700003', descripcion: 'Radio principal', ip: '192.0.2.20', servidor_conectado: false, nodos_disponibles: [] },
          { nodo_local: '700004', servidor_conectado: false, nodos_disponibles: [] }
        ],
        resultados: []
      }
    });
    await fixture.whenStable();
    fixture.detectChanges();
    const element = fixture.nativeElement as HTMLElement;
    const titulos = Array.from(element.querySelectorAll('.allstarlink-grupo-titulo'), titulo => titulo.textContent?.trim());
    expect(titulos).toEqual(['700003 - Radio principal - 192.0.2.20', '700001 - Radio local - 192.0.2.10', '700002 - Radio Madrid - radio.example.org', '700004']);
    expect(Array.from(element.querySelectorAll('.allstarlink-grupo-titulo strong'), nodo => nodo.textContent))
      .toEqual(['700003', '700001', '700002', '700004']);
    const detalles = Array.from(element.querySelectorAll('.allstarlink-grupo-detalle'));
    expect(detalles).toHaveLength(3);
    for (const detalle of detalles) {
      expect(getComputedStyle(detalle).fontWeight).toBe('400');
      expect(detalle.querySelector('strong, b')).toBeNull();
    }
    expect(element.querySelectorAll('h4')[0].textContent).toContain('Nodo principal');
    expect(Array.from(element.querySelectorAll('h4')).some(titulo => titulo.textContent?.includes('Nodo remoto'))).toBe(false);
    component.nodoPrincipal.set('700004');
    expect(component.grupos().map(grupo => grupo.nodoLocal)).toEqual(['700004', '700001', '700002', '700003']);
    for (const local of ['700001', '700002', '700003', '700004']) {
      http.expectOne(`${nodesUrl}?local_node=${local}`).flush(respuestaServidor(local));
    }
    http.verify();
    fixture.destroy();
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
