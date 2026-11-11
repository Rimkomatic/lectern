import { describe, expect, it } from 'vitest';
import { formatBytes, formatDuration, formatTime, formatWhen, percent } from './format';

describe('formatTime', () => {
  it('formats minutes and hours', () => {
    expect(formatTime(0)).toBe('0:00');
    expect(formatTime(5)).toBe('0:05');
    expect(formatTime(75)).toBe('1:15');
    expect(formatTime(3725)).toBe('1:02:05');
    expect(formatTime(59.9)).toBe('0:59');
  });
  it('is safe with bad input', () => {
    expect(formatTime(-3)).toBe('0:00');
    expect(formatTime(NaN)).toBe('0:00');
    expect(formatTime(Infinity)).toBe('0:00');
  });
});

describe('formatDuration', () => {
  it('is compact', () => {
    expect(formatDuration(30)).toBe('30s');
    expect(formatDuration(59)).toBe('59s');
    expect(formatDuration(90)).toBe('1m'); // minutes are floored, seconds dropped
    expect(formatDuration(5400)).toBe('1h 30m');
    expect(formatDuration(7200)).toBe('2h');
  });
  it('is empty when unknown', () => {
    expect(formatDuration(0)).toBe('');
    expect(formatDuration(null)).toBe('');
    expect(formatDuration(undefined)).toBe('');
    expect(formatDuration(NaN)).toBe('');
  });
});

describe('formatBytes', () => {
  it('scales units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1023)).toBe('1023 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB');
    expect(formatBytes(250 * 1024 * 1024)).toBe('250 MB');
  });
  it('rejects nonsense', () => {
    expect(formatBytes(-1)).toBe('');
    expect(formatBytes(NaN)).toBe('');
  });
});

describe('percent', () => {
  it('rounds and clamps', () => {
    expect(percent(1, 3)).toBe(33);
    expect(percent(2, 3)).toBe(67);
    expect(percent(5, 5)).toBe(100);
    expect(percent(9, 5)).toBe(100);
    expect(percent(0, 0)).toBe(0);
    expect(percent(-1, 5)).toBe(0);
  });
});

describe('formatWhen', () => {
  it('handles missing and invalid dates', () => {
    expect(formatWhen(null)).toBe('never');
    expect(formatWhen('garbage')).toBe('never');
    expect(formatWhen('2024-05-01T10:00:00Z')).not.toBe('never');
  });
});
