import { Component } from '@angular/core';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { PasswordGeneratorComponent } from '../password-generator/password-generator.component';
import { PasswordMeterComponent } from '../password-meter/password-meter.component';

@Component({
  selector: 'app-password-tools',
  standalone: true,
  imports: [PasswordGeneratorComponent, PasswordMeterComponent, TranslatePipe],
  templateUrl: './password-tools.component.html',
  styleUrl: './password-tools.component.css'
})
export class PasswordToolsComponent {
  generatedPassword = '';

  updateGeneratedPassword(password: string): void {
    this.generatedPassword = password;
  }
}
