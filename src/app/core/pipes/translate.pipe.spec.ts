import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TranslationService } from '../services/translation.service';
import { TranslatePipe } from './translate.pipe';

describe('TranslatePipe', () => {
  let pipe: TranslatePipe;
  const translations: Record<string, string> = {
    TITLE: 'Node configuration',
    NODE: 'Access list for node {{ nodo }}',
    IMPORT: '{{ cantidad }} entries from {{ origen }} to {{ destino }}',
  };

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [{
        provide: TranslationService,
        useValue: {
          currentLang: signal('en'),
          revision: signal(0),
          translate: (key: string, params?: Record<string, string | number>) =>
            Object.entries(params ?? {}).reduce(
              (text, [name, value]) => text.replaceAll(`{{ ${name} }}`, () => String(value)),
              translations[key] || key,
            ),
        },
      }],
    });
    pipe = TestBed.runInInjectionContext(() => new TranslatePipe());
  });

  it('preserves translations without parameters and missing keys', () => {
    expect(pipe.transform('TITLE')).toBe('Node configuration');
    expect(pipe.transform('MISSING')).toBe('MISSING');
  });

  it('substitutes named parameters without evaluating Angular expressions', () => {
    expect(pipe.transform('NODE', { nodo: '616265' })).toBe('Access list for node 616265');
    expect(pipe.transform('IMPORT', { cantidad: 0, origen: '616265', destino: '616266' }))
      .toBe('0 entries from 616265 to 616266');
  });

  it('keeps unknown placeholders and treats parameter values as literal text', () => {
    expect(pipe.transform('NODE', {})).toBe('Access list for node {{ nodo }}');
    expect(pipe.transform('NODE', { nodo: '$&{{ otro }}' })).toBe('Access list for node $&{{ otro }}');
  });
});
