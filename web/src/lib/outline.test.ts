import { beforeEach, describe, expect, it } from 'vitest';
import { forgetAll, recall, remember } from './outline.svelte';

describe('outline memory', () => {
  beforeEach(forgetAll);

  it('remembers per course and forgets when another course is opened', () => {
    expect(recall(1)).toBeNull();
    remember(1, { expanded: [3, 4], scrollTop: 120 });
    expect(recall(1)).toMatchObject({ expanded: [3, 4], scrollTop: 120 });
    remember(1, { scrollTop: 300 });
    expect(recall(1)).toMatchObject({ expanded: [3, 4], scrollTop: 300 });
    expect(recall(2)).toBeNull();
    remember(2, { expanded: [9] });
    expect(recall(1)).toBeNull();
    expect(recall(2)).toMatchObject({ expanded: [9], scrollTop: 0 });
  });
});
