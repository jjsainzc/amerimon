import { Pipe, PipeTransform, inject } from '@angular/core';
import { TranslationService } from '../services/translation.service';

@Pipe({
  name: 'translate',
  pure: false // Importante para que detecte los cambios de idioma en tiempo real
})
export class TranslatePipe implements PipeTransform {
  private translationService = inject(TranslationService);

  transform(key: string): string {
    // Dependemos de la signal para obligar a actualizar la vista cuando cambie el idioma
    this.translationService.currentLang();
    return this.translationService.translate(key);
  }
}
