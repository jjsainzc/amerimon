import { PasswordEvaluatorService } from './password-evaluator.service';

describe('PasswordEvaluatorService Shannon entropy', () => {
  const service = new PasswordEvaluatorService();

  it('calculates total entropy for equally frequent characters', () => {
    expect(service.calculateShannonEntropy('')).toBe(0);
    expect(service.calculateShannonEntropy('aaaa')).toBe(0);
    expect(service.calculateShannonEntropy('aabb')).toBe(4);
  });

  it('uses the observed frequencies for a non-uniform distribution', () => {
    expect(service.calculateShannonEntropy('aaab')).toBeCloseTo(3.2451124978);
  });

  it('counts Unicode code points as characters', () => {
    expect(service.calculateShannonEntropy('😀😀😃😃')).toBe(4);
  });

  it('returns Shannon entropy in the password metrics', () => {
    const metrics = service.evaluate('aabb');

    expect(metrics.shannonEntropyBits).toBe(4);
    expect(metrics.shannonEntropyPerCharacter).toBe(1);
  });
});
