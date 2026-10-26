import { useCallback, useEffect, useState } from 'react';
import { errorMessage, type BackupInfo, type JobResult, type PakInfo } from '../api';
import { useApi } from '../apiContext';
import { useJob } from '../useJob';
import { formatBytes, formatTime } from '../util';
import { ConfirmDialog } from './ConfirmDialog';
import { JobProgress } from './JobProgress';
import { Modal } from './Modal';

interface Props {
  pak: PakInfo;
  hasUnsavedChanges: boolean;
  onClose: () => void;
  onRestored: (result: JobResult | undefined) => void | Promise<void>;
}

export function BackupsDrawer({ pak, hasUnsavedChanges, onClose, onRestored }: Props) {
  const api = useApi();
  const job = useJob();
  const [backups, setBackups] = useState<BackupInfo[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<BackupInfo | null>(null);
  const [restoring, setRestoring] = useState<BackupInfo | null>(null);
  const [restoreError, setRestoreError] = useState<string | null>(null);
  const [restored, setRestored] = useState<BackupInfo | null>(null);

  const load = useCallback(async () => {
    setLoadError(null);
    try {
      const res = await api.listBackups(pak.path);
      const sorted = [...res.backups].sort((a, b) => {
        if (a.kind !== b.kind) return a.kind === 'original' ? -1 : 1;
        return b.modTime.localeCompare(a.modTime);
      });
      setBackups(sorted);
    } catch (e) {
      setLoadError(errorMessage(e));
    }
  }, [api, pak.path]);

  useEffect(() => {
    void load();
  }, [load]);

  async function restore(b: BackupInfo) {
    setConfirm(null);
    setRestoring(b);
    setRestoreError(null);
    setRestored(null);
    try {
      const final = await job.run(() => api.restore({ pak: pak.path, backup: b.path }));
      if (final.state === 'error') {
        setRestoreError(final.error || 'Restore failed.');
        return;
      }
      setRestored(b);
      await onRestored(final.result);
      void load();
    } catch (e) {
      setRestoreError(errorMessage(e));
    }
  }

  const busy = job.running;

  return (
    <Modal title={`Backups — ${pak.name}`} onClose={busy ? undefined : onClose} size="drawer">
      <div className="backups">
        <p className="muted small">
          A timestamped backup is written before every save. The first backup of the untouched pak is kept as the
          original and is never deleted.
        </p>

        {loadError && (
          <div className="alert alert-error" role="alert">
            Could not list backups: {loadError}{' '}
            <button type="button" className="link-btn" onClick={() => void load()}>
              Retry
            </button>
          </div>
        )}
        {backups === null && !loadError && <p className="muted">Loading…</p>}
        {backups !== null && backups.length === 0 && <p className="muted">No backups yet — they appear after the first save.</p>}

        {restoring && (busy || job.status) && (
          <div className="restore-status">
            <div className="small">
              Restoring <span className="mono">{restoring.name}</span>
            </div>
            <JobProgress status={job.status} />
          </div>
        )}
        {restoreError && (
          <div className="alert alert-error" role="alert">
            <strong>Restore failed:</strong> {restoreError}
          </div>
        )}
        {restored && !busy && !restoreError && (
          <div className="alert alert-success" role="status">
            Restored <span className="mono">{restored.name}</span>. Files were reloaded.
          </div>
        )}

        {backups && backups.length > 0 && (
          <ul className="backup-list">
            {backups.map((b) => (
              <li key={b.path} className={`backup backup-${b.kind}`}>
                <div className="backup-main">
                  <div className="backup-name mono">{b.name}</div>
                  <div className="muted small">
                    {b.kind === 'original' ? (
                      <span className="badge badge-original">Original — never deleted</span>
                    ) : (
                      'Timestamped'
                    )}{' '}
                    · {formatTime(b.modTime)} · {formatBytes(b.size)}
                  </div>
                </div>
                <button type="button" className="btn btn-small" disabled={busy} onClick={() => setConfirm(b)}>
                  Restore…
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {confirm && (
        <ConfirmDialog
          title="Restore this backup?"
          confirmLabel={`Replace ${pak.name} with ${confirm.name}`}
          danger
          onCancel={() => setConfirm(null)}
          onConfirm={() => void restore(confirm)}
        >
          <p>
            <span className="mono">{pak.name}</span> will be replaced by the backup{' '}
            <span className="mono">{confirm.name}</span> ({formatTime(confirm.modTime)}).
          </p>
          <p>Close the game first.</p>
          {hasUnsavedChanges && (
            <p className="warn-text">You have unsaved edits in the editor. They will be discarded when files reload.</p>
          )}
        </ConfirmDialog>
      )}
    </Modal>
  );
}
