import { Pipe, PipeTransform, inject } from '@angular/core';
import { TranslationService } from '../services/translation.service';

@Pipe({
  name: 'translate',
  pure: false // Sigue siendo impuro para evaluar en cada ciclo
})
export class TranslatePipe implements PipeTransform {
  private translationService = inject(TranslationService);

  transform(key: string, params?: Record<string, string | number>): string {
    // Registramos la dependencia de la señal reactiva
    this.translationService.currentLang();

    // Retornamos la traducción
    const translation = this.translationService.translate(key);
    if (!params) { return translation; }
    return translation.replace(/\{\{\s*(\w+)\s*\}\}/g, (placeholder: string, name: string) =>
      Object.hasOwn(params, name) ? String(params[name]) : placeholder);
  }
}
