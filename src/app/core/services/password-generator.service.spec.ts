import { PasswordGeneratorService } from './password-generator.service';
import { PasswordEvaluatorService } from './password-evaluator.service';

describe('PasswordGeneratorService', () => {
  const service = new PasswordGeneratorService();
  const evaluator = new PasswordEvaluatorService();

  it('matches the C# generator length, selected character groups, and global case-insensitive uniqueness', () => {
    const password = service.getPassword(16, 'mMne');

    expect(password).toHaveLength(16);
    expect(password).toMatch(/[a-z]/);
    expect(password).toMatch(/[A-Z]/);
    expect(password).toMatch(/[0-9]/);
    expect(password).toMatch(/[^a-zA-Z0-9]/);
    expect(new Set([...password.toLowerCase()]).size).toBe(password.length);
    expect(evaluator.calculateShannonEntropy(password)).toBeCloseTo(password.length * Math.log2(password.length));
  });

  it('preserves the C# character-group order for the first generated characters', () => {
    const password = service.getPassword(8, 'mMne');

    expect(password[0]).toMatch(/[a-z]/);
    expect(password[1]).toMatch(/[0-9]/);
    expect(password[2]).toMatch(/[!.,;{}^*+$%&\/()=?¿¡]/);
    expect(password[3]).toMatch(/[A-Z]/);
  });

  it('allows lengths above 64 and balances character reuse after unique characters are exhausted', () => {
    const password = service.getPassword(65, 'mMne');

    expect(password).toHaveLength(65);
    expect(new Set([...password.toLowerCase()]).size).toBeLessThan(65);
    expect(evaluator.calculateShannonEntropy(password)).toBeGreaterThan(65 * Math.log2(65) * 0.9);
  });

  it('rejects lengths below 8', () => {
    expect(() => service.getPassword(7, 'mM')).toThrow(RangeError);
  });

  it('uses the C# default source for an empty source and rejects a one-character source', () => {
    const password = service.getPassword(16, '');
    expect(password).toHaveLength(16);
    expect(password).toMatch(/[a-z]/);
    expect(password).toMatch(/[A-Z]/);
    expect(password).toMatch(/[0-9]/);
    expect(password).toMatch(/[^a-zA-Z0-9]/);
    expect(() => service.getPassword(12, 'm')).toThrow('PASSWORD_TOOLS_ERROR_CRITERIA');
  });

  it('preserves the C# behavior for source strings without recognized types', () => {
    expect(service.getPassword(12, 'xy')).toBe('');
  });

});
