import { useCallback, useEffect, useMemo, useState } from 'react';
import { errorMessage, isApiError, type Issue, type JobResult, type JobStatus, type PakInfo } from '../api';
import { useApi } from '../apiContext';
import { useJob } from '../useJob';
import { baseName, formatBytes, lineDiffStats } from '../util';
import { DiffView } from './DiffView';
import { JobProgress } from './JobProgress';
import { Modal } from './Modal';

export interface FileChange {
  name: string;
  original: string;
  modified: string;
  sha1: string;
}

interface Props {
  pak: PakInfo;
  changes: FileChange[];
  onClose: () => void;
  onSaved: (result: JobResult) => void | Promise<void>;
  onReloadFromDisk: () => void;
  onJumpToIssue: (file: string, line: number) => void;
}

type Failure =
  | { kind: 'conflict'; message: string }
  | { kind: 'validation'; message: string; issues: Issue[] }
  | { kind: 'nochanges'; message: string }
  | { kind: 'other'; message: string };

type FileIssue = Issue & { file: string };

export function ReviewModal({ pak, changes: changesProp, onClose, onSaved, onReloadFromDisk, onJumpToIssue }: Props) {
  const api = useApi();
  const job = useJob();
  // Snapshot: the parent reloads its files after a successful save.
  const [changes] = useState(changesProp);
  const [issues, setIssues] = useState<Record<string, Issue[]> | null>(null);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [ackWarnings, setAckWarnings] = useState(false);
  const [failure, setFailure] = useState<Failure | null>(null);
  const [result, setResult] = useState<JobResult | null>(null);
  const [recompress, setRecompress] = useState(false);
  const canRecompress = pak.version >= 11 && (pak.compressionMethods ?? []).some((m) => m.toLowerCase() === 'oodle');

  const check = useCallback(async () => {
    setCheckError(null);
    setIssues(null);
    try {
      const res = await Promise.all(changes.map((c) => api.validate(c.modified)));
      const map: Record<string, Issue[]> = {};
      changes.forEach((c, i) => {
        map[c.name] = res[i].issues;
      });
      setIssues(map);
    } catch (e) {
      setCheckError(errorMessage(e));
    }
  }, [api, changes]);

  useEffect(() => {
    void check();
  }, [check]);

  const stats = useMemo(() => changes.map((c) => lineDiffStats(c.original, c.modified)), [changes]);
  const totals = stats.reduce(
    (a, s) => ({ added: a.added + s.added, removed: a.removed + s.removed }),
    { added: 0, removed: 0 },
  );

  const all: FileIssue[] = issues
    ? Object.entries(issues).flatMap(([file, list]) => list.map((i) => ({ file, ...i })))
    : [];
  const errors = all.filter((i) => i.severity === 'error');
  const warnings = all.filter((i) => i.severity === 'warning');
  const saving = job.running;
  const done = result !== null;

  const canSave =
    issues !== null && errors.length === 0 && (warnings.length === 0 || ackWarnings) && !saving && !done;

  async function save() {
    setFailure(null);
    const files: Record<string, string> = {};
    const expectSha1: Record<string, string> = {};
    for (const c of changes) {
      files[c.name] = c.modified;
      expectSha1[c.name] = c.sha1;
    }
    let final: JobStatus;
    try {
      final = await job.run(() =>
        api.save(recompress ? { pak: pak.path, files, expectSha1, recompress: true } : { pak: pak.path, files, expectSha1 }),
      );
    } catch (e) {
      if (isApiError(e) && e.status === 409) setFailure({ kind: 'conflict', message: e.message });
      else if (isApiError(e) && e.status === 422)
        setFailure({ kind: 'validation', message: e.message, issues: e.issues ?? [] });
      else if (isApiError(e) && e.status === 400) setFailure({ kind: 'nochanges', message: e.message });
      else setFailure({ kind: 'other', message: errorMessage(e) });
      return;
    }
    if (final.state === 'error' || !final.result) {
      setFailure({ kind: 'other', message: final.error || 'Save failed.' });
      return;
    }
    setResult(final.result);
    await onSaved(final.result);
  }

  const saveLabel = `Rebuild ${pak.name} (${formatBytes(pak.size)}) — a backup is kept`;

  const footer = done ? (
    <button type="button" className="btn btn-primary" onClick={onClose} data-autofocus>
      Done
    </button>
  ) : (
    <>
      <button type="button" className="btn" onClick={onClose} disabled={saving}>
        {saving ? 'Saving…' : 'Back to editor'}
      </button>
      <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={!canSave}>
        {saveLabel}
      </button>
    </>
  );

  return (
    <Modal title="Review changes" onClose={saving ? undefined : onClose} size="large" footer={footer}>
      <div className="review">
        <div className="review-summary">
          <strong>
            {changes.length} file{changes.length === 1 ? '' : 's'} changed
          </strong>
          <span className="add">+{totals.added}</span>
          <span className="del">−{totals.removed}</span>
          <span className="muted">lines</span>
        </div>

        {!done && (
          <div className="alert alert-info">
            Close the game (and any tool that has <span className="mono">{pak.name}</span> open) before saving — the
            pak file is replaced on disk.
          </div>
        )}

        {issues === null && !checkError && <p className="muted">Checking files…</p>}
        {checkError && (
          <div className="alert alert-error" role="alert">
            Could not validate the files: {checkError}{' '}
            <button type="button" className="link-btn" onClick={() => void check()}>
              Retry
            </button>
          </div>
        )}

        {errors.length > 0 && (
          <div className="alert alert-error" role="alert" data-testid="blocking-errors">
            <strong>
              Saving is blocked: {errors.length} error{errors.length === 1 ? '' : 's'} must be fixed first.
            </strong>
            <IssueList items={errors} onJump={onJumpToIssue} />
          </div>
        )}

        {errors.length === 0 && warnings.length > 0 && !done && (
          <div className="alert alert-warning">
            <strong>
              {warnings.length} warning{warnings.length === 1 ? '' : 's'}
            </strong>
            <IssueList items={warnings} onJump={onJumpToIssue} />
            <label className="checkbox">
              <input
                type="checkbox"
                checked={ackWarnings}
                onChange={(e) => setAckWarnings(e.target.checked)}
                disabled={saving}
              />
              I understand these warnings and want to save anyway
            </label>
          </div>
        )}

        {canRecompress && !done && (
          <label
            className="checkbox"
            title="Off (recommended): edited files are stored uncompressed. The game reads uncompressed entries normally and this is the safest option. On: edited files that were Oodle-compressed are compressed again with Oodle Kraken (needs the Oodle DLL; every block is checked by decompressing it). The pak stays slightly smaller."
          >
            <input
              type="checkbox"
              checked={recompress}
              onChange={(e) => setRecompress(e.target.checked)}
              disabled={saving}
            />
            Recompress edited files with Oodle
            <span className="muted small"> (off = stored uncompressed, safest)</span>
          </label>
        )}

        {failure && (
          <FailureBox failure={failure} onReload={onReloadFromDisk} onJump={onJumpToIssue} files={changes} />
        )}

        {(saving || (job.status !== null && failure === null)) && !done && <JobProgress status={job.status} />}

        {done && result && (
          <div className="alert alert-success" role="status">
            <strong>Saved. {pak.name} was rebuilt and verified.</strong>
            <div>
              Backup of the previous pak: <span className="mono select">{result.backup}</span>
            </div>
            {result.originalBackup && (
              <div>
                Original (never deleted): <span className="mono select">{result.originalBackup}</span>
              </div>
            )}
            <div>
              New size: {formatBytes(result.newSize)} · changed: {result.changed.map(baseName).join(', ') || '—'}
              {result.recompressed && result.recompressed.length > 0 && (
                <> · recompressed with Oodle: {result.recompressed.map(baseName).join(', ')}</>
              )}
            </div>
            <div className="small">The files were reloaded from the new pak.</div>
          </div>
        )}

        {!done &&
          changes.map((c, i) => (
            <section key={c.name} className="review-file">
              <h3>
                <span className="mono">{c.name}</span>
                <span className="add">+{stats[i].added}</span>
                <span className="del">−{stats[i].removed}</span>
              </h3>
              <DiffView original={c.original} modified={c.modified} />
            </section>
          ))}
      </div>
    </Modal>
  );
}

