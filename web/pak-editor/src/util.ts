export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '?';
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

export function baseName(p: string): string {
  const parts = p.split(/[\/]/);
  return parts[parts.length - 1] || p;
}

export function formatTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

const PRIMARY = ['defaultengine.ini', 'defaultgame.ini'];

/** Returns the pak entries for DefaultEngine.ini and DefaultGame.ini (in that order) and the rest. */
export function splitConfigFiles(configFiles: string[]): { primary: string[]; others: string[] } {
  const primary: string[] = [];
  for (const want of PRIMARY) {
    const hit = configFiles.find((f) => baseName(f).toLowerCase() === want);
    if (hit) primary.push(hit);
  }
  return { primary, others: configFiles.filter((f) => !primary.includes(f)) };
}

/** Counts added/removed lines between two texts (LCS on the differing middle). */
export function lineDiffStats(a: string, b: string): { added: number; removed: number } {
  if (a === b) return { added: 0, removed: 0 };
  const la = a.split('\n');
  const lb = b.split('\n');
  let start = 0;
  while (start < la.length && start < lb.length && la[start] === lb[start]) start++;
  let endA = la.length;
  let endB = lb.length;
  while (endA > start && endB > start && la[endA - 1] === lb[endB - 1]) {
    endA--;
    endB--;
  }
  const A = la.slice(start, endA);
  const B = lb.slice(start, endB);
  const n = A.length;
  const m = B.length;
  if (n === 0 || m === 0 || n * m > 25_000_000) return { added: m, removed: n };
  let prev = new Uint32Array(m + 1);
  let cur = new Uint32Array(m + 1);
  for (let i = 1; i <= n; i++) {
    for (let j = 1; j <= m; j++) {
      cur[j] = A[i - 1] === B[j - 1] ? prev[j - 1] + 1 : Math.max(prev[j], cur[j - 1]);
    }
    [prev, cur] = [cur, prev];
  }
  const lcs = prev[m];
  return { added: m - lcs, removed: n - lcs };
}
