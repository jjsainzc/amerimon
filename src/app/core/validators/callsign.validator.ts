import { AbstractControl, ValidationErrors, ValidatorFn } from '@angular/forms';

const SUFIJOS_PORTABLES = new Set(['P', 'M', 'MM', 'AM', 'QRP', 'QRPP', 'LH']);

function esIndicativoBase(valor: string): boolean {
  const partes = valor.match(/^([A-Z0-9]{1,3})([0-9])([A-Z0-9]{0,3}[A-Z])$/);
  return !!partes && /[A-Z]/.test(partes[1]);
}

export function indicativoQrz(entrada: string | null | undefined): string | null {
  const valor = (entrada ?? '').trim().toUpperCase().replace(/\s+/g, '');
  if (!valor || valor === 'GUEST') { return null; }

  const partes = valor.split('/');
  if (partes.length === 1) { return esIndicativoBase(valor) ? valor : null; }
  if (partes.length !== 2) { return null; }

  const [principal, modificador] = partes;
  if (esIndicativoBase(principal) &&
      (SUFIJOS_PORTABLES.has(modificador) || /^\d{1,3}$/.test(modificador))) {
    return principal;
  }
  return /^[A-Z0-9]{1,4}$/.test(principal) && esIndicativoBase(modificador) ? modificador : null;
}

export function callsignValido(sinIndicativo: () => boolean = () => false): ValidatorFn {
  return (control: AbstractControl): ValidationErrors | null => {
    const valor = (control.value ?? '').toString().trim().toUpperCase().replace(/\s+/g, '');
    if (!valor) { return null; }
    if (valor === 'GUEST') {
      return sinIndicativo() ? null : { indicativoReservado: true };
    }

    return indicativoQrz(valor) ? null : { indicativoInvalido: true };
  };
}
