// Session token handling. The backend opens http://127.0.0.1:PORT/?t=TOKEN;
// we keep the token in memory + sessionStorage (so reload works) and strip it
// from the address bar so it does not linger in history or screenshots.

const STORAGE_KEY = 'pak-editor.session-token';
let token: string | null = null;

function readStorage(): string | null {
  try {
    return window.sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeStorage(v: string): void {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, v);
  } catch {
    /* storage unavailable: memory only */
  }
}

/** Reads ?t= from the URL (falling back to sessionStorage) and removes it from the address bar. */
export function initSessionToken(): string | null {
  const url = new URL(window.location.href);
  const fromUrl = url.searchParams.get('t');
  if (fromUrl) {
    token = fromUrl;
    writeStorage(fromUrl);
    url.searchParams.delete('t');
    const search = url.searchParams.toString();
    window.history.replaceState(window.history.state, '', url.pathname + (search ? `?${search}` : '') + url.hash);
  } else {
    token = readStorage();
  }
  return token;
}

export function getSessionToken(): string | null {
  return token;
}

/** Test helper. */
export function setSessionTokenForTests(v: string | null): void {
  token = v;
}
