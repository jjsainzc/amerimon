import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { vi } from 'vitest';
import { environment } from '../../../core/environment';
import { TranslationService } from '../../../core/services/translation.service';
import { IaxUsersComponent } from './iax-users.component';
import es from '../../../../../public/assets/i18n/es.json';
import en from '../../../../../public/assets/i18n/en.json';

describe('IaxUsersComponent', () => {
  let fixture: ComponentFixture<IaxUsersComponent>;
  let http: HttpTestingController;
  const url = `${environment.apiUrl}/iax-users`;
  const user = {
    name: 'radio', type: 'friend', context: 'iax-client', auth: 'md5',
    secret: 'test-secret', host: 'dynamic', allow: ['ulaw'], disallow: ['all'], transfer: 'no',
  };

  beforeEach(async () => {
    TestBed.configureTestingModule({
      imports: [IaxUsersComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
      ],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(IaxUsersComponent);
    fixture.detectChanges();
    const loading = TestBed.inject(TranslationService).loadLanguage('es');
    http.expectOne('./assets/i18n/es.json').flush(es);
    await loading;
    fixture.detectChanges();
  });

  afterEach(() => http.verify());

  it('only enables saving changed values and disables it again after reverting', async () => {
    http.expectOne(url).flush({ HC1LNM: { ...user, name: 'HC1LNM' } });
    const component = fixture.componentInstance;
    component.seleccionarUsuario({ ...user, name: 'HC1LNM' });
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(true);
    component.guardarUsuario();
    http.expectNone(url);
    const input: HTMLInputElement = fixture.nativeElement.querySelector('input[name="context"]');
    input.value = 'changed';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    input.value = user.context;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    component.nuevoUsuario();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
  });

  it('loads once and renders the API map without requiring an open input', () => {
    http.expectOne(url).flush({ radio: user });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('radio');
    expect(fixture.nativeElement.querySelector('app-admin-volver')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('[aria-modal]')).toBeNull();
  });

  it('copies list identifiers and edited fields without navigation, saving or copying secrets', async () => {
    http.expectOne(url).flush({ radio: user });
    fixture.detectChanges();
    const writeText = vi.fn().mockResolvedValue(undefined);
    const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      fixture.nativeElement.querySelector('.sip-users-fila > .clipboard-copy').click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('radio');
      expect(fixture.componentInstance.editando()).toBe(false);
      fixture.componentInstance.seleccionarUsuario(user);
      fixture.detectChanges();
      await fixture.whenStable();
      const context: HTMLInputElement = fixture.nativeElement.querySelector('input[name="context"]');
      context.value = 'edited-context';
      context.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      context.parentElement!.querySelector<HTMLButtonElement>('.clipboard-copy')!.click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('edited-context');
      expect(fixture.nativeElement.querySelectorAll('form .clipboard-copy').length).toBe(3);
      expect(fixture.nativeElement.querySelector('app-password-field .clipboard-copy')).toBeNull();
      http.expectNone(url);
    } finally {
      if (original) { Object.defineProperty(navigator, 'clipboard', original); }
      else { Reflect.deleteProperty(navigator, 'clipboard'); }
    }
  });

  it('keeps the edited data visible after a backend error and supports retry', () => {
    http.expectOne(url).flush({ radio: user });
    fixture.componentInstance.seleccionarUsuario(user);
    fixture.componentInstance.usuarioActual().context = 'edited-context';
    fixture.componentInstance.guardarUsuario();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]').textContent).toContain('no parece un indicativo de radioaficionado');
    http.expectNone(url);
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    const request = http.expectOne(url);
    expect(request.request.method).toBe('POST');
    expect(request.request.body.secret).toBe('test-secret');
    request.flush({ message: 'No se pudo escribir iax.conf' }, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain('No se pudo escribir');
    expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    fixture.componentInstance.guardarUsuario();
    fixture.detectChanges();
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    http.expectOne(url).flush({ message: 'Guardado' });
    http.expectOne(url).flush({ radio: user });
    expect(fixture.componentInstance.editando()).toBe(false);
  });

  it('can recover from a loading error', () => {
    http.expectOne(url).flush({ message: 'Revisa permisos' }, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Revisa permisos');
    fixture.nativeElement.querySelector('.sip-users-toolbar button').click();
    http.expectOne(url).flush({});
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('No hay usuarios');
  });

  it('does not submit an empty form', async () => {
    http.expectOne(url).flush({});
    fixture.componentInstance.nuevoUsuario();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
    http.expectNone(url);
  });

  it('uses the shared password eye without submitting or modifying the secret', async () => {
    http.expectOne(url).flush({ radio: user });
    fixture.componentInstance.seleccionarUsuario(user);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const password: HTMLInputElement = fixture.nativeElement.querySelector('app-password-field input');
    const eye: HTMLButtonElement = fixture.nativeElement.querySelector('.pf-ojo');
    expect(password.id).toBe('iax-secret');
    expect(password.type).toBe('password');
    expect(password.value).toBe(user.secret);
    expect(eye.type).toBe('button');
    expect(eye.getAttribute('aria-label')).toBe(es.MOSTRAR_CONTRASENA);
    eye.click();
    fixture.detectChanges();
    expect(password.type).toBe('text');
    expect(eye.getAttribute('aria-label')).toBe(es.OCULTAR_CONTRASENA);
    expect(eye.getAttribute('aria-pressed')).toBe('true');
    eye.click();
    fixture.detectChanges();
    expect(password.type).toBe('password');
    expect(fixture.componentInstance.usuarioActual().secret).toBe(user.secret);
    expect(fixture.nativeElement.querySelector('.sip-users-eliminar')).toBeNull();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
    http.expectNone(url);
  });

  it('offers deletion for arbitrary usernames but protects iaxrpt and hidden sections', () => {
    http.expectOne(url).flush({
      radio: user,
      HC1LNM: { ...user, name: 'HC1LNM' },
    });
    fixture.componentInstance.seleccionarUsuario({ ...user, name: 'iaxrpt' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.sip-users-eliminar')).toBeNull();

    fixture.componentInstance.seleccionarUsuario({ ...user, name: 'HC1LNM' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.sip-users-eliminar')).not.toBeNull();
    fixture.componentInstance.seleccionarUsuario({ ...user, name: 'test-user' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.sip-users-eliminar')).not.toBeNull();
    for (const name of ['iaxrpt', 'IAXRPT', 'radio', 'allstar-sys', 'allstar-public', 'general']) {
      expect(fixture.componentInstance.puedeEliminarUsuario(name)).toBe(false);
    }
    http.expectNone(`${url}/HC1LNM`);
  });

  it('validates the shared secret field and sends its edited value', async () => {
    http.expectOne(url).flush({ radio: user });
    fixture.componentInstance.seleccionarUsuario(user);
    fixture.detectChanges();
    await fixture.whenStable();
    const password: HTMLInputElement = fixture.nativeElement.querySelector('app-password-field input');
    password.value = '';
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    password.value = 'edited-secret';
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(false);
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
    fixture.detectChanges();
    http.expectNone(url);
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    const request = http.expectOne(url);
    expect(request.request.body.secret).toBe('edited-secret');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('fieldset').disabled).toBe(true);
    request.flush({ message: 'Guardado' });
    http.expectOne(url).flush({ radio: user });
  });

  it('switches loading, list, empty state and form labels without changing configuration', async () => {
    expect(fixture.nativeElement.querySelector('[role="status"]').textContent).toBe(es.CARGANDO);
    const loading = TestBed.inject(TranslationService).loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush(en);
    await loading;
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="status"]').textContent).toBe(en.CARGANDO);
    expect(fixture.nativeElement.querySelector('h2').textContent).toBe(en.IAX_USERS_TITLE);
    http.expectOne(url).flush({ radio: user });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Context: iax-client');
    fixture.componentInstance.seleccionarUsuario(user);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const before = structuredClone(fixture.componentInstance.usuarioActual());
    for (const label of [en.IAX_USERS_NAME, en.SIP_USERS_CONTEXT, en.IAX_USERS_SECRET, en.IAX_USERS_AUTH, en.GUARDAR, en.CANCELAR]) {
      expect(fixture.nativeElement.querySelector('form').textContent).toContain(label);
    }
    await TestBed.inject(TranslationService).loadLanguage('es');
    fixture.detectChanges();
    for (const label of [es.IAX_USERS_NAME, es.SIP_USERS_CONTEXT, es.IAX_USERS_SECRET, es.IAX_USERS_AUTH, es.GUARDAR, es.CANCELAR]) {
      expect(fixture.nativeElement.querySelector('form').textContent).toContain(label);
    }
    expect(fixture.componentInstance.usuarioActual()).toEqual(before);
    fixture.componentInstance.editando.set(false);
    fixture.componentInstance.usuariosList.set([]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain(es.IAX_USERS_EMPTY);
    await TestBed.inject(TranslationService).loadLanguage('en');
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain(en.IAX_USERS_EMPTY);
    expect(fixture.nativeElement.textContent).toContain(en.IAX_USERS_NEW);
    expect(fixture.nativeElement.textContent).toContain(en.RECARGAR_LISTA);
    http.expectNone(url);
  });

  for (const operation of ['load', 'save']) {
    it(`translates ${operation} fallback errors when switching languages`, async () => {
      const component = fixture.componentInstance;
      let errorMessage: string;
      if (operation === 'load') {
        errorMessage = 'IAX_USERS_LOAD_ERROR';
      } else {
        http.expectOne(url).flush({ radio: user });
        component.seleccionarUsuario(user);
        component.usuarioActual().context = 'edited-context';
        errorMessage = 'IAX_USERS_SAVE_ERROR';
        component.guardarUsuario();
        fixture.detectChanges();
        (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
      }
      http.expectOne(url).flush({}, { status: 503, statusText: 'Unavailable' });
      fixture.detectChanges();
      const spanish: Record<string, string> = es;
      const english: Record<string, string> = en;
      expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toBe(spanish[errorMessage]);
      const loading = TestBed.inject(TranslationService).loadLanguage('en');
      http.expectOne('./assets/i18n/en.json').flush(en);
      await loading;
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toBe(english[errorMessage]);
    });
  }

  it('deletes a non-callsign user and refreshes the list', () => {
    const callsignUser = { ...user, name: 'test-user' };
    http.expectOne(url).flush({ HC1LNM: callsignUser });
    fixture.componentInstance.seleccionarUsuario(callsignUser);
    fixture.componentInstance.eliminarUsuario(callsignUser.name);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]').textContent).toContain('¿Eliminar el usuario IAX test-user?');
    http.expectNone(`${url}/test-user`);
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    const request = http.expectOne(`${url}/test-user`);
    expect(request.request.method).toBe('DELETE');
    request.flush({ message: 'Eliminado' });
    http.expectOne(url).flush({});
    expect(fixture.componentInstance.editando()).toBe(false);
    expect(fixture.componentInstance.usuariosList()).toEqual([]);
  });

  it('saves callsign sections directly and requires confirmation for system sections', () => {
    http.expectOne(url).flush({ 'HC1LNM': { ...user, name: 'HC1LNM' }, 'system-user': { ...user, name: 'system-user' } });
    fixture.componentInstance.seleccionarUsuario({ ...user, name: 'HC1LNM' });
    fixture.componentInstance.usuarioActual().context = 'edited-context';
    fixture.componentInstance.guardarUsuario();
    http.expectOne(url).flush({ message: 'Guardado' });
    http.expectOne(url).flush({});

    fixture.componentInstance.seleccionarUsuario({ ...user, name: 'system-user' });
    fixture.componentInstance.usuarioActual().context = 'edited-context';
    fixture.componentInstance.guardarUsuario();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]').textContent).toContain('system-user');
    http.expectNone(url);
    (fixture.nativeElement.querySelector('.confirmacion-cancelar') as HTMLButtonElement).click();
    http.expectNone(url);
    fixture.componentInstance.guardarUsuario();
    fixture.detectChanges();
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    http.expectOne(url).flush({ message: 'Guardado' });
    http.expectOne(url).flush({});
  });
});
