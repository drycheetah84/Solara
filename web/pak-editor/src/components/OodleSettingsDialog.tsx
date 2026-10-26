import { useEffect, useState, type FormEvent } from 'react';
import { errorMessage, type SettingsResponse } from '../api';
import { useApi } from '../apiContext';
import { Modal } from './Modal';

interface Props {
  /** Why the dialog was opened automatically (e.g. a compressed file failed to load). */
  reason?: string | null;
  onClose: () => void;
  /** Called after the settings were validated and saved. */
  onSaved: (s: SettingsResponse) => void;
}

const SOURCE_LABEL: Record<string, string> = {
  settings: 'saved setting',
  env: 'PAK_EDITOR_OODLE_DLL environment variable',
  'exe-dir': 'found next to pak-editor.exe',
};

/** Lets the user point pak-editor at an Oodle DLL (needed for Oodle-compressed paks such as Quest builds). */
export function OodleSettingsDialog({ reason, onClose, onSaved }: Props) {
  const api = useApi();
  const [current, setCurrent] = useState<SettingsResponse | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .getSettings()
      .then((s) => {
        if (cancelled) return;
        setCurrent(s);
        setPath(s.oodleDll || s.oodlePath || '');
      })
      .catch((e: unknown) => {
        if (!cancelled) setLoadError(errorMessage(e));
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  async function apply(dll: string) {
    setBusy(true);
    setError(null);
    try {
      const s = await api.saveSettings({ oodleDll: dll });
      setCurrent(s);
      onSaved(s);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const p = path.trim().replace(/^"(.*)"$/, '$1');
    if (!p) {
      setError('Enter the full path of the Oodle DLL.');
      return;
    }
    void apply(p);
  };

  const footer = (
    <>
      <button type="button" className="btn" onClick={onClose} disabled={busy}>
        Close
      </button>
      <button type="submit" form="oodle-form" className="btn btn-primary" disabled={busy}>
        {busy ? 'Checking…' : 'Validate and save'}
      </button>
    </>
  );

  return (
    <Modal title="Oodle settings" onClose={busy ? undefined : onClose} footer={footer}>
      <div className="oodle-settings">
        {reason && (
          <div className="alert alert-warning" role="alert">
            {reason}
          </div>
        )}
        <p>
          Quest/Android builds store their config files compressed with <strong>Oodle</strong>. pak-editor needs the
          Oodle DLL to read them; it is not shipped with this tool. Use <span className="mono">oo2core_9_win64.dll</span>{' '}
          (or Unreal&apos;s <span className="mono">oodle-data-shared.dll</span>) from an Unreal Engine install or a game
          that ships it.
        </p>
        {loadError && (
          <div className="alert alert-error" role="alert">
            Could not read the current settings: {loadError}
          </div>
        )}
        {current && (
          <p className="small" data-testid="oodle-status">
            {current.oodleAvailable ? (
              <>
                Status: <strong>available</strong> — <span className="mono select">{current.oodlePath}</span> (
                {SOURCE_LABEL[current.oodleSource] ?? current.oodleSource})
                {!current.oodleCanCompress && ' · this DLL can only decompress (recompressing is unavailable)'}
              </>
            ) : (
              <>
                Status: <strong>not available</strong>
                {current.oodleError && <> — {current.oodleError}</>}
                {!current.oodleSupported && ' — Oodle is only supported on Windows'}
              </>
            )}
          </p>
        )}
        <form id="oodle-form" onSubmit={onSubmit}>
          <label htmlFor="oodle-dll">Oodle DLL path</label>
          <input
            id="oodle-dll"
            type="text"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="C:\Program Files\Epic Games\UE_5.x\Engine\Binaries\ThirdParty\Oodle\Win64\oo2core_9_win64.dll"
            spellCheck={false}
            autoComplete="off"
            data-autofocus
          />
        </form>
        {error && (
          <div className="alert alert-error" role="alert">
            {error}
          </div>
        )}
        {current?.oodleDll && (
          <button type="button" className="link-btn small" onClick={() => void apply('')} disabled={busy}>
            Forget the saved path (auto-detect from PAK_EDITOR_OODLE_DLL or next to the exe)
          </button>
        )}
        <p className="small muted">
          The path is checked (the DLL must export OodleLZ_Decompress) and saved in your user settings folder.
        </p>
      </div>
    </Modal>
  );
}
