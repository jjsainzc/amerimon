import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AbstractControl, FormBuilder, FormGroup, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { PasswordGeneratorService } from '../../core/services/password-generator.service';
import { PasswordEvaluatorService } from '../../core/services/password-evaluator.service';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

const minimumPasswordOptions: ValidatorFn = (control: AbstractControl): ValidationErrors | null => {
  const selectedOptions = ['useLower', 'useUpper', 'useNumbers', 'useSymbols']
    .filter(option => control.get(option)?.value).length;
  return selectedOptions === 0 || selectedOptions >= 2 ? null : { minimumPasswordOptions: true };
};

@Component({
  selector: 'app-password-generator',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, TranslatePipe],
  templateUrl: './password-generator.component.html',
  styleUrls: ['./password-generator.component.css']
})
export class PasswordGeneratorComponent implements OnInit {
  @Output() passwordGenerated = new EventEmitter<string>();

  genForm: FormGroup;
  generatedPassword: string = '';
  entropy: number = 0;
  errorMessage: string = '';
  copied: boolean = false;

  constructor(
    private fb: FormBuilder,
    private pwdGenService: PasswordGeneratorService,
    private pwdEvaluator: PasswordEvaluatorService
  ) {
    this.genForm = this.fb.group({
      length: [12, [Validators.required, Validators.min(8), Validators.pattern(/^\d+$/)]],
      useLower: [true],
      useUpper: [true],
      useNumbers: [true],
      useSymbols: [true]
    }, { validators: minimumPasswordOptions });

  }

  ngOnInit(): void {
    this.generate();
  }

  // Construye el string de criterios "src" requerido por tu algoritmo original: m, M, n, e
  private buildSrcString(): string {
    const values = this.genForm.value;
    let src = '';
    if (values.useLower) src += 'm';
    if (values.useUpper) src += 'M';
    if (values.useNumbers) src += 'n';
    if (values.useSymbols) src += 'e';
    return src;
  }

  generate(): void {
    this.errorMessage = '';
    this.copied = false;
    if (this.genForm.invalid) {
      this.generatedPassword = '';
      this.entropy = 0;
      this.errorMessage = this.genForm.get('length')?.invalid
        ? 'PASSWORD_TOOLS_ERROR_LENGTH'
        : this.genForm.hasError('minimumPasswordOptions')
          ? 'PASSWORD_TOOLS_ERROR_CRITERIA'
          : 'PASSWORD_TOOLS_ERROR_CONFIGURATION';
      this.passwordGenerated.emit('');
      return;
    }

    const length = Number(this.genForm.get('length')?.value);
    const src = this.buildSrcString();

    try {
      this.generatedPassword = this.pwdGenService.getPassword(length, src);
      this.entropy = this.pwdEvaluator.calculateShannonEntropy(this.generatedPassword);
      if (!this.generatedPassword || !Number.isFinite(this.entropy)) {
        throw new Error('PASSWORD_TOOLS_ERROR_GENERIC');
      }
      this.passwordGenerated.emit(this.generatedPassword);
    } catch (error: unknown) {
      this.errorMessage = error instanceof Error
        ? error.message.startsWith('PASSWORD_TOOLS_ERROR_')
          ? error.message
          : error instanceof RangeError
            ? 'PASSWORD_TOOLS_ERROR_LENGTH'
            : 'PASSWORD_TOOLS_ERROR_SECURE_RANDOM'
        : 'PASSWORD_TOOLS_ERROR_GENERIC';
      this.generatedPassword = '';
      this.entropy = 0;
      this.passwordGenerated.emit('');
    }
  }

  async copyToClipboard(): Promise<void> {
    if (this.generatedPassword) {
      try {
        if (!navigator.clipboard?.writeText) {
          throw new Error('El portapapeles no está disponible en este navegador.');
        }

        await navigator.clipboard.writeText(this.generatedPassword);
        this.errorMessage = '';
        this.copied = true;
        setTimeout(() => (this.copied = false), 2000);
      } catch (error: unknown) {
        this.copied = false;
        this.errorMessage = 'PASSWORD_TOOLS_ERROR_CLIPBOARD';
      }
    }
  }
}
