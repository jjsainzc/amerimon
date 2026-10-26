import { Component, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslationService } from '../../core/services/translation.service';
import { PasswordFieldComponent } from './password-field.component';

@Component({
  standalone: true,
  imports: [FormsModule, PasswordFieldComponent],
  styles: ['.campo-password { display: grid; }'],
  template: `
    <form>
      <div>
        <label for="new-password">Nueva contraseña</label>
        <app-password-field inputId="new-password" name="nueva" [(ngModel)]="nueva" (visibilityChange)="nuevaVisible = $event" />
      </div>
      <div class="campo-password" [style.display]="nuevaVisible ? 'none' : 'grid'">
        <label for="repeat-password">Repetir contraseña</label>
        <app-password-field inputId="repeat-password" name="repetir" [(ngModel)]="repetir" [required]="!nuevaVisible" />
      </div>
      <p role="status">{{ nueva === repetir ? 'coinciden' : 'no coinciden' }}</p>
    </form>
  `
})
class PasswordFieldHostComponent {
  nueva = 'nueva-clave-123';
  repetir = 'otra-clave-456';
  nuevaVisible = false;
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

  it('hides confirmation while revealing the new password and preserves its value for validation', () => {
    const element = fixture.nativeElement as HTMLElement;
    const nueva = element.querySelector<HTMLInputElement>('#new-password')!;

    element.querySelector<HTMLButtonElement>('app-password-field[name="nueva"] button')?.click();
    fixture.detectChanges();

    expect(nueva.type).toBe('text');
    expect(nueva.value).toBe('nueva-clave-123');
    const repeatContainer = element.querySelector('#repeat-password')?.closest('div')!;
    expect(getComputedStyle(repeatContainer).display).toBe('none');
    expect(element.querySelector('[role="status"]')?.textContent).toContain('no coinciden');

    element.querySelector<HTMLButtonElement>('app-password-field[name="nueva"] button')?.click();
    fixture.detectChanges();
    const repetir = element.querySelector<HTMLInputElement>('#repeat-password')!;
    expect(getComputedStyle(repeatContainer).display).toBe('grid');
    expect(repetir.value).toBe('otra-clave-456');
    repetir.value = 'nueva-clave-123';
    repetir.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    expect(element.querySelector('[role="status"]')?.textContent).toContain('coinciden');
  });
});
