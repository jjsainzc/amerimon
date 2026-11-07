import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { NodeReconnection, NodeReconnectionsService } from '../../../core/services/node-reconnections.service';
import { TranslationService } from '../../../core/services/translation.service';
import { NodeReconnectionsComponent } from './node-reconnections.component';
import { AllstarlinkService } from '../../../core/services/allstarlink.service';

describe('Automatic node reconnection', () => {
  const entry: NodeReconnection = {
    id: 1, source_node: '50001', node: '3123456', enabled: true, interval_seconds: 60, action: 'monitor', permanent: true,
    next_check_at: 1700000060, last_check_at: 1700000000, last_result: 'RECONNECT_PRESENT', last_details: ''
  };

  function setup() {
    const service = {
      listar: vi.fn(() => of({ entries: [{ ...entry }] })),
      guardar: vi.fn(() => of({ status: 'ok' })),
      eliminar: vi.fn(() => of({ status: 'ok' }))
    };
    const allstarlink = {
      listarServidores: vi.fn(() => of({
        status: 'success', data: {
          count: 0, resultados: [], nodo_principal: '50001',
          tablas: ['50001', '50003', '1998'].map(node => ({
            nodo_local: node, descripcion: `Server ${node}`, servidor_conectado: false, nodos_disponibles: []
          }))
        }
      }))
    };
    TestBed.configureTestingModule({
      imports: [NodeReconnectionsComponent],
      providers: [
        provideRouter([]),
        { provide: NodeReconnectionsService, useValue: service },
        { provide: AllstarlinkService, useValue: allstarlink },
        { provide: TranslationService, useValue: {
          currentLang: signal('es'), revision: signal(0),
          translate: (key: string, params?: Record<string, string>) => key === 'RECONNECT_SOURCE_SAVE_ERROR'
            ? `${key}: ${params?.['source']} <- ${params?.['destination']}` : key
        } }
      ]
    });
    const fixture = TestBed.createComponent(NodeReconnectionsComponent);
    fixture.detectChanges();
    return { fixture, component: fixture.componentInstance, service, allstarlink };
  }

  it('creates an EchoLink destination with a personal interval and mode', () => {
    const { fixture, component, service } = setup();
    component.open();
    expect(component.form.getRawValue()).toEqual({
      source_node: '50001', node: '', enabled: false, interval_seconds: 60, action: 'connect', permanent: false
    });
    component.form.setValue({ source_node: '50003', node: '3123457', enabled: true, interval_seconds: 30, action: 'localmonitor', permanent: true });
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(false);
    save.click();
    expect(service.guardar).toHaveBeenCalledWith({
      source_node: '50003', node: '3123457', enabled: true, interval_seconds: 30, action: 'localmonitor', permanent: true
    }, null);
    expect(component.editorOpen()).toBe(false);
    expect(service.listar).toHaveBeenCalledTimes(2);
  });

  it('validates destination and frequency, disables unchanged saves and retains failed edits', () => {
    const { fixture, component, service } = setup();
    component.open(entry);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    for (const interval of [0, 1.5, 86401]) {
      component.form.controls.interval_seconds.setValue(interval);
      component.save();
      expect(service.guardar).not.toHaveBeenCalled();
    }
    component.form.controls.interval_seconds.setValue(120);
    component.form.controls.node.setValue('123;reload');
    component.save();
    expect(service.guardar).not.toHaveBeenCalled();
    component.form.controls.node.setValue('3123456');
    service.guardar.mockReturnValue(throwError(() => ({ error: { message: 'RECONNECT_DB_ERROR' } })));
    component.save();
    expect(component.error()).toBe('RECONNECT_DB_ERROR');
    expect(component.editorOpen()).toBe(true);
    expect(component.form.controls.interval_seconds.value).toBe(120);
  });

  it('toggles activation without modifying settings and confirms deletion', () => {
    const { fixture, component, service } = setup();
    const identity: HTMLElement = fixture.nativeElement.querySelector('.identity');
    expect(Array.from(identity.querySelectorAll('.endpoint strong')).map(node => node.childNodes[0].textContent?.trim()))
      .toEqual(['50001', '3123456']);
    expect(identity.querySelector('.route-arrow')?.classList.contains('fa-arrow-left')).toBe(true);
    const buttons: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('.actions button'));
    expect(buttons.map(button => button.getAttribute('aria-label'))).toEqual(['RECONNECT_DISABLE', 'EDITAR', 'ELIMINAR']);
    expect(buttons[0].getAttribute('aria-pressed')).toBe('true');
    buttons[0].click();
    expect(service.guardar).toHaveBeenCalledWith({
      source_node: '50001', node: entry.node, enabled: false, interval_seconds: 60, action: 'monitor', permanent: true
    }, 1);
    buttons[2].click();
    expect(service.eliminar).not.toHaveBeenCalled();
    expect(component.pendingDelete()?.id).toBe(1);
    component.delete();
    expect(service.eliminar).toHaveBeenCalledWith(1);
    expect(component.pendingDelete()).toBeNull();
  });

  it('does not show loading labels or change the card content while reading the list', () => {
    const { fixture, component } = setup();
    component.entries.set([entry, { ...entry, id: 2, enabled: false }]);
    fixture.detectChanges();
    const list: HTMLElement = fixture.nativeElement.querySelector('.list');
    const content = list.textContent;
    component.loading.set(true);
    fixture.detectChanges();
    expect(list.textContent).toBe(content);
    expect(list.querySelector('.refresh-status')).toBeNull();
    expect(fixture.nativeElement.textContent).not.toContain('CARGANDO');
    component.loading.set(false);
    fixture.detectChanges();
    expect(list.textContent).toBe(content);
  });

  it('preserves list state after activation and deletion failures', () => {
    const { component, service } = setup();
    service.guardar.mockReturnValue(throwError(() => ({ error: { message: 'RECONNECT_PRIMARY_ERROR' } })));
    component.toggle(entry);
    expect(component.entries()[0].enabled).toBe(true);
    expect(component.error()).toBe('RECONNECT_PRIMARY_ERROR');
    component.pendingDelete.set(entry);
    service.eliminar.mockReturnValue(throwError(() => ({ error: { message: 'RECONNECT_DB_ERROR' } })));
    component.delete();
    expect(component.entries().length).toBe(1);
    expect(component.pendingDelete()?.id).toBe(1);
    expect(component.busy()).toBe(false);
  });

  it('shows a stopped timer instead of pending or historical checks when disabled', () => {
    const { fixture, component } = setup();
    component.entries.set([{
      ...entry, enabled: false, last_result: 'RECONNECT_REQUESTED', last_details: 'Previous attempt'
    }]);
    fixture.detectChanges();
    const status: HTMLElement = fixture.nativeElement.querySelector('.status');
    expect(status.textContent).toContain('RECONNECT_STOPPED');
    expect(status.textContent).not.toContain('RECONNECT_REQUESTED');
    expect(status.textContent).not.toContain('RECONNECT_NEXT_CHECK');
    expect(status.querySelector('details')).toBeNull();
    expect(fixture.nativeElement.querySelector('.actions button').getAttribute('aria-pressed')).toBe('false');
  });

  it('does not automatically reload the list or overwrite open edits', () => {
    vi.useFakeTimers();
    try {
      const { fixture, component, service } = setup();
      vi.advanceTimersByTime(5000);
      expect(service.listar).toHaveBeenCalledTimes(1);
      component.open(entry);
      component.form.controls.interval_seconds.setValue(120);
      vi.advanceTimersByTime(10000);
      expect(service.listar).toHaveBeenCalledTimes(1);
      expect(component.form.controls.interval_seconds.value).toBe(120);
      component.close();
      vi.advanceTimersByTime(5000);
      expect(service.listar).toHaveBeenCalledTimes(1);
      fixture.destroy();
      vi.advanceTimersByTime(5000);
      expect(service.listar).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('uses server discovery for the source combo and preserves unavailable saved sources', () => {
    const { fixture, component, service, allstarlink } = setup();
    component.open();
    fixture.detectChanges();
    const select: HTMLSelectElement = fixture.nativeElement.querySelector('select[formControlName="source_node"]');
    expect(Array.from(select.options).map(option => option.value)).toEqual(['', '50001', '50003', '1998']);
    select.value = '1998';
    select.dispatchEvent(new Event('change'));
    expect(component.form.controls.source_node.value).toBe('1998');
    component.form.controls.node.setValue('1998');
    component.save();
    expect(service.guardar).not.toHaveBeenCalled();
    expect(component.error()).toBe('RECONNECT_SELF');
    component.close();
    component.open({ ...entry, source_node: '99999' });
    component.form.controls.interval_seconds.setValue(120);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    expect(component.form.controls.source_node.value).toBe('99999');
    component.save();
    expect(component.error()).toBe('RECONNECT_SOURCE_SAVE_ERROR: 99999 <- 3123456');
    expect(allstarlink.listarServidores).toHaveBeenCalled();
  });

  it('reports discovery errors and keeps legacy entries on the current primary', () => {
    const { component, service, allstarlink } = setup();
    component.open({ ...entry, source_node: '' });
    component.form.controls.interval_seconds.setValue(120);
    component.save();
    expect(service.guardar).toHaveBeenCalledWith(expect.objectContaining({ source_node: '' }), 1);
    allstarlink.listarServidores.mockReturnValue(throwError(() => ({ error: { message: 'RECONNECT_SOURCE_ERROR' } })));
    component.load();
    expect(component.sources()).toEqual([]);
    expect(component.error()).toBe('RECONNECT_LIST_LOAD_ERROR RECONNECT_SOURCE_ERROR');
  });

  it('accepts a destination outside the server list and identifies only the failing source in save errors', () => {
    const { component, service } = setup();
    component.open();
    component.form.controls.node.setValue('1995');
    component.save();
    expect(service.guardar).toHaveBeenCalledWith(expect.objectContaining({
      source_node: '50001', node: '1995'
    }), null);
    service.guardar.mockReturnValue(throwError(() => ({ error: {
      message: 'RECONNECT_SOURCE_SAVE_ERROR', source_node: '50001', destination_node: '1995'
    } })));
    component.toggle({ ...entry, node: '1995' });
    expect(component.error()).toBe('RECONNECT_SOURCE_SAVE_ERROR: 50001 <- 1995');
  });
});
