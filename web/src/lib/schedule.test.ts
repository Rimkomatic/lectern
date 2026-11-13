import { describe, expect, it } from 'vitest';
import { everyText, scheduleText } from './schedule';

describe('everyText', () => {
  it('picks the largest unit that divides evenly', () => {
    expect(everyText(900)).toBe('15 minutes');
    expect(everyText(3600)).toBe('hour');
    expect(everyText(7200)).toBe('2 hours');
    expect(everyText(86400)).toBe('day');
    expect(everyText(172800)).toBe('2 days');
    expect(everyText(60)).toBe('minute');
    expect(everyText(90)).toBe('90 seconds');
    expect(everyText(1)).toBe('second');
  });
  it('never says zero', () => {
    expect(everyText(0)).toBe('second');
    expect(everyText(-5)).toBe('second');
  });
});

describe('scheduleText', () => {
  it('says nothing before the server has reported', () => {
    expect(scheduleText(undefined)).toBe('');
  });
  it('words the daily scan with its time and zone', () => {
    const t = scheduleText({ mode: 'daily', at: '03:00', zone: 'IST, UTC+05:30' });
    expect(t).toContain('every day at 03:00 (IST, UTC+05:30)');
    expect(t).toContain('Scan now');
  });
  it('works without a zone', () => {
    expect(scheduleText({ mode: 'daily', at: '04:15' })).toContain('every day at 04:15.');
  });
  it('words an interval', () => {
    expect(scheduleText({ mode: 'interval', every_seconds: 900 })).toBe('Scans by itself every 15 minutes.');
  });
  it('words "off"', () => {
    expect(scheduleText({ mode: 'off' })).toContain('Scan now');
    expect(scheduleText({ mode: 'off' })).toContain('off');
  });
});
