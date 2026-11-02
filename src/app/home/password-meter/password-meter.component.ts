import { Component, DestroyRef, Input, OnInit, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { PasswordEvaluatorService } from '../../core/services/password-evaluator.service';
import { PasswordMetrics } from '../../core/models/password-meter.model';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

@Component({
  selector: 'app-password-meter',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, TranslatePipe],
  templateUrl: './password-meter.component.html',
  styleUrls: ['./password-meter.component.css']
})
export class PasswordMeterComponent implements OnInit {
  private readonly destroyRef = inject(DestroyRef);
  passwordControl = new FormControl('');
  showPassword = false; // Controla la visibilidad del input (máscara / texto)
  metrics!: PasswordMetrics;
  get recommendations(): string[] {
    if (!this.passwordControl.value) {
      return ['PASSWORD_METER_RECOMMEND_LENGTH'];
    }

    const recommendations: string[] = [];
    const details = this.metrics.details;
    const length = Array.from(this.passwordControl.value).length;
    const maximumEntropy = length > 1 ? length * Math.log2(length) : 0;
    const entropyRatio = maximumEntropy > 0 ? this.metrics.shannonEntropyBits / maximumEntropy : 0;
    const characterTypes = [
      details.alphaUC > 0,
      details.alphaLC > 0,
      details.number > 0,
      details.symbol > 0
    ].filter(Boolean).length;

    if (length < 12) {
      recommendations.push('PASSWORD_METER_RECOMMEND_LENGTH');
    }
    if (entropyRatio < 0.75 && length > 1) {
      recommendations.push('PASSWORD_METER_RECOMMEND_DIVERSITY');
    }
    if (characterTypes < 3) {
      recommendations.push('PASSWORD_METER_RECOMMEND_CHARACTER_TYPES');
    }
    if (details.repChar > 0) {
      recommendations.push('PASSWORD_METER_RECOMMEND_REPETITION');
    }
    if (details.consecAlphaUC + details.consecAlphaLC + details.consecNumber > 0) {
      recommendations.push('PASSWORD_METER_RECOMMEND_CONSECUTIVE');
    }
    if (details.seqAlpha + details.seqNumber > 0) {
      recommendations.push('PASSWORD_METER_RECOMMEND_SEQUENCES');
    }

    return recommendations.length ? recommendations : ['PASSWORD_METER_RECOMMEND_GOOD'];
  }
  @Input() set password(value: string) {
    if (this.passwordControl.value !== value) {
      this.passwordControl.setValue(value);
    }
  }

  constructor(private pwdEvaluator: PasswordEvaluatorService) {
    this.metrics = this.pwdEvaluator.evaluate('');
  }

  ngOnInit(): void {
    this.passwordControl.valueChanges.pipe(takeUntilDestroyed(this.destroyRef)).subscribe(value => {
      this.metrics = this.pwdEvaluator.evaluate(value || '');
    });
    this.metrics = this.pwdEvaluator.evaluate(this.passwordControl.value || '');
  }

  toggleMask(): void {
    this.showPassword = !this.showPassword;
  }

  get complexityTranslationKey(): string {
    const complexityKeys: Record<PasswordMetrics['complexity'], string> = {
      'Too Short': 'PASSWORD_METER_COMPLEXITY_TOO_SHORT',
      'Very Weak': 'PASSWORD_METER_COMPLEXITY_VERY_WEAK',
      'Weak': 'PASSWORD_METER_COMPLEXITY_WEAK',
      'Good': 'PASSWORD_METER_COMPLEXITY_GOOD',
      'Strong': 'PASSWORD_METER_COMPLEXITY_STRONG',
      'Very Strong': 'PASSWORD_METER_COMPLEXITY_VERY_STRONG'
    };
    return complexityKeys[this.metrics.complexity];
  }
}
