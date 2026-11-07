import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { PrivateNodesService } from '../../../core/services/private-nodes.service';
import { TranslationService } from '../../../core/services/translation.service';
import { PrivateNodesComponent } from './private-nodes.component';

describe('Private nodes save changes', () => {
  it('stages individual server visibility in the list and keeps edits after save errors', async () => {
    const node = { id: '1998', rxchannel: 'Local/pseudo', settings: {}, show_as_server: false };
    const service = {
      obtener: vi.fn(() => of({ path: '/test/nodes', writable: true, lines: [] })),
      obtenerRPT: vi.fn(() => of({ status: 'ok', path: '/test/rpt.conf', nodes: [node], template_settings: {}, template_descriptions: {} })),
      guardarRPT: vi.fn(() => throwError(() => ({ error: { message: 'Save failed' } }))),
    };
    TestBed.configureTestingModule({
      imports: [PrivateNodesComponent],
      providers: [
        provideRouter([]),
        { provide: PrivateNodesService, useValue: service },
        { provide: TranslationService, useValue: { currentLang: signal('es'), revision: signal(0), translate: (key: string) => key } },
      ],
    });
    const fixture = TestBed.createComponent(PrivateNodesComponent);
    fixture.detectChanges();
    await fixture.whenStable();
    const component = fixture.componentInstance;
    const actions: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('.private-rpt-acciones button'));
    expect(actions.map(button => button.getAttribute('aria-label'))).toEqual(['GUARDAR_CAMBIOS', 'EDITAR', 'ELIMINAR']);
    for (const button of actions) {
      expect(button.title).toBe(button.getAttribute('aria-label'));
      expect(button.textContent?.trim()).toBe('');
      expect(button.querySelector('i')?.getAttribute('aria-hidden')).toBe('true');
    }
    expect(actions[0].disabled).toBe(true);
    const checkbox: HTMLInputElement = fixture.nativeElement.querySelector('.private-rpt-lista input[type="checkbox"]');
    expect(checkbox.checked).toBe(false);
    expect(checkbox.getAttribute('aria-describedby')).toBe('rpt-server-help-1998');
    expect(fixture.nativeElement.querySelector('.private-rpt-identidad strong').textContent).toBe('1998');
    component.guardarServidores();
    expect(service.guardarRPT).not.toHaveBeenCalled();
    checkbox.checked = true;
    checkbox.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(component.cambiosServidores.changed(component.rptNodes())).toBe(true);
    expect(actions[0].disabled).toBe(false);
    component.cambiarServidor('1998', false);
    expect(component.cambiosServidores.changed(component.rptNodes())).toBe(false);
    component.cambiarServidor('1998', true);
    actions[0].click();
    expect(service.guardarRPT).toHaveBeenCalledWith([{ ...node, show_as_server: true }]);
    expect(component.rptNodes()[0].show_as_server).toBe(true);
    expect(component.errorRPT()).toBe('Save failed');
    actions[1].click();
    fixture.detectChanges();
    expect(component.editorAbierto()).toBe(true);
    expect(fixture.nativeElement.querySelector('.private-rpt-editor input[type="checkbox"]')).toBeNull();
    component.cerrarEditor();
    fixture.detectChanges();
    actions[2].click();
    expect(component.pendienteEliminar()).toBe('1998');
  });

  it('tracks edited, added and removed lines, retaining edits after errors and resetting on reload', () => {
    const service = {
      obtener: vi.fn(() => of({ path: '/test/nodes', writable: true, lines: ['1234 = radio@example'] })),
      guardar: vi.fn(() => throwError(() => ({ error: { message: 'Save failed' } }))),
      obtenerRPT: vi.fn(() => of({ status: 'ok', path: '/test/rpt.conf', nodes: [], template_settings: {}, template_descriptions: {} })),
      guardarRPT: vi.fn(() => of({ status: 'ok', path: '/test/rpt.conf' })),
    };
    TestBed.configureTestingModule({
      imports: [PrivateNodesComponent],
      providers: [
        provideRouter([]),
        { provide: PrivateNodesService, useValue: service },
        { provide: TranslationService, useValue: { currentLang: signal('es'), revision: signal(0), translate: (key: string) => key } },
      ],
    });
    const fixture = TestBed.createComponent(PrivateNodesComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(true);
    component.guardar();
    expect(service.guardar).not.toHaveBeenCalled();
    component.agregar();
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    component.quitar(1);
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    component.actualizar(0, '1234 = radio@changed');
    component.guardar();
    fixture.detectChanges();
    expect(component.error()).toBe('Save failed');
    expect(save.disabled).toBe(false);
    component.cargar();
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
  });

  it('creates a four-digit rpt.conf node with the configured rxchannel and defaults', () => {
    const service = {
      obtener: vi.fn(() => of({ path: '/test/nodes', writable: true, lines: [] })),
      guardar: vi.fn(() => of({ status: 'ok', path: '/test/nodes' })),
      obtenerRPT: vi.fn(() => of({ status: 'ok', path: '/test/rpt.conf', nodes: [], template_settings: {}, template_descriptions: {} })),
      guardarRPT: vi.fn(() => of({ status: 'ok', path: '/test/rpt.conf' })),
    };
    TestBed.configureTestingModule({
      imports: [PrivateNodesComponent],
      providers: [
        provideRouter([]),
        { provide: PrivateNodesService, useValue: service },
        { provide: TranslationService, useValue: { currentLang: signal('es'), revision: signal(0), translate: (key: string) => key } },
      ],
    });
    const fixture = TestBed.createComponent(PrivateNodesComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    expect(fixture.nativeElement.querySelectorAll('section.private-nodes').length).toBe(2);
    component.abrirNuevoNodo();
    expect(component.nodoActual()).toEqual({
      id: '',
      show_as_server: false,
      rxchannel: 'USRP/127.0.0.1:34001:32001',
      settings: {
        duplex: '0',
        hangtime: '0',
        althangtime: '0',
        holdofftelem: '1',
        telemdefault: '0',
        telemdynamic: '0',
        linktolink: 'no',
        nounkeyct: '1',
        totime: '180000',
      },
    });
    component.actualizarNodo({ id: '1998', rxchannel: 'USRP/127.0.0.1:34001:32001' });
    component.guardarNodo();

    expect(service.guardarRPT).toHaveBeenCalledWith([{
      id: '1998',
      show_as_server: false,
      rxchannel: 'USRP/127.0.0.1:34001:32001',
      settings: {
        duplex: '0',
        hangtime: '0',
        althangtime: '0',
        holdofftelem: '1',
        telemdefault: '0',
        telemdynamic: '0',
        linktolink: 'no',
        nounkeyct: '1',
        totime: '180000',
      },
    }]);
  });

  it('offers template parameters with documentation, allows custom overrides and retains editor help', async () => {
    const currentLang = signal<'es' | 'en'>('es');
    const service = {
      obtener: vi.fn(() => of({ path: '/test/nodes', writable: true, lines: [] })),
      obtenerRPT: vi.fn(() => of({
        status: 'ok', path: '/test/rpt.conf', nodes: [],
        template_settings: { duplex: '2', rxchannel: 'Local/pseudo', context: 'radio', custom: '1' },
        template_descriptions: { context: 'Dialplan context.\nUsed for incoming calls.' },
      })),
    };
    const translations: Record<string, Record<string, string>> = {
      es: { PARAMETRO_RPT: 'Parámetro', PARAMETRO_RPT_DOCUMENTACION: 'Descripción de la plantilla' },
      en: { PARAMETRO_RPT: 'Parameter', PARAMETRO_RPT_DOCUMENTACION: 'Template description' },
    };
    TestBed.configureTestingModule({
      imports: [PrivateNodesComponent],
      providers: [
        provideRouter([]),
        { provide: PrivateNodesService, useValue: service },
        { provide: TranslationService, useValue: {
          currentLang, revision: signal(0),
          translate: (key: string) => translations[currentLang()][key] ?? key,
        } },
      ],
    });
    const fixture = TestBed.createComponent(PrivateNodesComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.abrirNuevoNodo();
    fixture.detectChanges();
    await fixture.whenStable();
    const editor: HTMLElement = fixture.nativeElement.querySelector('.private-rpt-editor');
    expect(editor.querySelectorAll('.rxchannel-ayuda li').length).toBe(8);
    expect(component.parametrosDisponibles().map(parameter => parameter.key)).toEqual(['context', 'custom']);
    const select: HTMLSelectElement = editor.querySelector('select')!;
    select.value = Array.from(select.options).find(option => option.textContent === 'context = radio')!.value;
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(component.parametroValue()).toBe('radio');
    expect(editor.querySelector('.private-rpt-descripcion')?.textContent).toBe('Dialplan context.\nUsed for incoming calls.');
    expect(editor.textContent).toContain('Descripción de la plantilla');
    currentLang.set('en');
    fixture.detectChanges();
    expect(editor.textContent).toContain('Template description');
    expect(editor.querySelector('.private-rpt-descripcion')?.textContent).toBe('Dialplan context.\nUsed for incoming calls.');

    component.parametroValue.set('private-radio');
    component.agregarParametro();
    expect(component.nodoActual().settings['context']).toBe('private-radio');
    expect(component.rptTemplateSettings()['context']).toBe('radio');
    expect(component.parametrosDisponibles().map(parameter => parameter.key)).toEqual(['custom']);
    component.seleccionarParametro('custom');
    fixture.detectChanges();
    expect(editor.querySelector('.private-rpt-descripcion')?.textContent).toBe('PARAMETRO_RPT_SIN_DOCUMENTACION');
    component.seleccionarParametro('__custom__');
    component.parametroKey.set('new_setting');
    component.parametroValue.set('42');
    component.agregarParametro();
    expect(component.nodoActual().settings['new_setting']).toBe('42');
    component.seleccionarParametro('__custom__');
    component.parametroKey.set('rxchannel');
    component.parametroValue.set('Local/pseudo');
    component.agregarParametro();
    expect(component.nodoActual().settings['rxchannel']).toBeUndefined();
    expect(component.errorRPT()).toBe('PARAMETRO_RPT_INVALIDO_O_DUPLICADO');
  });
});
