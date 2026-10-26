import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { calcularRedIPv4, PrimaryNodeComponent } from './primary-node.component';
import { environment } from '../../../core/environment';
import { NodeAccessDocument, isNodeAccessDocument } from '../../../core/services/primary-node.service';
import { TranslationService } from '../../../core/services/translation.service';
import es from '../../../../../public/assets/i18n/es.json';
import en from '../../../../../public/assets/i18n/en.json';

const url = `${environment.apiUrl}/primary-node`;
const translations: Record<string, Record<string, string>> = { es, en };
const config = { node: '616265', callsign: 'TEST1', password_set: true, password: 'test_password', path: '/test/rpt.conf' };

describe('Primary node administration', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [PrimaryNodeComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])]
    });
  });

  async function crear(amiResponse: object = {
    user: 'amerimon', secret: 'ami_secret', secret_set: true, bind_address: '127.0.0.1', permit: [], deny: []
  }) {
    const fixture = TestBed.createComponent(PrimaryNodeComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    const languageLoaded = TestBed.inject(TranslationService).loadLanguage('es');
    http.expectOne('./assets/i18n/es.json').flush(translations['es']);
    http.expectOne(url).flush(config);
    http.expectOne(`${url}/access`).flush({ node: config.node, entries: [] });
    http.expectOne(`${url}/ami`).flush(amiResponse);
    await languageLoaded;
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, http, component: fixture.componentInstance, element: fixture.nativeElement as HTMLElement };
  }

  it('translates the full template and substitutes node values when the language changes', async () => {
    const { fixture, http, component, element } = await crear();
    expect(element.textContent).toContain('Lista de acceso del nodo 616265');
    component.confirmando.set(true);
    component.numero = '616267';
    const loading = TestBed.inject(TranslationService).loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush(translations['en']);
    await loading;
    fixture.detectChanges();
    expect(element.textContent).toContain('Access list for node 616265');
    expect(element.textContent).toContain('The configuration for node 616265 will be saved as 616267.');
    expect(element.textContent).toContain('The backend needs local AMI access.');
    expect(element.querySelector('button[title="Open advanced network calculator"]')).not.toBeNull();
    expect(element.textContent).not.toContain('{{');
    http.verify();
    fixture.destroy();
  });

  it('calculates IPv4 networks from prefixes or dotted masks', () => {
    expect(calcularRedIPv4('192.168.1.37/24')).toBe('192.168.1.0/24');
    expect(calcularRedIPv4('10.2.3.4/255.255.0.0')).toBe('10.2.0.0/16');
    expect(calcularRedIPv4('10.2.3.4')).toBe('10.2.3.4/32');
    expect(calcularRedIPv4('192.168.1.37/255.0.255.0')).toBeNull();
    expect(calcularRedIPv4('not-an-ip')).toBeNull();
  });

  it('saves the AMI configuration and refreshes the verified values', async () => {
    const { fixture, http, component } = await crear();
    component.amiSecret.set('new_secret');
    component.amiBindAddress.set('0.0.0.0');
    component.permitsNuevas.set('192.168.1.37/24');
    component.deniesNuevas.set('10.0.0.0/8');
    component.guardarAMI();
    const request = http.expectOne(`${url}/ami`);
    expect(request.request.method).toBe('PUT');
    expect(request.request.body).toEqual({
      user: 'amerimon',
      secret: 'new_secret',
      secret_set: true,
      bind_address: '0.0.0.0',
      permit: ['192.168.1.37/24'],
      deny: ['10.0.0.0/8'],
    });
    request.flush({ status: 'ok', message: 'manager reload verificado' });
    http.expectOne(`${url}/ami`).flush({
      user: 'amerimon', secret: 'new_secret', secret_set: true,
      bind_address: '0.0.0.0', permit: ['192.168.1.0/24', '127.0.0.1/32'], deny: ['10.0.0.0/8']
    });
    fixture.detectChanges();
    expect(component.avisoAMI()).toContain('manager reload');
    expect(component.amiConfig()?.permit).toContain('192.168.1.0/24');
    expect(component.permitsNuevas()).toBe('');
    expect(component.deniesNuevas()).toBe('');
    http.verify();
    fixture.destroy();
  });

  it('shows and hides the loaded AMI secret with its eye button', async () => {
    const { fixture, http, element } = await crear();
    const amiSecretField = element.querySelector<HTMLElement>('.ami-config app-password-field');
    expect(amiSecretField?.querySelector<HTMLInputElement>('input[type="password"]')?.value).toBe('ami_secret');

    amiSecretField?.querySelector<HTMLButtonElement>('button[aria-label="Mostrar contraseña"]')?.click();
    fixture.detectChanges();
    expect(amiSecretField?.querySelector<HTMLInputElement>('input[type="text"]')?.value).toBe('ami_secret');

    amiSecretField?.querySelector<HTMLButtonElement>('button[aria-label="Ocultar contraseña"]')?.click();
    fixture.detectChanges();
    expect(amiSecretField?.querySelector<HTMLInputElement>('input[type="password"]')?.value).toBe('ami_secret');

    const loading = TestBed.inject(TranslationService).loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush(translations['en']);
    await loading;
    fixture.detectChanges();
    expect(amiSecretField?.querySelector('button[aria-label="Show password"]')).not.toBeNull();
    amiSecretField?.querySelector<HTMLButtonElement>('button[aria-label="Show password"]')?.click();
    fixture.detectChanges();
    expect(amiSecretField?.querySelector('button[aria-label="Hide password"]')).not.toBeNull();
    http.verify();
    fixture.destroy();
  });

  it('keeps the AMI form working when the API returns null ACL lists', async () => {
    const { fixture, http, element } = await crear({
      user: 'amerimon', secret: 'ami_secret', secret_set: true, bind_address: '127.0.0.1',
      permit: null, deny: null
    });
    expect(element.querySelector('.ami-config app-password-field button[aria-label="Mostrar contraseña"]')).not.toBeNull();
    expect(element.textContent).toContain('No hay reglas permit/deny configuradas');
    expect(element.querySelector<HTMLInputElement>('.ami-config app-password-field input[type="password"]')?.value)
      .toBe('ami_secret');
    http.verify();
    fixture.destroy();
  });

  it('imports confirmed lists into the loaded primary and refreshes the merged data', async () => {
    const { fixture, http, component } = await crear();
    const documento: NodeAccessDocument = {
      format: 'amerimon-node-access', version: 1, node: '616266',
      entries: [{ type: 'allowlist', node: '448511', reason: 'Imported' }]
    };
    component.numero = '616267';
    component.importacionPendiente.set({ documento, nodo: '616265' });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('[aria-label="Confirmar importación de listas"]')?.textContent)
      .toContain('616266');
    http.expectNone(`${url}/access/import`);
    component.importarAcceso();
    const request = http.expectOne(`${url}/access/import`);
    expect(request.request.body).toEqual({ primary_node: '616265', document: documento });
    request.flush({ imported: 1 });
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: documento.entries });
    expect(component.avisoAcceso()).toContain('1 entradas importadas');
    expect(component.importacionPendiente()).toBeNull();
    http.verify();
    fixture.destroy();
  });

  it('rejects invalid JSON document shapes and unsupported versions', () => {
    expect(isNodeAccessDocument(null)).toBe(false);
    expect(isNodeAccessDocument({ format: 'amerimon-node-access', version: 2, node: '616265', entries: [] })).toBe(false);
    expect(isNodeAccessDocument({
      format: 'amerimon-node-access', version: 1, node: '616265', entries: [{ type: 'other', node: '448511', reason: '' }]
    })).toBe(false);
    expect(isNodeAccessDocument({ format: 'amerimon-node-access', version: 1, node: '616265', entries: [] })).toBe(true);
  });

  it('reads a selected JSON file and requests confirmation without changing AstDB', async () => {
    const { fixture, http, component, element } = await crear();
    const documento: NodeAccessDocument = {
      format: 'amerimon-node-access', version: 1, node: '616266',
      entries: [{ type: 'denylist', node: 'TEST2', reason: 'Prueba' }]
    };
    const input = element.querySelector<HTMLInputElement>('input[type="file"]')!;
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [{ size: 100, text: async () => JSON.stringify(documento) }]
    });
    input.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(component.importacionPendiente()).toEqual({ documento, nodo: '616265' });
    expect(component.leyendoArchivo()).toBe(false);
    http.expectNone(`${url}/access/import`);
    http.verify();
    fixture.destroy();
  });

  it('reports malformed and oversized input files without submitting an import', async () => {
    const { fixture, http, component, element } = await crear();
    const input = element.querySelector<HTMLInputElement>('input[type="file"]')!;
    Object.defineProperty(input, 'files', {
      configurable: true, value: [{ size: 10, text: async () => '{invalid json' }]
    });
    input.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    expect(component.errorAcceso()).toContain('JSON válido');
    expect(component.importacionPendiente()).toBeNull();
    Object.defineProperty(input, 'files', {
      configurable: true, value: [{ size: 1024 * 1024 + 1 }]
    });
    input.dispatchEvent(new Event('change'));
    expect(component.errorAcceso()).toContain('1 MiB');
    http.expectNone(`${url}/access/import`);
    http.verify();
    fixture.destroy();
  });

  it('reports import failures without announcing success', async () => {
    const { fixture, http, component } = await crear();
    component.importacionPendiente.set({
      nodo: '616265',
      documento: { format: 'amerimon-node-access', version: 1, node: '616265', entries: [] }
    });
    component.importarAcceso();
    http.expectOne(`${url}/access/import`).flush(
      { message: 'No se pudo importar; se restauraron las entradas anteriores' }, { status: 500, statusText: 'Error' }
    );
    expect(component.errorAcceso()).toContain('se restauraron');
    expect(component.avisoAcceso()).toBe('');
    expect(component.guardandoAcceso()).toBe(false);
    http.verify();
    fixture.destroy();
  });

  it('loads the current password hidden and lets the administrator reveal and hide it', async () => {
    const { fixture, http, component, element } = await crear();
    expect(component.actual()).toBe('616265');
    expect(element.querySelector<HTMLInputElement>('input[name="numero"]')?.value).toBe('616265');
    expect(element.querySelector<HTMLInputElement>('input[name="indicativo"]')?.value).toBe('TEST1');
    expect(element.querySelector<HTMLInputElement>('input[type="password"]')?.value).toBe('test_password');
    element.querySelector<HTMLButtonElement>('button[aria-label="Mostrar contraseña"]')!.click();
    fixture.detectChanges();
    expect(element.querySelector<HTMLInputElement>('app-password-field input[type="text"]')?.value).toBe('test_password');
    element.querySelector<HTMLButtonElement>('button[aria-label="Ocultar contraseña"]')!.click();
    fixture.detectChanges();
    expect(element.querySelector<HTMLInputElement>('app-password-field input[type="password"]')?.value).toBe('test_password');
    expect(component.passwordConfigurada()).toBe(true);
    expect(element.textContent).toContain('No hay restricciones configuradas.');
    http.verify();
    fixture.destroy();
  });

  it('requires confirmation, preserves a blank password and never automatically restarts Asterisk', async () => {
    const { fixture, http, component, element } = await crear();
    component.numero = '616266';
    component.indicativo = 'NEW2';
    fixture.detectChanges();
    await fixture.whenStable();
    element.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    fixture.detectChanges();
    http.expectNone(url);
    expect(element.querySelector('[aria-label="Confirmar configuración"]')).not.toBeNull();
    const confirm = Array.from(element.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('Confirmar guardado'))!;
    confirm.click();
    const request = http.expectOne(url);
    expect(request.request.method).toBe('PUT');
    expect(request.request.body).toEqual({
      current_node: '616265', node: '616266', callsign: 'NEW2', password: ''
    });
    request.flush({ node: '616266', restart_required: true, message: 'Guardado. Reinicia desde Sistema.' });
    http.expectOne(url).flush({ ...config, node: '616266', callsign: 'NEW2' });
    http.expectOne(`${url}/access`).flush({ node: '616266', entries: [] });
    expect(component.actual()).toBe('616266');
    expect(component.aviso()).toContain('Reinicia');
    http.expectNone(`${environment.apiUrl}/system/control`);
    http.verify();
    fixture.destroy();
  });

  it('can edit identity when access-list queries fail, and offers a separate retry', async () => {
    const fixture = TestBed.createComponent(PrimaryNodeComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    const http = TestBed.inject(HttpTestingController);
    http.expectOne('./assets/i18n/es.json').flush(translations['es']);
    http.expectOne(url).flush(config);
    http.expectOne(`${url}/access`).flush({ message: 'AMI no disponible' }, { status: 503, statusText: 'Unavailable' });
    http.expectOne(`${url}/ami`).flush({
      user: 'amerimon', secret: 'ami_secret', secret_set: true, bind_address: '127.0.0.1', permit: [], deny: []
    });
    expect(component.actual()).toBe('616265');
    expect(component.error()).toBe('');
    expect(component.errorAcceso()).toBe('AMI no disponible');
    component.cargarAcceso();
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: [] });
    expect(component.errorAcceso()).toBe('');
    http.verify();
    fixture.destroy();
  });

  it('saves and deletes an access entry using the loaded primary node, not an unsaved new number', async () => {
    const { fixture, http, component } = await crear();
    component.numero = '616266';
    const entry = { type: 'denylist' as const, node: 'TEST2', reason: 'Prueba' };
    component.cambiarAcceso(entry);
    const put = http.expectOne(`${url}/access`);
    expect(put.request.method).toBe('PUT');
    expect(put.request.body).toEqual({ primary_node: '616265', ...entry });
    put.flush({ status: 'ok' });
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: [entry] });
    expect(component.entradas()).toEqual([entry]);
    component.cambiarAcceso(entry, true);
    const remove = http.expectOne(`${url}/access`);
    expect(remove.request.method).toBe('DELETE');
    expect(remove.request.body).toEqual({ primary_node: '616265', ...entry });
    remove.flush({ status: 'ok' });
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: [] });
    expect(component.entradas()).toEqual([]);
    http.verify();
    fixture.destroy();
  });

  it('surfaces save failures and removes the newly typed password from the form', async () => {
    const { fixture, http, component } = await crear();
    component.password = 'new_password';
    component.guardar();
    http.expectOne(url).flush({ message: 'No se pudo crear el respaldo' }, { status: 500, statusText: 'Error' });
    expect(component.error()).toBe('No se pudo crear el respaldo');
    expect(component.aviso()).toBe('');
    expect(component.password).toBe('');
    expect(component.guardando()).toBe(false);
    http.verify();
    fixture.destroy();
  });

  it('renders access entries in a table with deletion in the rightmost column', async () => {
    const { fixture, http, component, element } = await crear();
    const entries = [
      { type: 'allowlist' as const, node: '448511', reason: 'Permitido de prueba' },
      { type: 'denylist' as const, node: 'TEST2/P', reason: '' }
    ];
    component.entradas.set(entries);
    fixture.detectChanges();
    const table = element.querySelector<HTMLTableElement>('table[aria-label="Lista de acceso del nodo principal"]')!;
    expect(Array.from(table.tHead!.rows[0].cells, cell => cell.textContent?.trim()))
      .toEqual(['Lista', 'Nodo / indicativo', 'Motivo', 'Acción']);
    const rows = Array.from(table.tBodies[0].rows);
    expect(rows).toHaveLength(2);
    expect(rows[0].cells[1].textContent).toBe('448511');
    expect(rows[1].cells[1].textContent).toBe('TEST2/P');
    expect(rows[0].cells[2].textContent).toBe('Permitido de prueba');
    const remove = rows[1].cells[3].querySelector<HTMLButtonElement>('button')!;
    expect(remove.querySelector('i.fa-trash-can')).not.toBeNull();
    expect(remove.getAttribute('aria-label')).toBe('Eliminar TEST2/P de bloqueados');
    remove.click();
    const request = http.expectOne(`${url}/access`);
    expect(request.request.method).toBe('DELETE');
    expect(request.request.body).toEqual({ primary_node: '616265', ...entries[1] });
    fixture.detectChanges();
    expect(rows.every(row => row.cells[3].querySelector<HTMLButtonElement>('button')!.disabled)).toBe(true);
    request.flush({ status: 'ok' });
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: [entries[0]] });
    fixture.detectChanges();
    expect(table.tBodies[0].rows).toHaveLength(1);
    http.verify();
    fixture.destroy();
  });

  it('does not present an invalid or private primary number as saveable', async () => {
    const { fixture, http, element } = await crear();
    const numero = element.querySelector<HTMLInputElement>('input[name="numero"]')!;
    numero.value = '1998';
    numero.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const save = Array.from(element.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.trim() === 'Guardar configuración')!;
    expect(save.disabled).toBe(true);
    http.verify();
    fixture.destroy();
  });

  it('uses patterns compatible with browser Unicode sets and portable callsigns', async () => {
    const { fixture, http, element } = await crear();
    for (const input of element.querySelectorAll<HTMLInputElement>('input[pattern]')) {
      const pattern = new RegExp(`^(?:${input.getAttribute('pattern')})$`, 'v');
      if (input.name === 'indicativo' || input.name === 'remoto') {
        expect(pattern.test('EA4ABC/P')).toBe(true);
        expect(pattern.test('TEST-1')).toBe(true);
        expect(pattern.test('BAD CALL')).toBe(false);
      }
    }
    http.verify();
    fixture.destroy();
  });

  it('confirms clearing one list, preserves the other and supports cancellation', async () => {
    const { fixture, http, component, element } = await crear();
    const allowed = { type: 'allowlist' as const, node: '448511', reason: '' };
    const denied = { type: 'denylist' as const, node: 'TEST2', reason: '' };
    component.entradas.set([allowed, denied]);
    fixture.detectChanges();
    const button = (text: string) => Array.from(element.querySelectorAll<HTMLButtonElement>('button'))
      .find(item => item.textContent?.trim() === text)!;
    button('Vaciar permitidos').click();
    fixture.detectChanges();
    expect(element.querySelector('[aria-label="Confirmar vaciado de lista"]')?.textContent).toContain('616265');
    http.expectNone(`${url}/access/clear`);
    button('Cancelar').click();
    fixture.detectChanges();
    expect(component.vaciadoPendiente()).toBeNull();
    button('Vaciar denegados').click();
    fixture.detectChanges();
    button('Confirmar vaciado').click();
    const request = http.expectOne(`${url}/access/clear`);
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      primary_node: '616265', type: 'denylist', confirmation: 'VACIAR denylist'
    });
    fixture.detectChanges();
    expect(button('Vaciar permitidos').disabled).toBe(true);
    request.flush({ status: 'ok' });
    http.expectOne(`${url}/access`).flush({ node: '616265', entries: [allowed] });
    expect(component.entradas()).toEqual([allowed]);
    expect(component.avisoAcceso()).toContain('denegados');
    http.verify();
    fixture.destroy();
  });

  it('shows clear failures without claiming success or discarding displayed entries', async () => {
    const { fixture, http, component } = await crear();
    const entry = { type: 'allowlist' as const, node: '448511', reason: '' };
    component.entradas.set([entry]);
    component.solicitarVaciado('allowlist');
    component.vaciarAcceso();
    http.expectOne(`${url}/access/clear`).flush(
      { message: 'Vaciado parcial. Recarga la lista.' }, { status: 500, statusText: 'Error' }
    );
    expect(component.errorAcceso()).toBe('Vaciado parcial. Recarga la lista.');
    expect(component.avisoAcceso()).toBe('');
    expect(component.entradas()).toEqual([entry]);
    expect(component.guardandoAcceso()).toBe(false);
    http.verify();
    fixture.destroy();
  });
});
