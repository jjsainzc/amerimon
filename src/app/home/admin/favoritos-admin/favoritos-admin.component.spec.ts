import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { FavoritosService } from '../../../core/services/favoritos.service';
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
    TestBed.configureTestingModule({
      imports: [FavoritosAdminComponent],
      providers: [
        { provide: FavoritosService, useValue: service },
        provideRouter([]),
      ],
    });
    fixture = TestBed.createComponent(FavoritosAdminComponent);
    fixture.detectChanges();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('searches by number, indicativo or description and lets the admin choose a matching node', () => {
    vi.useFakeTimers();
    const component = fixture.componentInstance;
    component.abrirNuevo();
    component.form.controls.busqueda.setValue('HC1LNM');
    vi.advanceTimersByTime(300);
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
});
