import { Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { environment } from '../environment';

type Translations = Record<string, string>;

@Injectable({
  providedIn: 'root'
})
export class TranslationService {
  public currentLang = signal<string>('es');
  public readonly revision = signal(0);

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
      this.revision.update(revision => revision + 1);

      console.log('Idioma cambiado exitosamente a:', lang);
    } catch (error) {
      console.error(`No se pudo cargar el idioma: ${lang}`, error);
    }
  }

  translate(key: string, params?: Record<string, string | number>): string {
    const translation = this.translations[key] || key;
    if (!params) { return translation; }
    return translation.replace(/\$\{\s*(\w+)\s*\}|\{\{\s*(\w+)\s*\}\}/g, (placeholder, dollarName, braceName) => {
      const name = dollarName ?? braceName;
      return name !== undefined && Object.hasOwn(params, name) ? String(params[name]) : placeholder;
    });
  }

  /** Persiste el idioma elegido en el servidor para que la pantalla Nextion reciba sus textos. */
  syncServerLanguage(lang: string): void {
    this.http.put(`${environment.apiUrl}/language`, { language: lang }).subscribe({
      error: error => console.warn('No se pudo sincronizar el idioma con el servidor', error),
    });
  }
}
