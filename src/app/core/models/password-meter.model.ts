export type PasswordStatus = 'fail' | 'pass' | 'exceed' | 'warn';
export type ComplexityLevel = 'Too Short' | 'Very Weak' | 'Weak' | 'Good' | 'Strong' | 'Very Strong';

export interface PasswordMetrics {
  score: number;
  complexity: ComplexityLevel;
  shannonEntropyBits: number;
  shannonEntropyPerCharacter: number;
  details: {
    length: number;
    alphaUC: number;
    alphaLC: number;
    number: number;
    symbol: number;
    midChar: number;
    requirements: number;
    alphasOnly: number;
    numbersOnly: number;
    repChar: number;
    consecAlphaUC: number;
    consecAlphaLC: number;
    consecNumber: number;
    seqAlpha: number;
    seqNumber: number;
  };
  bonuses: {
    length: string;
    alphaUC: string;
    alphaLC: string;
    number: string;
    symbol: string;
    midChar: string;
    requirements: string;
    alphasOnly: string;
    numbersOnly: string;
    repChar: string;
    consecAlphaUC: string;
    consecAlphaLC: string;
    consecNumber: string;
    seqAlpha: string;
    seqNumber: string;
  };
  statuses: {
    [key: string]: PasswordStatus;
  };
}
