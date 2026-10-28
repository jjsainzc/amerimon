import { describe, expect, it } from 'vitest';
import { isRadioAmateurCallsign } from './radio-callsign';

describe('isRadioAmateurCallsign', () => {
  it.each(['HC1LNM', 'W2LRC-R', 'VK2ABC/4'])('accepts the callsign %s', callsign => {
    expect(isRadioAmateurCallsign(callsign)).toBe(true);
  });

  it.each(['1001', 'system-user', 'system1user', 'radio'])('rejects the system name %s', name => {
    expect(isRadioAmateurCallsign(name)).toBe(false);
  });

  it('ignores surrounding whitespace and case', () => {
    expect(isRadioAmateurCallsign(' hc1lnm ')).toBe(true);
  });
});
