import { useState, type FormEvent } from 'react';
import { errorMessage, isApiError, type PakInfo, type ScanResponse } from '../api';
import { useApi } from '../apiContext';
import { forgetPath, loadRecentPaths, rememberPath } from '../recent';
import { formatBytes, splitConfigFiles } from '../util';

interface Props {
  initialPath: string;
  initialResult: ScanResponse | null;
  onScanned: (path: string, res: ScanResponse) => void;
  onPick: (pak: PakInfo) => void;
  onOpenOodle?: () => void;
}

function scanHint(e: unknown): string | null {
  if (!isApiError(e)) return null;
  if (e.kind === 'network') return 'pak-editor does not respond. Check that its console window is still open.';
  if (e.status === 404) return 'Point at the game\'s Content folder (the one containing a "Paks" folder), or the Paks folder itself.';
  if (e.status === 400) return 'Paste a full folder path, e.g. copied from the Explorer address bar.';
  return null;
}

/** "Oodle" / "uncompressed" label for a pak (empty when the backend did not report it). */
export function compressionLabel(p: PakInfo): string {
  if (p.compressionMethods === undefined) return '';
  return p.compressionMethods.length > 0 ? p.compressionMethods.join(', ') : 'uncompressed';
}

export function StartScreen({ initialPath, initialResult, onScanned, onPick, onOpenOodle }: Props) {
  const api = useApi();
  const [path, setPath] = useState(initialPath);
  const [recent, setRecent] = useState<string[]>(() => loadRecentPaths());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<ScanResponse | null>(initialResult);

  async function scan(p: string) {
    const trimmed = p.trim().replace(/^"(.*)"$/, '$1');
    if (!trimmed) {
      setError(new Error('Enter the path to the game Content folder.'));
      return;
    }
    setPath(trimmed);
    setBusy(true);
    setError(null);
    try {
      const res = await api.scan(trimmed);
      setResult(res);
      setRecent(rememberPath(trimmed));
      onScanned(trimmed, res);
    } catch (e) {
      setResult(null);
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    void scan(path);
  };

  const hint = error ? scanHint(error) : null;

  return (
    <main className="start">
      <div className="start-inner">
        <div className="start-header">
          <h1 className="app-title">A2 Pak Config Editor</h1>
          {onOpenOodle && (
            <button type="button" className="link-btn" onClick={onOpenOodle}>
              Oodle settings
            </button>
          )}
        </div>
        <p className="muted">
          Edit <code>DefaultEngine.ini</code> and <code>DefaultGame.ini</code> inside the game&apos;s pak. Every save
          keeps a backup and the original pak is never deleted.
        </p>

        <form className="scan-form" onSubmit={onSubmit}>
          <label htmlFor="content-path">Game Content folder</label>
          <div className="row">
            <input
              id="content-path"
              type="text"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="C:\...\1.0.30907\A2\Content"
              spellCheck={false}
              autoComplete="off"
              autoFocus
            />
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {busy ? 'Scanning…' : 'Scan'}
            </button>
          </div>
          {error !== null && (
            <div className="alert alert-error" role="alert">
              <strong>{errorMessage(error)}</strong>
              {hint && <div>{hint}</div>}
            </div>
          )}
        </form>

        {recent.length > 0 && (
          <section className="recent" aria-label="Recent folders">
            <h2 className="section-title">Recent</h2>
            <ul>
              {recent.map((p) => (
                <li key={p}>
                  <button type="button" className="link-btn mono" onClick={() => void scan(p)} disabled={busy}>
                    {p}
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    aria-label={`Remove ${p} from recent`}
                    onClick={() => setRecent(forgetPath(p))}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </section>
        )}

        {result && (
          <section className="pak-results" aria-label="Pak files">
            <h2 className="section-title">
              {result.paks.length} pak file{result.paks.length === 1 ? '' : 's'} in <span className="mono">{result.root}</span>
            </h2>
            {result.paks.length === 0 && <p className="muted">No .pak files were found in this folder.</p>}
            <ul className="pak-list">
              {result.paks.map((p) => {
                const { primary } = splitConfigFiles(p.configFiles);
                const reason = !p.supported ? p.error || 'This pak format is not supported' : null;
                const comp = compressionLabel(p);
                const needsOodle = p.supported && p.oodleRequired && !p.oodleAvailable;
                return (
                  <li key={p.path}>
                    <button
                      type="button"
                      className="pak-item"
                      disabled={!p.supported}
                      onClick={() => onPick(p)}
                      aria-describedby={reason ? `reason-${p.name}` : undefined}
                    >
                      <span className="pak-name">{p.name}</span>
                      <span className="pak-meta">
                        {formatBytes(p.size)} · v{p.version}
                        {comp && ` · ${comp}`} · {p.entries.toLocaleString()} entries
                        {p.supported && ` · ${p.configFiles.length} ini files`}
                      </span>
                      {reason ? (
                        <span id={`reason-${p.name}`} className="pak-reason">
                          Unsupported: {reason}
                        </span>
                      ) : primary.length === 0 ? (
                        <span className="pak-reason warn">No DefaultEngine.ini / DefaultGame.ini in this pak</span>
                      ) : (
                        <span className="pak-files">{primary.join(', ')}</span>
                      )}
                      {needsOodle && (
                        <span className="pak-reason warn">
                          Config files are Oodle-compressed: set up the Oodle DLL (Oodle settings) to open them
                        </span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          </section>
        )}
      </div>
    </main>
  );
}
