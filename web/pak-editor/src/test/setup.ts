import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// jsdom has no layout; CodeMirror measures text ranges.
const emptyRects = (): DOMRectList => {
  const list: DOMRect[] = [];
  return Object.assign(list, { item: (i: number) => list[i] ?? null }) as unknown as DOMRectList;
};
if (typeof Range !== 'undefined') {
  Range.prototype.getClientRects ??= emptyRects;
  Range.prototype.getBoundingClientRect ??= () => new DOMRect(0, 0, 0, 0);
}

afterEach(() => {
  cleanup();
});
