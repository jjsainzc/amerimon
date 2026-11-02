import { Injectable } from '@angular/core';
import { PasswordMetrics, PasswordStatus, ComplexityLevel } from '../models/password-meter.model';

@Injectable({
  providedIn: 'root'
})
export class PasswordEvaluatorService {

  public calculateShannonEntropy(password: string): number {
    const characters = Array.from(password);
    if (characters.length === 0) {
      return 0;
    }

    const frequencies = new Map<string, number>();
    for (const character of characters) {
      frequencies.set(character, (frequencies.get(character) ?? 0) + 1);
    }

    const length = characters.length;
    const entropyPerCharacter = [...frequencies.values()].reduce((entropy, frequency) => {
      const probability = frequency / length;
      return entropy - probability * Math.log2(probability);
    }, 0);

    return entropyPerCharacter * length;
  }

  private strReverse(str: string): string {
    let newstring = '';
    for (let s = 0; s < str.length; s++) {
      newstring = str.charAt(s) + newstring;
    }
    return newstring;
  }

  public evaluate(pwd: string): PasswordMetrics {
    let nScore = 0;
    let nLength = 0;
    let nAlphaUC = 0;
    let nAlphaLC = 0;
    let nNumber = 0;
    let nSymbol = 0;
    let nMidChar = 0;
    let nRequirements = 0;
    let nAlphasOnly = 0;
    let nNumbersOnly = 0;
    let nRepChar = 0;
    let nConsecAlphaUC = 0;
    let nConsecAlphaLC = 0;
    let nConsecNumber = 0;
    let nConsecSymbol = 0;
    let nConsecCharType = 0;
    let nSeqAlpha = 0;
    let nSeqNumber = 0;
    let nReqChar = 0;

    const nMinPwdLen = 8;
    const nMultLength = 4;
    const nMultNumber = 4;
    const nMultSymbol = 6;
    const nMultMidChar = 2;
    const nMultRepChar = 1;
    const nMultConsecAlphaUC = 2;
    const nMultConsecAlphaLC = 2;
    const nMultConsecNumber = 2;
    const nMultSeqAlpha = 3;
    const nMultSeqNumber = 3;

    let nTmpAlphaUC: any = '';
    let nTmpAlphaLC: any = '';
    let nTmpNumber: any = '';
    let nTmpSymbol: any = '';

    let sAlphaUC = '    0';
    let sAlphaLC = '    0';
    let sNumber = '    0';
    let sSymbol = '    0';
    let sMidChar = '    0';
    let sRequirements = '    0';
    let sAlphasOnly = '    0';
    let sNumbersOnly = '    0';
    let sRepChar = '    0';
    let sConsecAlphaUC = '    0';
    let sConsecAlphaLC = '    0';
    let sConsecNumber = '    0';
    let sSeqAlpha = '    0';
    let sSeqNumber = '    0';

    let sComplexity: ComplexityLevel = 'Too Short';

    const sAlphas = 'abcdefghijklmnopqrstuvwxyz';
    const sNumerics = '01234567890';

    if (pwd) {
      nScore = parseInt(String(pwd.length * nMultLength), 10);
      nLength = pwd.length;
      const arrPwd = pwd.replace(/\s+/g, '').split(/\s*/);
      const arrPwdLen = arrPwd.length;

      // Loop principal para patrones
      for (let a = 0; a < arrPwdLen; a++) {
        if (arrPwd[a].match(new RegExp(/[A-Z]/g))) {
          if (nTmpAlphaUC !== '') { if ((nTmpAlphaUC + 1) === a) { nConsecAlphaUC++; nConsecCharType++; } }
          nTmpAlphaUC = a;
          nAlphaUC++;
        } else if (arrPwd[a].match(new RegExp(/[a-z]/g))) {
          if (nTmpAlphaLC !== '') { if ((nTmpAlphaLC + 1) === a) { nConsecAlphaLC++; nConsecCharType++; } }
          nTmpAlphaLC = a;
          nAlphaLC++;
        } else if (arrPwd[a].match(new RegExp(/[0-9]/g))) {
          if (a > 0 && a < (arrPwdLen - 1)) { nMidChar++; }
          if (nTmpNumber !== '') { if ((nTmpNumber + 1) === a) { nConsecNumber++; nConsecCharType++; } }
          nTmpNumber = a;
          nNumber++;
        } else if (arrPwd[a].match(new RegExp(/[^a-zA-Z0-9_]/g))) {
          if (a > 0 && a < (arrPwdLen - 1)) { nMidChar++; }
          if (nTmpSymbol !== '') { if ((nTmpSymbol + 1) === a) { nConsecSymbol++; nConsecCharType++; } }
          nTmpSymbol = a;
          nSymbol++;
        }

        // Loop interno para caracteres repetidos
        for (let b = 0; b < arrPwdLen; b++) {
          if (arrPwd[a].toLowerCase() === arrPwd[b].toLowerCase() && a !== b) { nRepChar++; }
        }
      }

      // Secuencias alfabéticas
      for (let s = 0; s < 23; s++) {
        const sFwd = sAlphas.substring(s, parseInt(String(s + 3), 10));
        const sRev = this.strReverse(sFwd);
        if (pwd.toLowerCase().indexOf(sFwd) !== -1 || pwd.toLowerCase().indexOf(sRev) !== -1) {
          nSeqAlpha++;
        }
      }

      // Secuencias numéricas
      for (let s = 0; s < 8; s++) {
        const sFwd = sNumerics.substring(s, parseInt(String(s + 3), 10));
        const sRev = this.strReverse(sFwd);
        if (pwd.toLowerCase().indexOf(sFwd) !== -1 || pwd.toLowerCase().indexOf(sRev) !== -1) {
          nSeqNumber++;
        }
      }

      // Bonificaciones generales
      if (nAlphaUC > 0 && nAlphaUC < nLength) {
        nScore = parseInt(String(nScore + ((nLength - nAlphaUC) * 2)), 10);
        sAlphaUC = '+ ' + parseInt(String((nLength - nAlphaUC) * 2), 10);
      }
      if (nAlphaLC > 0 && nAlphaLC < nLength) {
        nScore = parseInt(String(nScore + ((nLength - nAlphaLC) * 2)), 10);
        sAlphaLC = '+ ' + parseInt(String((nLength - nAlphaLC) * 2), 10);
      }
      if (nNumber > 0 && nNumber < nLength) {
        nScore = parseInt(String(nScore + (nNumber * nMultNumber)), 10);
        sNumber = '+ ' + parseInt(String(nNumber * nMultNumber), 10);
      }
      if (nSymbol > 0) {
        nScore = parseInt(String(nScore + (nSymbol * nMultSymbol)), 10);
        sSymbol = '+ ' + parseInt(String(nSymbol * nMultSymbol), 10);
      }
      if (nMidChar > 0) {
        nScore = parseInt(String(nScore + (nMidChar * nMultMidChar)), 10);
        sMidChar = '+ ' + parseInt(String(nMidChar * nMultMidChar), 10);
      }

      // Deducciones por malas prácticas
      if ((nAlphaLC > 0 || nAlphaUC > 0) && nSymbol === 0 && nNumber === 0) {
        nScore = parseInt(String(nScore - nLength), 10);
        nAlphasOnly = nLength;
        sAlphasOnly = '- ' + nLength;
      }
      if (nAlphaLC === 0 && nAlphaUC === 0 && nSymbol === 0 && nNumber > 0) {
        nScore = parseInt(String(nScore - nLength), 10);
        nNumbersOnly = nLength;
        sNumbersOnly = '- ' + nLength;
      }
      if (nRepChar > 0) {
        nScore = parseInt(String(nScore - (nRepChar * nRepChar)), 10);
        sRepChar = '- ' + nRepChar;
      }
      if (nConsecAlphaUC > 0) {
        nScore = parseInt(String(nScore - (nConsecAlphaUC * nMultConsecAlphaUC)), 10);
        sConsecAlphaUC = '- ' + parseInt(String(nConsecAlphaUC * nMultConsecAlphaUC), 10);
      }
      if (nConsecAlphaLC > 0) {
        nScore = parseInt(String(nScore - (nConsecAlphaLC * nMultConsecAlphaLC)), 10);
        sConsecAlphaLC = '- ' + parseInt(String(nConsecAlphaLC * nMultConsecAlphaLC), 10);
      }
      if (nConsecNumber > 0) {
        nScore = parseInt(String(nScore - (nConsecNumber * nMultConsecNumber)), 10);
        sConsecNumber = '- ' + parseInt(String(nConsecNumber * nMultConsecNumber), 10);
      }
      if (nSeqAlpha > 0) {
        nScore = parseInt(String(nScore - (nSeqAlpha * nMultSeqAlpha)), 10);
        sSeqAlpha = '- ' + parseInt(String(nSeqAlpha * nMultSeqAlpha), 10);
      }
      if (nSeqNumber > 0) {
        nScore = parseInt(String(nScore - (nSeqNumber * nMultSeqNumber)), 10);
        sSeqNumber = '- ' + parseInt(String(nSeqNumber * nMultSeqNumber), 10);
      }

      // Requerimientos obligatorios
      const arrChars = [nLength, nAlphaUC, nAlphaLC, nNumber, nSymbol];
      const arrCharsIds = ['nLength', 'nAlphaUC', 'nAlphaLC', 'nNumber', 'nSymbol'];
      const statuses: { [key: string]: PasswordStatus } = {};

      for (let c = 0; c < arrChars.length; c++) {
        const minVal = (arrCharsIds[c] === 'nLength') ? parseInt(String(nMinPwdLen - 1), 10) : 0;
        if (arrChars[c] === parseInt(String(minVal + 1), 10)) {
          nReqChar++;
          statuses[arrCharsIds[c]] = 'pass';
        } else if (arrChars[c] > parseInt(String(minVal + 1), 10)) {
          nReqChar++;
          statuses[arrCharsIds[c]] = 'exceed';
        } else {
          statuses[arrCharsIds[c]] = 'fail';
        }
      }

      nRequirements = nReqChar;
      const nMinReqChars = (pwd.length >= nMinPwdLen) ? 3 : 4;
      if (nRequirements > nMinReqChars) {
        nScore = parseInt(String(nScore + (nRequirements * 2)), 10);
        sRequirements = '+ ' + parseInt(String(nRequirements * 2), 10);
      }

      // Bonificaciones adicionales
      statuses['nMidChar'] = (nMidChar === 1) ? 'pass' : (nMidChar > 1 ? 'exceed' : 'fail');
      statuses['nRequirements'] = (nRequirements === (nMinReqChars + 1)) ? 'pass' : (nRequirements > (nMinReqChars + 1) ? 'exceed' : 'fail');

      // Evaluaciones sugeridas / advertencias
      const warnChars = [nAlphasOnly, nNumbersOnly, nRepChar, nConsecAlphaUC, nConsecAlphaLC, nConsecNumber, nSeqAlpha, nSeqNumber];
      const warnIds = ['nAlphasOnly', 'nNumbersOnly', 'nRepChar', 'nConsecAlphaUC', 'nConsecAlphaLC', 'nConsecNumber', 'nSeqAlpha', 'nSeqNumber'];

      for (let c = 0; c < warnChars.length; c++) {
        statuses[warnIds[c]] = (warnChars[c] > 0) ? 'warn' : 'pass';
      }

      // Rango de puntaje final (0 - 100)
      if (nScore > 100) { nScore = 100; } else if (nScore < 0) { nScore = 0; }

      if (nScore >= 0 && nScore < 20) { sComplexity = 'Very Weak'; }
      else if (nScore >= 20 && nScore < 40) { sComplexity = 'Weak'; }
      else if (nScore >= 40 && nScore < 60) { sComplexity = 'Good'; }
      else if (nScore >= 60 && nScore < 80) { sComplexity = 'Strong'; }
      else if (nScore >= 80 && nScore <= 100) { sComplexity = 'Very Strong'; }

      const shannonEntropyBits = this.calculateShannonEntropy(pwd);

      return {
        score: nScore,
        complexity: sComplexity,
        shannonEntropyBits,
        shannonEntropyPerCharacter: shannonEntropyBits / Array.from(pwd).length,
        details: {
          length: nLength,
          alphaUC: nAlphaUC,
          alphaLC: nAlphaLC,
          number: nNumber,
          symbol: nSymbol,
          midChar: nMidChar,
          requirements: nRequirements,
          alphasOnly: nAlphasOnly,
          numbersOnly: nNumbersOnly,
          repChar: nRepChar,
          consecAlphaUC: nConsecAlphaUC,
          consecAlphaLC: nConsecAlphaLC,
          consecNumber: nConsecNumber,
          seqAlpha: nSeqAlpha,
          seqNumber: nSeqNumber
        },
        bonuses: {
          length: '+ ' + nScore,
          alphaUC: sAlphaUC,
          alphaLC: sAlphaLC,
          number: sNumber,
          symbol: sSymbol,
          midChar: sMidChar,
          requirements: sRequirements,
          alphasOnly: sAlphasOnly,
          numbersOnly: sNumbersOnly,
          repChar: sRepChar,
          consecAlphaUC: sConsecAlphaUC,
          consecAlphaLC: sConsecAlphaLC,
          consecNumber: sConsecNumber,
          seqAlpha: sSeqAlpha,
          seqNumber: sSeqNumber
        },
        statuses
      };

    } else {
      // Estado por defecto (contraseña vacía)
      return {
        score: 0,
        complexity: 'Too Short',
        shannonEntropyBits: 0,
        shannonEntropyPerCharacter: 0,
        details: {
          length: 0, alphaUC: 0, alphaLC: 0, number: 0, symbol: 0,
          midChar: 0, requirements: 0, alphasOnly: 0, numbersOnly: 0,
          repChar: 0, consecAlphaUC: 0, consecAlphaLC: 0, consecNumber: 0,
          seqAlpha: 0, seqNumber: 0
        },
        bonuses: {
          length: '    0', alphaUC: '    0',
          alphaLC: '    0', number: '    0',
          symbol: '    0', midChar: '    0',
          requirements: '    0', alphasOnly: '    0',
          numbersOnly: '    0', repChar: '    0',
          consecAlphaUC: '    0', consecAlphaLC: '    0',
          consecNumber: '    0', seqAlpha: '    0',
          seqNumber: '    0'
        },
        statuses: {}
      };
    }
  }
}
