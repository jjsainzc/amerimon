import { Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class TranslationService {
  public currentLang = signal<string>('es');
  private translations: Record<string, any> = {};

  constructor(private http: HttpClient) {
    // Carga inicial por defecto
    this.loadLanguage('es');
  }

  async loadLanguage(lang: string) {
    try {
      const data = await firstValueFrom(this.http.get<any>(`assets/i18n/${lang}.json`));
      this.translations = data;
      this.currentLang.set(lang);
    } catch (error) {
      console.error(`No se pudo cargar el idioma: ${lang}`, error);
    }
  }

  // Función para obtener la traducción de una clave
  translate(key: string): string {
    return this.translations[key] || key;
  }
}
