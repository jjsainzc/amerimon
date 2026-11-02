import { Injectable } from '@angular/core';

@Injectable({
  providedIn: 'root'
})
export class PasswordGeneratorService {
  private readonly numbers = '0123456789';
  private readonly uppercase = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
  private readonly lowercase = 'abcdefghijklmnopqrstuvwxyz';
  private readonly symbols = '!.,;{}^*+$%&/()=?¿¡';

  private randomIndex(maxExclusive: number): number {
    if (!Number.isInteger(maxExclusive) || maxExclusive < 1 || maxExclusive > 256) {
      throw new RangeError('El tamaño del conjunto aleatorio no es válido.');
    }

    const cryptoApi = globalThis.crypto;
    if (!cryptoApi?.getRandomValues) {
      throw new Error('Este navegador no dispone de un generador seguro de contraseñas.');
    }

    const limit = Math.floor(256 / maxExclusive) * maxExclusive;
    const value = new Uint8Array(1);
    do {
      cryptoApi.getRandomValues(value);
    } while (value[0] >= limit);

    return value[0] % maxExclusive;
  }

  private getRandomChar(characters: string): string {
    return characters[this.randomIndex(characters.length)];
  }

  private makePass(password: string[], characters: string): void {
    const frequencies = new Map<string, number>();
    for (const character of password) {
      const normalized = character.toLowerCase();
      frequencies.set(normalized, (frequencies.get(normalized) ?? 0) + 1);
    }

    const candidates: string[] = [];
    let minimumFrequency = Number.POSITIVE_INFINITY;
    for (const character of characters) {
      const frequency = frequencies.get(character.toLowerCase()) ?? 0;
      if (frequency < minimumFrequency) {
        minimumFrequency = frequency;
        candidates.length = 0;
      }
      if (frequency === minimumFrequency) {
        candidates.push(character);
      }
    }

    password.push(this.getRandomChar(candidates.join('')));
  }

  private endsWithCharacterClass(password: string[], characters: RegExp): boolean {
    const value = password.join('');
    return value.length > 1 && characters.test(value);
  }

  private generateType(type: 'n' | 'e', src: string, password: string[], length: number): void {
    const endsWithType = type === 'n'
      ? this.endsWithCharacterClass(password, /[0-9]$/)
      : password.length > 1 && this.symbols.includes(password[password.length - 1]);
    if (src.includes(type) && password.length < length && !endsWithType) {
      this.makePass(password, type === 'n' ? this.numbers : this.symbols);
    }
  }

  public getPassword(length: number, source: string | null): string {
    if (!Number.isInteger(length) || length < 8) {
      throw new RangeError('PASSWORD_TOOLS_ERROR_LENGTH');
    }

    let src = source ?? '';
    if (src.length === 0) {
      src = 'mMne';
    } else if (src.length === 1) {
      throw new Error('PASSWORD_TOOLS_ERROR_CRITERIA');
    }

    const password: string[] = [];
    let toggle = true;
    let cycle = true;

    for (let index = 0; index < length; index++) {
      if (!cycle) {
        break;
      }

      if (toggle) {
        if (src.includes('m') && !this.endsWithCharacterClass(password, /[a-z]$/)) {
          this.makePass(password, this.lowercase);
          if (password.length >= length) {
            break;
          }
        }
      } else if (src.includes('M') && !this.endsWithCharacterClass(password, /[A-Z]$/)) {
        this.makePass(password, this.uppercase);
        if (password.length >= length) {
          break;
        }
      }

      const types: Array<'n' | 'e'> = toggle ? ['n', 'e'] : ['e', 'n'];
      for (const type of types) {
        this.generateType(type, src, password, length);
        if (password.length >= length) {
          cycle = false;
          break;
        }
      }

      toggle = !toggle;
    }

    return password.join('');
  }

}
