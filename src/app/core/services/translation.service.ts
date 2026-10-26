import { Injectable, signal, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { DOCUMENT } from '@angular/common';

@Injectable({
  providedIn: 'root'
})
export class TranslationService {
  public currentLang = signal<string>('es');

  // Caché en memoria para guardar los JSONs ya descargados y evitar recargas
  private loadedLanguages: Record<string, any> = {};
  private document = inject(DOCUMENT);

  // Diccionario activo actual
  private translations: Record<string, any> = {};

  constructor(private http: HttpClient) {
    // Carga inicial por defecto
    this.loadLanguage('es');
  }

  async loadLanguage(lang: string) {
    try {
      // document.baseURI obtiene automáticamente la URL base correcta (ej: https://.../amerimon/mon/)
      const baseUrl = this.document.baseURI;

      // const url = `${baseUrl}assets/i18n/${lang}.json`;
      const url = `./assets/i18n/${lang}.json`;

      const data = await firstValueFrom(this.http.get<any>(url));

      // Creamos un nuevo objeto para forzar el cambio de referencia
      this.translations = { ...data };

      // Actualizamos la señal
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
