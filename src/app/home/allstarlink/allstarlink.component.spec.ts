import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AllstarlinkComponent } from './allstarlink.component';
import { environment } from '../../core/environment';

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
    http.expectOne(`${environment.apiUrl}/allstarlink/activity`).flush({ status: 'success', data: [] });
    http.expectOne(`${environment.apiUrl}/allstarlink/nodes`).flush({
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
});
