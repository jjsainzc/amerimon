import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { Subject, of } from 'rxjs';
import { vi } from 'vitest';
import { FavoritosService } from '../../../core/services/favoritos.service';
import { TranslationService } from '../../../core/services/translation.service';
import { FavoritosAdminComponent } from './favoritos-admin.component';

describe('FavoritosAdminComponent', () => {
  let fixture: ComponentFixture<FavoritosAdminComponent>;
  let service: {
    listar: ReturnType<typeof vi.fn>;
    buscarAstdb: ReturnType<typeof vi.fn>;
    descripcionAstdb: ReturnType<typeof vi.fn>;
    crear: ReturnType<typeof vi.fn>;
    actualizar: ReturnType<typeof vi.fn>;
    borrar: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    service = {
      listar: vi.fn(() => of([])),
      buscarAstdb: vi.fn(() => of([
        { nodo: '700001', callsign: 'HC1LNM', descripcion: 'Principal', ubicacion: 'Quito' },
        { nodo: '700002', callsign: 'HC1LNM', descripcion: 'Secundario', ubicacion: 'Quito' },
      ])),
      descripcionAstdb: vi.fn(() => of({ descripcion: '' })),
      crear: vi.fn(() => of({ id: 1, numero: '700001', descripcion: '' })),
      actualizar: vi.fn(),
      borrar: vi.fn(),
    };
    const translations: Record<string, string> = {
      BUSQUEDA_FAVORITO: 'Búsqueda *',
      BUSCAR_EN_ASTDB_Y_ECHOLINK: 'Buscar en AstDB y EchoLink',
      NODO_SELECCIONADO: 'Nodo ${nodo} seleccionado.',
      RESULTADOS_NODOS_LIMITADOS: 'Se encontraron ${total} nodos. Solo se muestran los primeros 50.'
    };
    TestBed.configureTestingModule({
      imports: [FavoritosAdminComponent],
      providers: [
        { provide: FavoritosService, useValue: service },
        {
          provide: TranslationService,
          useValue: {
            currentLang: () => 'es',
            revision: signal(0),
            translate: (key: string, params?: Record<string, string | number>) =>
              Object.entries(params ?? {}).reduce(
                (text, [name, value]) => text.replaceAll(`\${${name}}`, String(value)),
                translations[key] ?? key
              )
          }
        },
        provideRouter([]),
      ],
    });
    fixture = TestBed.createComponent(FavoritosAdminComponent);
    fixture.detectChanges();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('copies search result identities without selecting or saving them', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      const component = fixture.componentInstance;
      component.abrirNuevo();
      component.form.controls.busqueda.setValue('HC1LNM');
      component.buscar();
      fixture.detectChanges();
      const row: HTMLElement = fixture.nativeElement.querySelector('.fa-coincidencia-fila');
      const buttons = row.querySelectorAll<HTMLButtonElement>('.clipboard-copy');
      buttons[0].click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('700001');
      buttons[1].click();
      await fixture.whenStable();
      expect(writeText).toHaveBeenLastCalledWith('HC1LNM');
      expect(component.nodoSeleccionado()).toBe('');
      expect(service.crear).not.toHaveBeenCalled();
      expect(row.querySelector('button button')).toBeNull();
    } finally {
      Reflect.deleteProperty(navigator, 'clipboard');
    }
  });

  it('searches by number, indicativo or description and lets the admin choose a matching node', () => {
    const component = fixture.componentInstance;
    component.abrirNuevo();
    component.form.controls.busqueda.setValue('HC1LNM');
    fixture.detectChanges();
    expect(service.buscarAstdb).not.toHaveBeenCalled();
    fixture.nativeElement.querySelector('[aria-label="Buscar en AstDB y EchoLink"]').click();
    fixture.detectChanges();

    expect(service.buscarAstdb).toHaveBeenCalledWith('HC1LNM');
    expect(fixture.nativeElement.querySelectorAll('input').length).toBe(2);
    expect(fixture.nativeElement.textContent).toContain('Búsqueda');
    expect(fixture.nativeElement.textContent).not.toContain('Indicativo (opcional)');
    expect(fixture.nativeElement.textContent).not.toContain('Número del nodo');
    const choices: NodeListOf<HTMLButtonElement> = fixture.nativeElement.querySelectorAll('.fa-coincidencia');
    expect(choices.length).toBe(2);
    choices[1].click();
    fixture.detectChanges();

    expect(component.nodoSeleccionado()).toBe('700002');
    expect(component.estadoBusqueda()).toContain('700002');
    component.guardar();

    expect(service.crear).toHaveBeenCalledWith({
      numero: '700002',
      descripcion: '',
    });
  });

  it('selects and saves an EchoLink destination with the connection prefix', () => {
    service.buscarAstdb.mockReturnValue(of([
      { nodo: '3996546', callsign: 'W2LRC-R', descripcion: 'EchoLink', ubicacion: '165.227.99.201' },
    ]));
    const component = fixture.componentInstance;
    component.abrirNuevo();
    component.form.controls.busqueda.setValue('W2LRC-R');
    component.buscar();
    fixture.detectChanges();

    expect(component.nodoSeleccionado()).toBe('3996546');
    expect(component.sugerida()).toBe('W2LRC-R EchoLink 165.227.99.201');
    expect(fixture.nativeElement.textContent).toContain('3996546');
    component.guardar();
    expect(service.crear).toHaveBeenCalledWith({ numero: '3996546', descripcion: '' });
  });

    it.each([50, 51, 5000])('renders at most 50 results and warns only above 50 (%i matches)', count => {
      const component = fixture.componentInstance;
      service.buscarAstdb.mockReturnValue(of(Array.from({ length: count }, (_, i) => ({
        nodo: String(700000 + i), callsign: 'TEST', descripcion: 'Nodo', ubicacion: ''
      }))));
      component.abrirNuevo();
      component.form.controls.busqueda.setValue('TEST');
      component.buscar();
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelectorAll('.fa-coincidencia').length).toBe(50);
      expect(!!component.avisoBusqueda()).toBe(count > 50);
      if (count > 50) {
        expect(component.avisoBusqueda()).toContain(String(count));
        expect(component.avisoBusqueda()).toContain('primeros 50');
      }
    });

    it('searches with Enter without submitting the favorite', () => {
      const component = fixture.componentInstance;
      component.abrirNuevo();
      component.form.controls.busqueda.setValue('HC1LNM');
      fixture.detectChanges();
      const input = fixture.nativeElement.querySelector('input[formControlName="busqueda"]');
      const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
      input.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
      expect(service.buscarAstdb).toHaveBeenCalledWith('HC1LNM');
      expect(service.crear).not.toHaveBeenCalled();
    });

    it('cancels pending results on editing or closing without starting another search', () => {
      const results = new Subject<never[]>();
      service.buscarAstdb.mockReturnValue(results.asObservable());
      const component = fixture.componentInstance;
      component.abrirNuevo();
      component.form.controls.busqueda.setValue('TEST');
      component.buscar();
      expect(component.buscando()).toBe(true);
      component.form.controls.busqueda.setValue('OTHER');
      results.next([]);
      expect(component.buscando()).toBe(false);
      expect(component.coincidencias()).toEqual([]);
      expect(component.estadoBusqueda()).toBe('');
      expect(service.buscarAstdb).toHaveBeenCalledTimes(1);
      component.buscar();
      component.cerrarFormulario();
      expect(component.buscando()).toBe(false);
      expect(component.formularioAbierto()).toBe(false);
      results.complete();
    });
});
