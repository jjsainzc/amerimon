import { Component, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslationService } from '../../core/services/translation.service';
import { PasswordFieldComponent } from './password-field.component';

@Component({
  standalone: true,
  imports: [FormsModule, PasswordFieldComponent],
  template: `
    <form>
      <div>
        <label for="new-password">Nueva contraseña</label>
        <app-password-field inputId="new-password" name="nueva" [(ngModel)]="nueva" />
      </div>
      <div>
        <label for="repeat-password">Repetir contraseña</label>
        <app-password-field inputId="repeat-password" name="repetir" [(ngModel)]="repetir" />
      </div>
      <p role="status">{{ nueva === repetir ? 'coinciden' : 'no coinciden' }}</p>
    </form>
  `
})
class PasswordFieldHostComponent {
  nueva = 'nueva-clave-123';
  repetir = 'otra-clave-456';
}

describe('PasswordFieldComponent', () => {
  let fixture: ComponentFixture<PasswordFieldHostComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PasswordFieldHostComponent],
      providers: [{
        provide: TranslationService,
        useValue: { currentLang: signal('es'), translate: (key: string) => key }
      }]
    }).compileComponents();
    fixture = TestBed.createComponent(PasswordFieldHostComponent);
    fixture.detectChanges();
  });

  it('keeps confirmation visible and preserves both values when revealing the new password', () => {
    const element = fixture.nativeElement as HTMLElement;
    const nueva = element.querySelector<HTMLInputElement>('#new-password')!;
    const repetir = element.querySelector<HTMLInputElement>('#repeat-password')!;

    element.querySelector<HTMLButtonElement>('app-password-field[name="nueva"] button')?.click();
    fixture.detectChanges();

    expect(nueva.type).toBe('text');
    expect(nueva.value).toBe('nueva-clave-123');
    expect(element.querySelector('#repeat-password')).not.toBeNull();
    expect(repetir.value).toBe('otra-clave-456');
    expect(element.querySelector('[role="status"]')?.textContent).toContain('no coinciden');

    repetir.value = 'nueva-clave-123';
    repetir.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    expect(element.querySelector('[role="status"]')?.textContent).toContain('coinciden');
  });
});
