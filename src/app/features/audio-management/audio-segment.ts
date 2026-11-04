export function audioSegmentBlob(buffer: ArrayBuffer, startMS: number, endMS: number): Blob {
  const view = new DataView(buffer);
  const tag = (offset: number) => String.fromCharCode(...new Uint8Array(buffer, offset, 4));
  if (buffer.byteLength < 44 || tag(0) !== 'RIFF' || tag(8) !== 'WAVE') {
    throw new Error('Invalid WAV');
  }
  const limit = view.getUint32(4, true) + 8;
  if (limit > buffer.byteLength) { throw new Error('Incomplete WAV'); }
  let validFormat = false;
  const chunks: Uint8Array[] = [];
  for (let offset = 12; offset + 8 <= limit;) {
    const size = view.getUint32(offset + 4, true);
    const start = offset + 8;
    if (start + size + size % 2 > limit) { throw new Error('Invalid WAV chunk'); }
    if (tag(offset) === 'fmt ') {
      validFormat = size >= 16 && view.getUint16(start, true) === 1 &&
        view.getUint16(start + 2, true) === 1 && view.getUint32(start + 4, true) === 8000 &&
        view.getUint32(start + 8, true) === 16000 && view.getUint16(start + 12, true) === 2 &&
        view.getUint16(start + 14, true) === 16;
    }
    if (tag(offset) === 'data') { chunks.push(new Uint8Array(buffer, start, size)); }
    offset = start + size + size % 2;
  }
  const size = chunks.reduce((total, chunk) => total + chunk.length, 0);
  if (!validFormat || !Number.isInteger(startMS) || !Number.isInteger(endMS) ||
      startMS < 0 || endMS <= startMS || endMS > Math.ceil(size / 16) || size % 2 !== 0) {
    throw new Error('Invalid WAV segment');
  }
  const start = startMS * 16;
  const end = Math.min(endMS * 16, size);
  if (start >= end) { throw new Error('Empty WAV segment'); }
  const pcm = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { pcm.set(chunk, offset); offset += chunk.length; }
  const header = new ArrayBuffer(44);
  const output = new DataView(header);
  const writeTag = (at: number, value: string) => {
    for (let i = 0; i < value.length; i++) { output.setUint8(at + i, value.charCodeAt(i)); }
  };
  writeTag(0, 'RIFF'); output.setUint32(4, 36 + end - start, true);
  writeTag(8, 'WAVE'); writeTag(12, 'fmt '); output.setUint32(16, 16, true);
  output.setUint16(20, 1, true); output.setUint16(22, 1, true);
  output.setUint32(24, 8000, true); output.setUint32(28, 16000, true);
  output.setUint16(32, 2, true); output.setUint16(34, 16, true);
  writeTag(36, 'data'); output.setUint32(40, end - start, true);
  return new Blob([header, pcm.slice(start, end)], { type: 'audio/wav' });
}
