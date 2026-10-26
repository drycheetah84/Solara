const KEY = 'pak-editor.recent-paths';
const MAX = 8;

export function loadRecentPaths(): string[] {
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return [];
    const v: unknown = JSON.parse(raw);
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string').slice(0, MAX) : [];
  } catch {
    return [];
  }
}

export function rememberPath(path: string): string[] {
  const next = [path, ...loadRecentPaths().filter((p) => p.toLowerCase() !== path.toLowerCase())].slice(0, MAX);
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* ignore */
  }
  return next;
}

export function forgetPath(path: string): string[] {
  const next = loadRecentPaths().filter((p) => p !== path);
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* ignore */
  }
  return next;
}
