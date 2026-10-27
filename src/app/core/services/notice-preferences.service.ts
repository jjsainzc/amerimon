import { Injectable, signal } from '@angular/core';

const storageKey = 'amerimon.notice-preferences.v1';

@Injectable({ providedIn: 'root' })
export class NoticePreferencesService {
  private readonly hidden = signal<ReadonlySet<string>>(new Set());
  readonly error = signal('');

  constructor() {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]');
      if (!Array.isArray(stored) || !stored.every(key => typeof key === 'string')) {
        throw new Error('Invalid notice preferences');
      }
      this.hidden.set(new Set(stored));
    } catch (error) {
      this.reportError(error);
    }
  }

  isHidden(key: string): boolean {
    return !!key && this.hidden().has(key);
  }

  hide(key: string): void {
    if (!key) { return; }
    const hidden = new Set(this.hidden());
    hidden.add(key);
    try {
      localStorage.setItem(storageKey, JSON.stringify([...hidden]));
      this.hidden.set(hidden);
      this.error.set('');
    } catch (error) {
      this.reportError(error);
    }
  }

  reset(): boolean {
    try {
      localStorage.removeItem(storageKey);
      this.hidden.set(new Set());
      this.error.set('');
      return true;
    } catch (error) {
      this.reportError(error);
      return false;
    }
  }

  private reportError(error: unknown): void {
    console.error('Unable to access notice preferences.', error);
    this.error.set('NOTICE_PREFERENCES_ERROR');
  }
}
