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

  /** Actualiza la contraseña visible para el componente y notifica el cambio al formulario Angular. */
  alEscribir(v: string): void {
    this.valor.set(v);
    this.onChange(v);
  }

  /** Recibe el valor del formulario sin emitir un nuevo cambio hacia él. */
  writeValue(v: string | null): void {
    this.valor.set(v ?? '');
  }

  /** Registra el callback del formulario que recibirá las ediciones de la contraseña. */
  registerOnChange(fn: (v: string) => void): void {
    this.onChange = fn;
  }

  /** Registra el callback que marca el control como tocado al perder el foco. */
  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  /** Sincroniza el estado deshabilitado del formulario con el campo de contraseña. */
  setDisabledState(disabled: boolean): void {
    this.deshabilitado.set(disabled);
  }
}