function IssueList({ items, onJump }: { items: FileIssue[]; onJump: (file: string, line: number) => void }) {
  return (
    <ul className="issue-list">
      {items.slice(0, 50).map((i, idx) => (
        <li key={idx}>
          <button type="button" className="link-btn" onClick={() => onJump(i.file, i.line)}>
            {i.file ? `${baseName(i.file)}:` : 'line '}
            {i.line}
          </button>{' '}
          {i.message}
        </li>
      ))}
      {items.length > 50 && <li className="muted">…and {items.length - 50} more</li>}
    </ul>
  );
}

interface FailureProps {
  failure: Failure;
  onReload: () => void;
  onJump: (file: string, line: number) => void;
  files: FileChange[];
}

function FailureBox({ failure, onReload, onJump, files }: FailureProps) {
  switch (failure.kind) {
    case 'conflict':
      return (
        <div className="alert alert-error" role="alert">
          <strong>The pak changed on disk since these files were loaded.</strong>
          <div>{failure.message}</div>
          <div>
            Another tool or a game update may have modified it. Reload to get the current contents — your edits in
            this session will be discarded.
          </div>
          <button type="button" className="btn btn-danger" onClick={onReload}>
            Reload from disk and discard my edits
          </button>
        </div>
      );
    case 'validation': {
      const file = files.length === 1 ? files[0].name : '';
      return (
        <div className="alert alert-error" role="alert">
          <strong>The server rejected the save: {failure.message}</strong>
          <IssueList items={failure.issues.map((i) => ({ ...i, file }))} onJump={onJump} />
        </div>
      );
    }
    case 'nochanges':
      return (
        <div className="alert alert-warning" role="alert">
          <strong>Nothing to save:</strong> {failure.message}
        </div>
      );
    default:
      return (
        <div className="alert alert-error" role="alert">
          <strong>Save failed:</strong> {failure.message}
          <div className="small">Your edits are still in the editor. You can retry.</div>
        </div>
      );
  }
}
