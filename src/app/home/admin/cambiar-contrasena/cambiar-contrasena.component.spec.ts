import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { CambiarContrasenaComponent } from './cambiar-contrasena.component';
import { environment } from '../../../core/environment';

@Component({ standalone: true, template: '' })
class TestPageComponent {}

describe('CambiarContrasenaComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [CambiarContrasenaComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([{ path: 'allstarlink', component: TestPageComponent }]),
      ],
    }).compileComponents();
  });

  afterEach(() => {
    const http = TestBed.inject(HttpTestingController);
    http.match(request => request.url === './assets/i18n/es.json').forEach(request => request.flush({}));
    http.verify();
  });

  it('skips repeat validation when the new password is visible', async () => {
    const fixture = TestBed.createComponent(CambiarContrasenaComponent);
    fixture.detectChanges();
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;

    const actual = element.querySelector<HTMLInputElement>('#actual')!;
    actual.value = 'old-password';
    actual.dispatchEvent(new Event('input', { bubbles: true }));
    const nueva = element.querySelector<HTMLInputElement>('#nueva')!;
    nueva.value = 'pass1234';
    nueva.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    element.querySelector<HTMLButtonElement>('#nueva')!.parentElement!.querySelector<HTMLButtonElement>('.pf-ojo')!.click();
    await fixture.whenStable();

    element.querySelector<HTMLFormElement>('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    const request = TestBed.inject(HttpTestingController).expectOne(`${environment.apiUrl}/auth/password`);
    expect(request.request.body).toEqual({ current: 'old-password', new: 'pass1234' });
    request.flush({ status: 'ok' });
    await fixture.whenStable();
  });
});
