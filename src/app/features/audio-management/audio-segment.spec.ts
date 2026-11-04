import { audioSegmentBlob } from './audio-segment';

function wav(): ArrayBuffer {
  const buffer = new ArrayBuffer(16044);
  const view = new DataView(buffer);
  for (const [at, text] of [[0, 'RIFF'], [8, 'WAVE'], [12, 'fmt '], [36, 'data']] as const) {
    for (let i = 0; i < text.length; i++) { view.setUint8(at + i, text.charCodeAt(i)); }
  }
  view.setUint32(4, 16036, true); view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); view.setUint16(22, 1, true);
  view.setUint32(24, 8000, true); view.setUint32(28, 16000, true);
  view.setUint16(32, 2, true); view.setUint16(34, 16, true);
  view.setUint32(40, 16000, true);
  for (let i = 44; i < buffer.byteLength; i++) { view.setUint8(i, i % 256); }
  return buffer;
}

describe('Local WAV segment preview', () => {
  it('slices exact samples into a standalone WAV without changing the original', async () => {
    const original = wav();
    const copy = original.slice(0);
    const blob = audioSegmentBlob(original, 250, 750);
    expect(blob.size).toBe(8044);
    expect(blob.type).toBe('audio/wav');
    const result = await new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(blob);
    });
    expect(new Uint8Array(result, 44)).toEqual(new Uint8Array(original, 4044, 8000));
    expect(new DataView(result).getUint32(40, true)).toBe(8000);
    expect(new Uint8Array(original)).toEqual(new Uint8Array(copy));
  });

  it('rejects invalid bounds, damaged data and incompatible formats', () => {
    for (const [start, end] of [[-1, 200], [500, 500], [600, 500], [0, 1001], [0.5, 200]]) {
      expect(() => audioSegmentBlob(wav(), start, end)).toThrow();
    }
    expect(() => audioSegmentBlob(new ArrayBuffer(10), 0, 1)).toThrow();
    const incompatible = wav();
    new DataView(incompatible).setUint16(22, 2, true);
    expect(() => audioSegmentBlob(incompatible, 0, 100)).toThrow();
  });
});
