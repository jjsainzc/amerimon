import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { EnlacesExternosService } from '../../../core/services/enlaces-externos.service';
import { TranslationService } from '../../../core/services/translation.service';
import { EnlacesExternosAdminComponent } from './enlaces-externos-admin.component';

describe('External links save changes', () => {
  it('requires valid changed values, supports reverting and retains edits after an error', async () => {
    const service = {
      listarAdmin: vi.fn(() => of([])),
      actualizar: vi.fn(() => throwError(() => ({ error: { message: 'Save failed' } }))),
      crear: vi.fn(),
    };
    TestBed.configureTestingModule({
      imports: [EnlacesExternosAdminComponent],
      providers: [
        provideRouter([]),
        { provide: EnlacesExternosService, useValue: service },
        { provide: TranslationService, useValue: { revision: signal(0), translate: (key: string) => key } },
      ],
    });
    const fixture = TestBed.createComponent(EnlacesExternosAdminComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.abrirEditar({ dashid: 1, descripcion: 'Original', url: 'https://example.com', target: '_blank', is_active: 1 });
    fixture.detectChanges();
    const save: HTMLButtonElement = fixture.nativeElement.querySelector('button[type="submit"]');
    expect(save.disabled).toBe(true);
    component.guardar();
    expect(service.actualizar).not.toHaveBeenCalled();
    const active: HTMLInputElement = fixture.nativeElement.querySelector('input[type="checkbox"]');
    active.click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(save.disabled).toBe(false);
    active.click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    const url: HTMLInputElement = fixture.nativeElement.querySelector('input[formControlName="url"]');
    url.value = 'invalid';
    url.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(save.disabled).toBe(true);
    url.value = 'https://example.org';
    url.dispatchEvent(new Event('input'));
    component.guardar();
    fixture.detectChanges();
    expect(component.error()).toBe('Save failed');
    expect(save.disabled).toBe(false);
    component.cerrarFormulario();
    component.abrirNuevo();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(true);
    component.form.patchValue({ descripcion: 'New', url: 'https://example.net' });
    fixture.componentRef.changeDetectorRef.markForCheck();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button[type="submit"]').disabled).toBe(false);
  });
});
