import { useCallback, useEffect, useState } from 'react';
import type { PakInfo, ScanResponse } from './api';
import { onUnauthorized } from './apiInstance';
import { StartScreen } from './components/StartScreen';
import { EditorScreen } from './components/EditorScreen';
import { SessionExpired } from './components/SessionExpired';
import { ConfirmDialog } from './components/ConfirmDialog';
import { OodleSettingsDialog } from './components/OodleSettingsDialog';

interface Props {
  hasToken: boolean;
  mock: boolean;
}

export function App({ hasToken, mock }: Props) {
  const [expired, setExpired] = useState(false);
  const [scan, setScan] = useState<ScanResponse | null>(null);
  const [scanPath, setScanPath] = useState('');
  const [pak, setPak] = useState<PakInfo | null>(null);
  const [dirty, setDirty] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [oodleDialog, setOodleDialog] = useState<{ reason: string | null } | null>(null);
  /** Bumped when Oodle settings change so the editor can retry loading. */
  const [oodleGeneration, setOodleGeneration] = useState(0);

  useEffect(() => onUnauthorized(() => setExpired(true)), []);

  const leaveEditor = useCallback(() => {
    setConfirmLeave(false);
    setDirty(false);
    setPak(null);
  }, []);

  const openOodle = useCallback(() => setOodleDialog({ reason: null }), []);
  const oodleRequired = useCallback(
    (file: string) =>
      setOodleDialog({
        reason: `${file} is Oodle-compressed. Point pak-editor at an Oodle DLL to open it.`,
      }),
    [],
  );

  const requestChangeFolder = useCallback(() => {
    if (dirty) setConfirmLeave(true);
    else leaveEditor();
  }, [dirty, leaveEditor]);

  if (!hasToken) return <SessionExpired reason="missing" />;
  if (expired) return <SessionExpired reason="expired" />;

  return (
    <div className="app">
      {mock && (
        <div className="mock-banner" role="note">
          Mock mode — in-memory fake backend, nothing is written to disk
        </div>
      )}
      {pak && scan ? (
        <EditorScreen
          key={pak.path}
          pak={pak}
          root={scan.root}
          onChangeFolder={requestChangeFolder}
          onDirtyChange={setDirty}
          onPakUpdated={setPak}
          onOpenOodle={openOodle}
          onOodleRequired={oodleRequired}
          oodleGeneration={oodleGeneration}
        />
      ) : (
        <StartScreen
          initialPath={scanPath}
          initialResult={scan}
          onScanned={(path, res) => {
            setScanPath(path);
            setScan(res);
          }}
          onPick={setPak}
          onOpenOodle={openOodle}
        />
      )}
      {oodleDialog && (
        <OodleSettingsDialog
          reason={oodleDialog.reason}
          onClose={() => setOodleDialog(null)}
          onSaved={(s) => {
            setPak((p) => (p ? { ...p, oodleAvailable: s.oodleAvailable } : p));
            if (s.oodleAvailable) {
              setOodleDialog(null);
              setOodleGeneration((g) => g + 1);
            }
          }}
        />
      )}
      {confirmLeave && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          confirmLabel="Discard changes and change folder"
          danger
          onCancel={() => setConfirmLeave(false)}
          onConfirm={leaveEditor}
        >
          <p>You have modified files that have not been saved to the pak. Leaving now discards those edits.</p>
        </ConfirmDialog>
      )}
    </div>
  );
}
