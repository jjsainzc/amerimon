import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { TranslationService } from './translation.service';

describe('TranslationService', () => {
  let service: TranslationService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(TranslationService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function loadSpanish(): Promise<void> {
    const loading = service.loadLanguage('es');
    http.expectOne('./assets/i18n/es.json').flush({ TITLE: 'Configuración' });
    await loading;
  }

  it('shares the initial request and reuses cached dictionaries when switching languages', async () => {
    await loadSpanish();
    expect(service.translate('TITLE')).toBe('Configuración');
    const loading = service.loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush({ TITLE: 'Configuration' });
    await loading;
    expect(service.translate('TITLE')).toBe('Configuration');

    await service.loadLanguage('es');
    expect(service.currentLang()).toBe('es');
    expect(service.translate('TITLE')).toBe('Configuración');
    await service.loadLanguage('en');
    expect(service.currentLang()).toBe('en');
    expect(service.translate('TITLE')).toBe('Configuration');
    http.expectNone('./assets/i18n/es.json');
    http.expectNone('./assets/i18n/en.json');
    expect(service.translate('UNKNOWN')).toBe('UNKNOWN');
  });

  it('shares simultaneous downloads for the same language', async () => {
    await loadSpanish();
    const first = service.loadLanguage('en');
    const second = service.loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush({ TITLE: 'Configuration' });
    await Promise.all([first, second]);
    expect(service.translate('TITLE')).toBe('Configuration');
  });

  it('interpolates both supported placeholder formats and preserves missing parameters', async () => {
    const loading = service.loadLanguage('es');
    http.expectOne('./assets/i18n/es.json').flush({
      DOLLAR: 'Nodo ${node}',
      BRACES: 'Nodo {{ node }}',
      MIXED: '${node} / {{ count }} / ${missing}',
    });
    await loading;

    expect(service.translate('DOLLAR', { node: 123 })).toBe('Nodo 123');
    expect(service.translate('BRACES', { node: '456' })).toBe('Nodo 456');
    expect(service.translate('MIXED', { node: '789', count: 2 })).toBe('789 / 2 / ${missing}');
    expect(service.translate('UNKNOWN', { node: '123' })).toBe('UNKNOWN');
  });

  it('does not replace the latest selection with a late response, but caches it', async () => {
    const spanish = service.loadLanguage('es');
    const english = service.loadLanguage('en');
    http.expectOne('./assets/i18n/en.json').flush({ TITLE: 'Configuration' });
    await english;
    http.expectOne('./assets/i18n/es.json').flush({ TITLE: 'Configuración' });
    await spanish;
    expect(service.currentLang()).toBe('en');
    expect(service.translate('TITLE')).toBe('Configuration');

    await service.loadLanguage('es');
    expect(service.translate('TITLE')).toBe('Configuración');
    http.expectNone('./assets/i18n/es.json');
  });

  it('keeps the cached selection when an earlier download finishes', async () => {
    await loadSpanish();
    const english = service.loadLanguage('en');
    await service.loadLanguage('es');
    http.expectOne('./assets/i18n/en.json').flush({ TITLE: 'Configuration' });
    await english;
    expect(service.currentLang()).toBe('es');
    expect(service.translate('TITLE')).toBe('Configuración');
  });

  it('reports failed downloads, preserves the active dictionary and allows retrying', async () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await loadSpanish();
      const failed = service.loadLanguage('en');
      http.expectOne('./assets/i18n/en.json').flush('Unavailable', {
        status: 503, statusText: 'Service Unavailable',
      });
      await failed;
      expect(log).toHaveBeenCalledWith('No se pudo cargar el idioma: en', expect.anything());
      expect(service.currentLang()).toBe('es');
      expect(service.translate('TITLE')).toBe('Configuración');

      const retry = service.loadLanguage('en');
      http.expectOne('./assets/i18n/en.json').flush({ TITLE: 'Configuration' });
      await retry;
      expect(service.translate('TITLE')).toBe('Configuration');
    } finally {
      log.mockRestore();
    }
  });
});
