import { Component, forwardRef, input, signal } from '@angular/core';
import { ControlValueAccessor, NG_VALUE_ACCESSOR } from '@angular/forms';

/** Campo de contraseña con botón de ojo para mostrarla u ocultarla; funciona con ngModel. */
@Component({
  selector: 'app-password-field',
  standalone: true,
  providers: [{ provide: NG_VALUE_ACCESSOR, useExisting: forwardRef(() => PasswordFieldComponent), multi: true }],
  templateUrl: './password-field.component.html',
  styleUrl: './password-field.component.css'
})
export class PasswordFieldComponent implements ControlValueAccessor {
  readonly autocomplete = input('off');
  readonly placeholder = input('');
  readonly required = input(false);
  readonly minlength = input<number | null>(null);

  readonly valor = signal('');
  readonly visible = signal(false);
  readonly deshabilitado = signal(false);

  private onChange: (v: string) => void = () => {};
  protected onTouched: () => void = () => {};

  alEscribir(v: string): void {
    this.valor.set(v);
    this.onChange(v);
  }

  writeValue(v: string | null): void {
    this.valor.set(v ?? '');
  }

  registerOnChange(fn: (v: string) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(disabled: boolean): void {
    this.deshabilitado.set(disabled);
  }
}
