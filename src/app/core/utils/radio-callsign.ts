const radioCallsignPattern = /^(?:[A-Z]{1,2}|[A-Z]\d|\d[A-Z])\d[A-Z]{1,4}(?:[-/][A-Z0-9]{1,6})*$/i;

export function isRadioAmateurCallsign(value: string): boolean {
  return radioCallsignPattern.test(value.trim());
}
