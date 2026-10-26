import { Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

type Translations = Record<string, string>;

@Injectable({
  providedIn: 'root'
})
export class TranslationService {
  public currentLang = signal<string>('es');

  private readonly loadedLanguages = new Map<string, Translations>();
  private readonly pendingLanguages = new Map<string, Promise<Translations>>();
  private latestRequest = 0;
  private translations: Translations = {};

  constructor(private http: HttpClient) {
    // Carga inicial por defecto
    this.loadLanguage('es');
  }

  async loadLanguage(lang: string): Promise<void> {
    const request = ++this.latestRequest;
    try {
      let data = this.loadedLanguages.get(lang);
      if (!data) {
        let pending = this.pendingLanguages.get(lang);
        if (!pending) {
          pending = firstValueFrom(this.http.get<Translations>(`./assets/i18n/${lang}.json`))
            .then(translations => {
              this.loadedLanguages.set(lang, translations);
              return translations;
            })
            .finally(() => this.pendingLanguages.delete(lang));
          this.pendingLanguages.set(lang, pending);
        }
        data = await pending;
      }
      // Una descarga anterior puede terminar después de otro cambio de idioma.
      if (request !== this.latestRequest) { return; }
      this.translations = data;
      this.currentLang.set(lang);

      console.log('Idioma cambiado exitosamente a:', lang);
    } catch (error) {
      console.error(`No se pudo cargar el idioma: ${lang}`, error);
    }
  }

  // Función para obtener la traducción de una clave
  translate(key: string): string {
    return this.translations[key] || key;
  }
}
