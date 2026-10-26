// Typed client for the pak-editor backend (cmd/pak-editor). Mirrors the API
// contract exactly; every request carries the X-Session-Token header.

export type Severity = 'error' | 'warning';

export interface Issue {
  line: number;
  severity: Severity;
  message: string;
}

export interface Section {
  name: string;
  line: number;
}

export interface PakInfo {
  path: string;
  name: string;
  size: number;
  version: number;
  entries: number;
  supported: boolean;
  /** ini entries, DefaultEngine/DefaultGame first */
  configFiles: string[];
  error?: string;
  /** footer compression method names, e.g. ["Oodle"] (empty = none) */
  compressionMethods?: string[];
  /** true when a config file is Oodle-compressed */
  oodleRequired?: boolean;
  /** true when the backend has an Oodle DLL loaded */
  oodleAvailable?: boolean;
}

export interface ScanResponse {
  root: string;
  paks: PakInfo[];
}

export type LineEnding = 'CRLF' | 'LF';

export interface FileResponse {
  name: string;
  /** LF-normalised */
  text: string;
  lineEnding: LineEnding;
  bom: boolean;
  size: number;
  sha1: string;
  sections: Section[];
  issues: Issue[];
  /** compression method of the stored entry ("" = uncompressed) */
  compression?: string;
}

export interface ValidateResponse {
  issues: Issue[];
  sections: Section[];
}

export interface SaveRequest {
  pak: string;
  files: Record<string, string>;
  expectSha1: Record<string, string>;
  /** v11 only: store edited Oodle entries compressed again (default: uncompressed) */
  recompress?: boolean;
}

export interface JobStarted {
  jobId: string;
}

export type JobState = 'running' | 'done' | 'error';
export type JobPhase = 'writing' | 'verifying' | 'backing-up' | 'swapping';

export interface JobResult {
  backup: string;
  originalBackup: string;
  newSize: number;
  changed: string[];
  /** entries stored Oodle-compressed by this save */
  recompressed?: string[];
}

export interface JobStatus {
  state: JobState;
  phase: JobPhase;
  progress: number;
  error?: string;
  result?: JobResult;
}

export type BackupKind = 'original' | 'timestamped';

export interface BackupInfo {
  path: string;
  name: string;
  size: number;
  modTime: string;
  kind: BackupKind;
}

export interface BackupsResponse {
  backups: BackupInfo[];
}

export interface RestoreRequest {
  pak: string;
  backup: string;
}

export interface SettingsResponse {
  /** saved DLL path ("" = auto-detect) */
  oodleDll: string;
  oodleAvailable: boolean;
  /** DLL actually in use (from settings, env or next to the exe) */
  oodlePath: string;
  oodleSource: '' | 'settings' | 'env' | 'exe-dir';
  oodleCanCompress: boolean;
  oodleSupported: boolean;
  oodleError?: string;
}

export interface SaveSettingsRequest {
  oodleDll: string;
}

/** Error code sent when an Oodle-compressed entry is read without the DLL. */
export const OODLE_REQUIRED = 'oodle_required';

export interface PakApi {
  scan(path: string): Promise<ScanResponse>;
  getFile(pak: string, name: string): Promise<FileResponse>;
  validate(text: string): Promise<ValidateResponse>;
  save(req: SaveRequest): Promise<JobStarted>;
  getJob(id: string): Promise<JobStatus>;
  listBackups(pak: string): Promise<BackupsResponse>;
  restore(req: RestoreRequest): Promise<JobStarted>;
  getSettings(): Promise<SettingsResponse>;
  saveSettings(req: SaveSettingsRequest): Promise<SettingsResponse>;
}

/** Error raised for non-2xx responses (kind 'http') or transport failures (kind 'network'). */
export class ApiError extends Error {
  readonly status: number;
  readonly kind: 'http' | 'network';
  readonly issues?: Issue[];
  /** machine-readable error code from the backend, e.g. 'oodle_required' */
  readonly code?: string;

