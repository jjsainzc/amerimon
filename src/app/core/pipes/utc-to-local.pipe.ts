import { formatDate } from '@angular/common';
import { LOCALE_ID, Pipe, PipeTransform, inject } from '@angular/core';

@Pipe({ name: 'utcToLocal', standalone: true })
export class UtcToLocalPipe implements PipeTransform {
  private readonly locale = inject(LOCALE_ID);

  /**
   * Formatea en hora local las fechas recibidas; interpreta como UTC los textos sin zona.
   * Devuelve null para valores vacíos y conserva los textos que no se puedan interpretar.
   */
  transform(value: string | number | Date | null | undefined, format = 'medium'): string | null {
    if (value == null || value === '') { return null; }

    let dateValue: string | number | Date = value;
    if (typeof value === 'string') {
      const normalized = value.trim()
        .replace(/^(\d{4})\/(\d{2})\/(\d{2})(?=[ T])/, '$1-$2-$3')
        .replace(' ', 'T');
      dateValue = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(normalized) ? normalized : `${normalized}Z`;
    }

    const date = new Date(dateValue);
    if (Number.isNaN(date.getTime())) { return typeof value === 'string' ? value : null; }
    return formatDate(date, format, this.locale);
  }
}
