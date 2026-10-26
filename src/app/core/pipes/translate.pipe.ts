import { Pipe, PipeTransform, inject, computed } from '@angular/core';
import { TranslationService } from '../services/translation.service';

@Pipe({
  name: 'translate',
  pure: false // Sigue siendo impuro para evaluar en cada ciclo
})
export class TranslatePipe implements PipeTransform {
  private translationService = inject(TranslationService);

  transform(key: string): string {
    // Registramos la dependencia de la señal reactiva
    this.translationService.currentLang();

    // Retornamos la traducción
    return this.translationService.translate(key);
  }
}
