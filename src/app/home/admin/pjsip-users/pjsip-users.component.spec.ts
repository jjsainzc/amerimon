import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { vi } from 'vitest';
import { routes } from '../../../app.routes';
import { AuthService } from '../../../core/services/auth.service';
import { environment } from '../../../core/environment';
import { TranslationService } from '../../../core/services/translation.service';
import { PjsipUsersComponent } from './pjsip-users.component';
import es from '../../../../../public/assets/i18n/es.json';
import en from '../../../../../public/assets/i18n/en.json';

describe('PjsipUsersComponent', () => {
  let fixture: ComponentFixture<PjsipUsersComponent>;
  let http: HttpTestingController;
  const url = `${environment.apiUrl}/pjsip-users`;
  const extension = {
    id: '1001', context: 'sip-phones', password: 'test-secret',
    allow: ['ulaw'], disallow: ['all'], caller_id: 'Test',
    max_contacts: '4', direct_media: 'no', media_encryption: 'no',
  };

  beforeEach(async () => {
    TestBed.configureTestingModule({
      imports: [PjsipUsersComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
      ],
    });

    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(PjsipUsersComponent);
    fixture.detectChanges();
    const loading = TestBed.inject(TranslationService).loadLanguage('es');
    http.expectOne('./assets/i18n/es.json').flush(es);
    await loading;
    fixture.detectChanges();
  });

  afterEach(() => http.verify());

  it('blocks unchanged saves and detects reverted edits including the password', async () => {
    http.expectOne(url).flush([extension]);
    const component = fixture.componentInstance;
    component.seleccionarExtension(extension);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(true);
    component.guardarExtension();
    http.expectNone(url);
    const password: HTMLInputElement = fixture.nativeElement.querySelector('app-password-field input');
    password.value = 'new-password';
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    password.value = extension.password;
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
  });

  it('loads once and renders without modal inputs when activated', () => {
    http.expectOne(url).flush([extension]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('1001');
    expect(fixture.nativeElement.querySelector('app-admin-volver')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('[aria-modal]')).toBeNull();
  });

  it('copies list identifiers and edited fields without navigation, saving or copying passwords', async () => {
    http.expectOne(url).flush([extension]);
    fixture.detectChanges();
    const writeText = vi.fn().mockResolvedValue(undefined);
    const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      fixture.nativeElement.querySelector('.sip-users-fila > .clipboard-copy').click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('1001');
      expect(fixture.componentInstance.editando()).toBe(false);
      fixture.componentInstance.seleccionarExtension(extension);
      fixture.detectChanges();
      await fixture.whenStable();
      const caller: HTMLInputElement = fixture.nativeElement.querySelector('input[name="caller_id"]');
      caller.value = 'Edited caller';
      caller.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      caller.parentElement!.querySelector<HTMLButtonElement>('.clipboard-copy')!.click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('Edited caller');
      expect(fixture.nativeElement.querySelectorAll('form .clipboard-copy').length).toBe(4);
      expect(fixture.nativeElement.querySelector('app-password-field .clipboard-copy')).toBeNull();
      http.expectNone(url);
    } finally {
      if (original) { Object.defineProperty(navigator, 'clipboard', original); }
      else { Reflect.deleteProperty(navigator, 'clipboard'); }
    }
  });

  it('keeps the form and backend error visible, then allows a save retry', async () => {
    http.expectOne(url).flush([extension]);
    fixture.componentInstance.seleccionarExtension(extension);
    fixture.componentInstance.extActual().context = 'edited-context';
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.componentInstance.guardarExtension();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
    const request = http.expectOne(url);
    expect(request.request.method).toBe('POST');
    expect(request.request.body.password).toBe('test-secret');
    request.flush({ message: 'No se pudo escribir pjsip.conf' }, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]').textContent).toContain('No se pudo escribir');
    expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    expect(fixture.componentInstance.guardando()).toBe(false);
    fixture.componentInstance.guardarExtension();
    fixture.detectChanges();
    http.expectOne(url).flush({ message: 'Guardado' });
    http.expectOne(url).flush([extension]);
    expect(fixture.componentInstance.editando()).toBe(false);
  });

  for (const editing of [false, true]) {
    it(`refreshes the list without saving when cancelling ${editing ? 'editing' : 'creation'}`, async () => {
      http.expectOne(url).flush([extension]);
      if (editing) {
        fixture.componentInstance.seleccionarExtension(extension);
      } else {
        fixture.componentInstance.nuevaExtension();
      }
      fixture.componentInstance.extActual.update(ext => ({ ...ext, caller_id: 'Unsaved caller' }));
      fixture.detectChanges();
      await fixture.whenStable();
      const cancel = Array.from(fixture.nativeElement.querySelectorAll('form button') as NodeListOf<HTMLButtonElement>)
        .find(button => button.textContent?.trim() === es.CANCELAR)!;
      cancel.click();
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('form')).toBeNull();
      const refresh = http.expectOne(url);
      expect(refresh.request.method).toBe('GET');
      refresh.flush([{ ...extension, caller_id: 'Refreshed caller' }]);
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('.sip-users-lista').textContent).toContain('Refreshed caller');
      expect(fixture.nativeElement.textContent).not.toContain('Unsaved caller');
    });

    it(`refreshes and returns to the list after ${editing ? 'updating' : 'creating'} an extension`, async () => {
      http.expectOne(url).flush([extension]);
      if (editing) {
        fixture.componentInstance.seleccionarExtension(extension);
      } else {
        fixture.componentInstance.nuevaExtension();
      }
      const saved = { ...extension, caller_id: 'Saved caller' };
      fixture.componentInstance.extActual.set(saved);
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
      const save = http.expectOne(url);
      expect(save.request.method).toBe('POST');
      save.flush({ message: 'Guardado' });
      const refresh = http.expectOne(url);
      expect(refresh.request.method).toBe('GET');
      refresh.flush([saved]);
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('form')).toBeNull();
      expect(fixture.nativeElement.querySelector('.sip-users-lista').textContent).toContain('Saved caller');
    });
  }

  it('can retry loading after an API error', () => {
    http.expectOne(url).flush({ message: 'Revisa permisos' }, { status: 500, statusText: 'Error' });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Revisa permisos');
    fixture.nativeElement.querySelector('.sip-users-toolbar button').click();
    http.expectOne(url).flush([]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('No hay extensiones');
  });

  it('does not submit an empty form', async () => {
    http.expectOne(url).flush([]);
    fixture.componentInstance.nuevaExtension();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
    http.expectNone(url);
  });

  it('uses the shared password eye without submitting or modifying the password', async () => {
    http.expectOne(url).flush([extension]);
    fixture.componentInstance.seleccionarExtension(extension);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const password: HTMLInputElement = fixture.nativeElement.querySelector('app-password-field input');
    const eye: HTMLButtonElement = fixture.nativeElement.querySelector('.pf-ojo');
    expect(password.id).toBe('pjsip-password');
    expect(password.type).toBe('password');
    expect(password.value).toBe(extension.password);
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
    expect(fixture.componentInstance.extActual().password).toBe(extension.password);
    expect(fixture.nativeElement.querySelector('.sip-users-eliminar i.fa-trash-can')).not.toBeNull();
    http.expectNone(url);
  });

  it('validates the shared password field and sends its edited value', async () => {
    http.expectOne(url).flush([extension]);
    fixture.componentInstance.seleccionarExtension(extension);
    fixture.detectChanges();
    await fixture.whenStable();
    const password: HTMLInputElement = fixture.nativeElement.querySelector('app-password-field input');
    password.value = '';
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    password.value = 'edited-password';
    password.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(false);
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit', { cancelable: true }));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
    const request = http.expectOne(url);
    expect(request.request.body.password).toBe('edited-password');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('fieldset').disabled).toBe(true);
    request.flush({ message: 'Guardado' });
    http.expectOne(url).flush([extension]);
  });

  it('deletes the selected extension and refreshes the list', () => {
    http.expectOne(url).flush([extension]);
    fixture.componentInstance.seleccionarExtension(extension);
    fixture.componentInstance.eliminarExtension(extension.id);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]').textContent).toContain('¿Eliminar la extensión PJSIP 1001?');
    http.expectNone(`${url}/1001`);
    (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
    const request = http.expectOne(`${url}/1001`);
    expect(request.request.method).toBe('DELETE');
    request.flush({ message: 'Eliminado' });
    http.expectOne(url).flush([]);
    expect(fixture.componentInstance.editando()).toBe(false);
    expect(fixture.componentInstance.extensionesList()).toEqual([]);
  });

  it('switches loading, list, empty state and form labels without changing configuration', async () => {
    expect(fixture.nativeElement.querySelector('[role="status"]').textContent).toBe(es.CARGANDO);
    const loading = TestBed.inject(TranslationService).loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush(en);
    await loading;
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="status"]').textContent).toBe(en.CARGANDO);
    expect(fixture.nativeElement.querySelector('h2').textContent).toBe(en.PJSIP_USERS_TITLE);
    http.expectOne(url).flush([extension]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Extension: 1001');
    expect(fixture.nativeElement.textContent).toContain('Caller ID: Test');
    expect(fixture.nativeElement.textContent).toContain('Context: sip-phones');
    fixture.componentInstance.seleccionarExtension(extension);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const before = structuredClone(fixture.componentInstance.extActual());
    for (const label of [en.PJSIP_USERS_ID, en.SIP_USERS_CONTEXT, en.CONTRASENA, en.PJSIP_USERS_CALLER_ID, en.PJSIP_USERS_MAX_CONTACTS, en.GUARDAR, en.CANCELAR, en.ELIMINAR]) {
      expect(fixture.nativeElement.querySelector('form').textContent).toContain(label);
    }
    await TestBed.inject(TranslationService).loadLanguage('es');
    fixture.detectChanges();
    for (const label of [es.PJSIP_USERS_ID, es.SIP_USERS_CONTEXT, es.CONTRASENA, es.PJSIP_USERS_CALLER_ID, es.PJSIP_USERS_MAX_CONTACTS, es.GUARDAR, es.CANCELAR, es.ELIMINAR]) {
      expect(fixture.nativeElement.querySelector('form').textContent).toContain(label);
    }
    expect(fixture.componentInstance.extActual()).toEqual(before);
    fixture.componentInstance.editando.set(false);
    fixture.componentInstance.extensionesList.set([]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain(es.PJSIP_USERS_EMPTY);
    await TestBed.inject(TranslationService).loadLanguage('en');
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain(en.PJSIP_USERS_EMPTY);
    expect(fixture.nativeElement.textContent).toContain(en.PJSIP_USERS_NEW);
    expect(fixture.nativeElement.textContent).toContain(en.RECARGAR_LISTA);
    http.expectNone(url);
  });

  for (const operation of ['load', 'save', 'delete']) {
    it(`translates ${operation} fallback errors when switching languages`, async () => {
      const component = fixture.componentInstance;
      let errorMessage: string;
      if (operation === 'load') {
        errorMessage = 'PJSIP_USERS_LOAD_ERROR';
      } else {
        http.expectOne(url).flush([extension]);
        component.seleccionarExtension(extension);
        errorMessage = operation === 'save' ? 'PJSIP_USERS_SAVE_ERROR' : 'PJSIP_USERS_DELETE_ERROR';
        if (operation === 'save') {
          component.extActual().context = 'edited-context';
          component.guardarExtension();
        } else {
          component.eliminarExtension(extension.id);
          fixture.detectChanges();
          (fixture.nativeElement.querySelector('.confirmacion-confirmar') as HTMLButtonElement).click();
        }
      }
      http.expectOne(operation === 'delete' ? `${url}/1001` : url)
        .flush({}, { status: 503, statusText: 'Unavailable' });
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

  it('uses the selected language and identifier in a cancelled deletion confirmation', async () => {
    http.expectOne(url).flush([extension]);
    const loading = TestBed.inject(TranslationService).loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush(en);
    await loading;
    fixture.componentInstance.eliminarExtension(extension.id);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]').textContent).toContain('Delete PJSIP extension 1001?');
    (fixture.nativeElement.querySelector('.confirmacion-cancelar') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectNone(`${url}/1001`);
    expect(fixture.componentInstance.extensionesList()).toEqual([extension]);
    expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('creates and edits PJSIP extensions without callsign warnings', () => {
    http.expectOne(url).flush([]);
    for (const id of ['2001', 'HC1LNM', 'phone-office']) {
      for (const editing of [false, true]) {
        if (editing) {
          fixture.componentInstance.seleccionarExtension({ ...extension, id });
          fixture.componentInstance.extActual().context = 'edited-context';
        } else {
          fixture.componentInstance.nuevaExtension();
          fixture.componentInstance.extActual.set({ ...extension, id });
        }
        fixture.componentInstance.guardarExtension();
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('[role="alertdialog"]')).toBeNull();
        const request = http.expectOne(url);
        expect(request.request.method).toBe('POST');
        expect(request.request.body.id).toBe(id);
        request.flush({ message: 'Guardado' });
        http.expectOne(url).flush([]);
      }
    }
  });
});

describe('IAX/PJSIP admin routes', () => {
  for (const module of ['iax', 'pjsip']) {
    it(`activates the actual lazy ${module}-users route and loads its API`, async () => {
      TestBed.configureTestingModule({
        providers: [
          provideRouter(routes), provideHttpClient(), provideHttpClientTesting(),
          { provide: AuthService, useValue: { esAdmin: () => true } },
          { provide: TranslationService, useValue: { currentLang: () => 'es', revision: () => 0, translate: (key: string) => key } },
        ],
      });
      const harness = await RouterTestingHarness.create();
      await harness.navigateByUrl(`/admin/${module}-users`);
      const http = TestBed.inject(HttpTestingController);
      http.expectOne(`${environment.apiUrl}/${module}-users`).flush(module === 'iax' ? {} : []);
      harness.detectChanges();
      expect(harness.routeNativeElement?.querySelector('h2')?.textContent).toContain(module.toUpperCase());
      expect(harness.routeNativeElement?.querySelector('app-admin-volver')).not.toBeNull();
      http.verify();
    });
  }
});
