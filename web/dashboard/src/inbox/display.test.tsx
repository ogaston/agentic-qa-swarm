import { describe, expect, it } from 'vitest';
import { relativeTime } from './display';

describe('relative notification time', () => {
  const now = Date.parse('2026-10-28T12:00:00Z');
  it.each([[undefined, '—'], ['bad', '—'], ['2026-10-28T12:01:00Z', 'hace unos segundos'], ['2026-10-28T11:59:30Z', 'hace unos segundos'], ['2026-10-28T11:00:00Z', 'hace 1 hora'], ['2026-10-26T12:00:00Z', 'hace 2 días']])('formats %s', (value, expected) => {
    expect(relativeTime(value, now)).toBe(expected);
  });
});
