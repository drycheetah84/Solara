import { beforeEach, describe, expect, it } from 'vitest';
import { getSessionToken, initSessionToken } from '../session';

describe('session token', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it('reads ?t= and strips it from the address bar, keeping other params', () => {
    window.history.replaceState(null, '', '/?t=abc123&debug=1#frag');
    const t = initSessionToken();
    expect(t).toBe('abc123');
    expect(getSessionToken()).toBe('abc123');
    expect(window.location.search).toBe('?debug=1');
    expect(window.location.hash).toBe('#frag');
    expect(window.location.href).not.toContain('abc123');
    expect(window.sessionStorage.getItem('pak-editor.session-token')).toBe('abc123');
  });

  it('falls back to sessionStorage after a reload', () => {
    window.history.replaceState(null, '', '/?t=first');
    initSessionToken();
    window.history.replaceState(null, '', '/');
    expect(initSessionToken()).toBe('first');
    expect(window.location.search).toBe('');
  });
});
