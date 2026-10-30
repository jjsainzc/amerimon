import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { PrivateNodesService } from '../../../core/services/private-nodes.service';
import { TranslationService } from '../../../core/services/translation.service';
import { PrivateNodesComponent } from './private-nodes.component';

describe('Private nodes save changes', () => {
  it('tracks edited, added and removed lines, retaining edits after errors and resetting on reload', () => {
    const service = {
      obtener: vi.fn(() => of({ path: '/test/nodes', writable: true, lines: ['1234 = radio@example'] })),
      guardar: vi.fn(() => throwError(() => ({ error: { message: 'Save failed' } }))),
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
});
