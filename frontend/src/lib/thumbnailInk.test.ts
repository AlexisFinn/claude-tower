import { describe, expect, it } from 'vitest';
import { DEFAULT_FG } from './termPalette';
import { strokeWidth, textColor } from './thumbnailInk';

describe('textColor', () => {
  it('lifts a dark color on the dark background', () => {
    const lifted = textColor('#30343c', null);
    expect(parseInt(lifted.slice(1, 3), 16)).toBeGreaterThan(0x30);
  });

  it('keeps a light color, and any color on a light background', () => {
    expect(textColor(DEFAULT_FG, null)).toBe(DEFAULT_FG);
    expect(textColor('#30343c', '#e7ecf3')).toBe('#30343c');
  });

  it('keeps a color it cannot read', () => {
    expect(textColor('red', null)).toBe('red');
  });
});

describe('strokeWidth', () => {
  it('grows with the font size', () => {
    expect(strokeWidth(12)).toBeGreaterThan(0);
    expect(strokeWidth(24)).toBeCloseTo(2 * strokeWidth(12));
  });
});