  constructor(message: string, status: number, kind: 'http' | 'network', issues?: Issue[], code?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.kind = kind;
    this.issues = issues;
    this.code = code;
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

export function isOodleRequired(e: unknown): boolean {
  return e instanceof ApiError && e.code === OODLE_REQUIRED;
}

export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}

export interface HttpApiOptions {
  getToken: () => string | null;
  baseUrl?: string;
  fetchImpl?: typeof fetch;
  onUnauthorized?: () => void;
}

interface ErrorBody {
  error?: unknown;
  issues?: unknown;
  code?: unknown;
}

function isIssueArray(v: unknown): v is Issue[] {
  return Array.isArray(v) && v.every((i) => typeof i === 'object' && i !== null && 'line' in i && 'message' in i);
}

export function createHttpApi(opts: HttpApiOptions): PakApi {
  const base = opts.baseUrl ?? '';

  async function request<T>(method: 'GET' | 'POST', path: string, body?: unknown): Promise<T> {
    const doFetch = opts.fetchImpl ?? fetch;
    const headers: Record<string, string> = { Accept: 'application/json' };
    const token = opts.getToken();
    if (token) headers['X-Session-Token'] = token;
    if (body !== undefined) headers['Content-Type'] = 'application/json';

    let res: Response;
    try {
      res = await doFetch(base + path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        cache: 'no-store',
      });
    } catch (e) {
      throw new ApiError(`Cannot reach pak-editor (${errorMessage(e)}). Is it still running?`, 0, 'network');
    }

    const raw = await res.text();
    let parsed: unknown = undefined;
    if (raw.length > 0) {
      try {
        parsed = JSON.parse(raw);
      } catch {
        parsed = undefined;
      }
    }

    if (!res.ok) {
      if (res.status === 401) opts.onUnauthorized?.();
      const eb = (parsed ?? {}) as ErrorBody;
      const msg = typeof eb.error === 'string' && eb.error ? eb.error : `${res.status} ${res.statusText || 'request failed'}`;
      throw new ApiError(
        msg,
        res.status,
        'http',
        isIssueArray(eb.issues) ? eb.issues : undefined,
        typeof eb.code === 'string' ? eb.code : undefined,
      );
    }
    if (parsed === undefined) {
      throw new ApiError('Invalid JSON response from pak-editor', res.status, 'http');
    }
    return parsed as T;
  }

  const q = encodeURIComponent;
  // Go encodes nil slices as null; normalise so the UI can rely on arrays.
  const arr = <T,>(v: T[] | null | undefined): T[] => v ?? [];
  return {
    scan: async (path) => {
      const r = await request<ScanResponse>('POST', '/api/scan', { path });
      return {
        ...r,
        paks: arr(r.paks).map((p) => ({
          ...p,
          configFiles: arr(p.configFiles),
          ...(p.compressionMethods !== undefined ? { compressionMethods: arr(p.compressionMethods) } : {}),
        })),
      };
    },
    getFile: async (pak, name) => {
      const r = await request<FileResponse>('GET', `/api/file?pak=${q(pak)}&name=${q(name)}`);
      return { ...r, sections: arr(r.sections), issues: arr(r.issues) };
    },
    validate: async (text) => {
      const r = await request<ValidateResponse>('POST', '/api/validate', { text });
      return { issues: arr(r.issues), sections: arr(r.sections) };
    },
    save: (req) => request<JobStarted>('POST', '/api/save', req),
    getJob: async (id) => {
      const r = await request<JobStatus>('GET', `/api/jobs/${q(id)}`);
      return r.result
        ? { ...r, result: { ...r.result, changed: arr(r.result.changed), recompressed: arr(r.result.recompressed) } }
        : r;
    },
    listBackups: async (pak) => {
      const r = await request<BackupsResponse>('GET', `/api/backups?pak=${q(pak)}`);
      return { backups: arr(r.backups) };
    },
    restore: (req) => request<JobStarted>('POST', '/api/restore', req),
    getSettings: () => request<SettingsResponse>('GET', '/api/settings'),
    saveSettings: (req) => request<SettingsResponse>('POST', '/api/settings', req),
  };
}

/** Polls a job until it finishes. Resolves with the final status (state done or error). */
export async function pollJob(
  api: PakApi,
  id: string,
  onUpdate: (s: JobStatus) => void,
  opts: { intervalMs?: number; signal?: AbortSignal; maxNetworkRetries?: number } = {},
): Promise<JobStatus> {
  const interval = opts.intervalMs ?? 300;
  const maxRetries = opts.maxNetworkRetries ?? 5;
  let failures = 0;
  for (;;) {
    if (opts.signal?.aborted) throw new ApiError('Cancelled', 0, 'network');
    let s: JobStatus;
    try {
      s = await api.getJob(id);
      failures = 0;
    } catch (e) {
      if (isApiError(e) && e.kind === 'network' && failures < maxRetries) {
        failures++;
        await new Promise((r) => setTimeout(r, interval * 2));
        continue;
      }
      throw e;
    }
    onUpdate(s);
    if (s.state !== 'running') return s;
    await new Promise((r) => setTimeout(r, interval));
  }
}
