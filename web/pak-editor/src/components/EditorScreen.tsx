import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { errorMessage, isOodleRequired, type FileResponse, type Issue, type JobResult, type PakInfo, type Section } from '../api';
import { useApi } from '../apiContext';
import { baseName, formatBytes, splitConfigFiles } from '../util';
import { BackupsDrawer } from './BackupsDrawer';
import { ConfirmDialog } from './ConfirmDialog';
import { IniEditor, type IniEditorHandle } from './IniEditor';
import { ProblemsPanel } from './ProblemsPanel';
import { ReviewModal, type FileChange } from './ReviewModal';
import { SectionNav } from './SectionNav';
import { compressionLabel } from './StartScreen';

interface Props {
  pak: PakInfo;
  root: string;
  onChangeFolder: () => void;
  onDirtyChange: (dirty: boolean) => void;
  onPakUpdated: (pak: PakInfo) => void;
  onOpenOodle?: () => void;
  /** Called when a file cannot be read because the Oodle DLL is missing. */
  onOodleRequired?: (file: string) => void;
  /** Changes when the Oodle settings change; a failed initial load is retried. */
  oodleGeneration?: number;
}

interface FileState {
  loaded: FileResponse;
  text: string;
  issues: Issue[];
  sections: Section[];
  /** Text the current issues/sections were computed for. */
  validatedText: string;
}

export function EditorScreen({
  pak,
  root,
  onChangeFolder,
  onDirtyChange,
  onPakUpdated,
  onOpenOodle,
  onOodleRequired,
  oodleGeneration = 0,
}: Props) {
  const api = useApi();
  const editor = useRef<IniEditorHandle>(null);
  const { primary, others } = useMemo(() => splitConfigFiles(pak.configFiles), [pak.configFiles]);

  const [files, setFiles] = useState<Record<string, FileState>>({});
  const [tabs, setTabs] = useState<string[]>([]);
  const [active, setActive] = useState<string>('');
  const [initError, setInitError] = useState<string | null>(null);
  const [openError, setOpenError] = useState<string | null>(null);
  const [opening, setOpening] = useState(false);
  const [allowOthers, setAllowOthers] = useState(false);
  const [validating, setValidating] = useState(false);
  const [validateError, setValidateError] = useState<string | null>(null);
  const [showReview, setShowReview] = useState(false);
  const [showBackups, setShowBackups] = useState(false);
  const [confirmRevert, setConfirmRevert] = useState<string | null>(null);
  const [pendingJump, setPendingJump] = useState<{ line: number; n: number } | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const loadedOk = useRef(false);
  const oodleRequiredRef = useRef(onOodleRequired);
  oodleRequiredRef.current = onOodleRequired;

  const fetchFile = useCallback(
    async (name: string): Promise<FileState> => {
      const loaded = await api.getFile(pak.path, name);
      return { loaded, text: loaded.text, issues: loaded.issues, sections: loaded.sections, validatedText: loaded.text };
    },
    [api, pak.path],
  );

  /** (Re)loads the given files from the pak, replacing editor contents. */
  const reloadFiles = useCallback(
    async (names: string[]) => {
      const loaded = await Promise.all(names.map((n) => fetchFile(n)));
      setFiles((prev) => {
        const next = { ...prev };
        names.forEach((n, i) => (next[n] = loaded[i]));
        return next;
      });
      names.forEach((n, i) => editor.current?.resetDoc(n, loaded[i].text));
    },
    [fetchFile],
  );

  // Initial load of DefaultEngine.ini / DefaultGame.ini (retried when the
  // Oodle settings change after a failed load).
  useEffect(() => {
    if (loadedOk.current) return;
    let cancelled = false;
    const initial = primary.length > 0 ? primary : others.slice(0, 1);
    if (initial.length === 0) {
      setInitError('This pak contains no ini config files.');
      return;
    }
    setInitError(null);
    (async () => {
      try {
        const loaded = await Promise.all(initial.map((n) => fetchFile(n)));
        if (cancelled) return;
        loadedOk.current = true;
        const map: Record<string, FileState> = {};
        initial.forEach((n, i) => (map[n] = loaded[i]));
        setFiles(map);
        setTabs(initial);
        setActive(initial[0]);
      } catch (e) {
        if (cancelled) return;
        setInitError(errorMessage(e));
        if (isOodleRequired(e)) oodleRequiredRef.current?.(baseName(initial[0]));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [fetchFile, primary, others, oodleGeneration]);

  const isPrimary = useCallback((n: string) => primary.includes(n), [primary]);
  const modified = tabs.filter((n) => files[n] && files[n].text !== files[n].loaded.text);
  const dirty = modified.length > 0;
  const current = files[active];
  const readOnly = !isPrimary(active) && !allowOthers;

  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange]);

  useEffect(() => {
    if (!dirty) return;
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', h);
    return () => window.removeEventListener('beforeunload', h);
  }, [dirty]);

  // Live validation of the active file (debounced).
  const activeText = current?.text;
  const activeValidated = current?.validatedText;
  useEffect(() => {
    if (activeText === undefined || activeText === activeValidated) {
      setValidating(false);
      return;
    }
    setValidating(true);
    const name = active;
    const text = activeText;
    let stale = false;
    const h = window.setTimeout(async () => {
      try {
        const r = await api.validate(text);
        if (stale) return;
        setValidateError(null);
        setFiles((prev) =>
          prev[name] && prev[name].text === text
            ? { ...prev, [name]: { ...prev[name], issues: r.issues, sections: r.sections, validatedText: text } }
            : prev,
        );
      } catch (e) {
        if (!stale) setValidateError(errorMessage(e));
      } finally {
        if (!stale) setValidating(false);
      }
    }, 400);
    return () => {
      stale = true;
      window.clearTimeout(h);
    };
  }, [api, active, activeText, activeValidated]);

  // Jump to a line once the requested tab is shown.
  useEffect(() => {
    if (pendingJump) {
      editor.current?.goToLine(pendingJump.line);
      setPendingJump(null);
    }
  }, [pendingJump, active]);

  const jump = useCallback((line: number) => setPendingJump({ line, n: Date.now() }), []);

  const changes: FileChange[] = modified.map((n) => ({
    name: n,
    original: files[n].loaded.text,
    modified: files[n].text,
    sha1: files[n].loaded.sha1,
  }));

  const openReview = useCallback(() => {
    if (!dirty) {
      setToast('No changes to save.');
      return;
    }
    setShowReview(true);
  }, [dirty]);

  // Ctrl+S / Cmd+S opens the review dialog.
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 's') {
        e.preventDefault();
        if (!showReview && !showBackups) openReview();
      }
    };
    window.addEventListener('keydown', h, true);
    return () => window.removeEventListener('keydown', h, true);
  }, [openReview, showReview, showBackups]);

  useEffect(() => {
    if (!toast) return;
    const h = window.setTimeout(() => setToast(null), 2500);
    return () => window.clearTimeout(h);
  }, [toast]);

  async function openOther(name: string) {
    if (!name) return;
    if (tabs.includes(name)) {
      setActive(name);
      return;
    }
    setOpening(true);
    setOpenError(null);
    try {
      const f = await fetchFile(name);
      setFiles((prev) => ({ ...prev, [name]: f }));
      setTabs((prev) => [...prev, name]);
      setActive(name);
    } catch (e) {
      setOpenError(`${baseName(name)}: ${errorMessage(e)}`);
      if (isOodleRequired(e)) onOodleRequired?.(baseName(name));
    } finally {
      setOpening(false);
    }
  }

  function closeTab(name: string) {
    const idx = tabs.indexOf(name);
    const next = tabs.filter((t) => t !== name);
    setTabs(next);
    setFiles((prev) => {
      const copy = { ...prev };
      delete copy[name];
      return copy;
    });
    editor.current?.resetDoc(name, '');
    if (active === name) setActive(next[Math.max(0, idx - 1)] ?? next[0] ?? '');
  }

  function revert(name: string) {
    setConfirmRevert(null);
    setFiles((prev) => (prev[name] ? { ...prev, [name]: { ...prev[name], text: prev[name].loaded.text } } : prev));
    const f = files[name];
    if (f) editor.current?.resetDoc(name, f.loaded.text);
  }

  const onEditorChange = useCallback((key: string, text: string) => {
    setFiles((prev) => (prev[key] ? { ...prev, [key]: { ...prev[key], text } } : prev));
  }, []);

  async function afterSave(result: JobResult) {
    onPakUpdated({ ...pak, size: result.newSize });
    try {
      await reloadFiles(tabs);
    } catch (e) {
      setToast(`Saved, but reloading failed: ${errorMessage(e)}`);
    }
  }

  async function afterRestore(result: JobResult | undefined) {
    if (result) onPakUpdated({ ...pak, size: result.newSize });
    try {
      await reloadFiles(tabs);
    } catch (e) {
      setToast(`Restored, but reloading failed: ${errorMessage(e)}`);
    }
  }

  async function reloadFromDisk() {
    setShowReview(false);
    try {
      await reloadFiles(tabs);
      setToast('Reloaded from disk.');
    } catch (e) {
      setToast(`Reload failed: ${errorMessage(e)}`);
    }
  }

  const unopenedOthers = others.filter((n) => !tabs.includes(n));

  return (
    <div className="editor-screen">
      <header className="topbar">
        <div className="topbar-title">
          <h1>{pak.name}</h1>
          <div className="muted small mono" title={pak.path}>
            {pak.path} · {formatBytes(pak.size)} · v{pak.version}
            {compressionLabel(pak) && ` · ${compressionLabel(pak)}`}
          </div>
        </div>
        <button type="button" className="link-btn" onClick={onChangeFolder} title={root}>
          Change folder
        </button>
        <div className="topbar-actions">
          {onOpenOodle && (
            <button type="button" className="btn" onClick={onOpenOodle} title="Oodle DLL settings (needed for Oodle-compressed paks)">
              Oodle
            </button>
          )}
          <button type="button" className="btn" onClick={() => setShowBackups(true)}>
            Backups
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={openReview}
            disabled={!dirty}
            title="Review and save (Ctrl+S)"
          >
            Review changes{dirty ? ` (${modified.length})` : ''}
          </button>
        </div>
      </header>

      {initError ? (
        <div className="alert alert-error center-msg" role="alert">
          Could not load config files: {initError}
        </div>
      ) : !current ? (
        <p className="muted center-msg">Loading config files…</p>
      ) : (
        <div className="workspace">
          <aside className="sidebar">
            <SectionNav key={active} sections={current.sections} onJump={jump} />
          </aside>
          <div className="main">
            <div className="tabbar">
              <div role="tablist" aria-label="Config files" className="tabs">
                {tabs.map((n) => {
                  const isMod = modified.includes(n);
                  return (
                    <div key={n} className={`tab ${n === active ? 'tab-active' : ''}`}>
                      <button
                        type="button"
                        role="tab"
                        aria-selected={n === active}
                        className="tab-btn"
                        onClick={() => setActive(n)}
                        title={n}
                      >
                        {baseName(n)}
                        {!isPrimary(n) && !allowOthers && <span className="tab-ro"> (read-only)</span>}
                        {isMod && (
                          <span className="tab-dot" aria-label="modified" title="Modified">
                            ●
                          </span>
                        )}
                      </button>
                      {!isPrimary(n) && (
                        <button
                          type="button"
                          className="tab-close"
                          aria-label={`Close ${baseName(n)}`}
                          disabled={isMod}
                          title={isMod ? 'Revert or save changes before closing' : 'Close'}
                          onClick={() => closeTab(n)}
                        >
                          ×
                        </button>
                      )}
                    </div>
                  );
                })}
              </div>
              <div className="tabbar-tools">
                {others.length > 0 && (
                  <>
                    <label className="visually-hidden" htmlFor="more-files">
                      More files
                    </label>
                    <select
                      id="more-files"
                      value=""
                      disabled={opening || unopenedOthers.length === 0}
                      onChange={(e) => void openOther(e.target.value)}
                    >
                      <option value="">
                        {opening ? 'Opening…' : `More files (${unopenedOthers.length})`}
                      </option>
                      {unopenedOthers.map((n) => (
                        <option key={n} value={n}>
                          {n}
                        </option>
                      ))}
                    </select>
                    <label className="checkbox small">
                      <input type="checkbox" checked={allowOthers} onChange={(e) => setAllowOthers(e.target.checked)} />
                      Allow editing other files
                    </label>
                  </>
                )}
                <button
                  type="button"
                  className="btn btn-small"
                  disabled={!modified.includes(active)}
                  onClick={() => setConfirmRevert(active)}
                >
                  Revert file
                </button>
              </div>
            </div>
            {openError && (
              <div className="alert alert-error slim" role="alert">
                Could not open {openError}
              </div>
            )}
            <div className="editor-area">
              <IniEditor
                ref={editor}
                docKey={active}
                initialText={current.text}
                readOnly={readOnly}
                issues={current.issues}
                onChange={onEditorChange}
              />
            </div>
            <div className="statusbar small">
              <span>{readOnly ? 'Read-only' : 'Editable'}</span>
              <span>{current.loaded.lineEnding}</span>
              <span>{current.loaded.bom ? 'UTF-8 BOM' : 'No BOM'}</span>
              <span>{formatBytes(current.loaded.size)}</span>
              {current.loaded.compression && <span>Stored {current.loaded.compression}-compressed</span>}
              <span className="mono" title={current.loaded.sha1}>
                sha1 {current.loaded.sha1.slice(0, 10)}
              </span>
              {validateError && <span className="warn-text">Live check unavailable: {validateError}</span>}
              <span className="spacer" />
              <span className="muted">Ctrl+F search · Ctrl+S review &amp; save</span>
            </div>
            <ProblemsPanel issues={current.issues} validating={validating} onJump={jump} />
          </div>
        </div>
      )}

      {toast && (
        <div className="toast" role="status">
          {toast}
        </div>
      )}

      {showReview && (
        <ReviewModal
          pak={pak}
          changes={changes}
          onClose={() => setShowReview(false)}
          onSaved={afterSave}
          onReloadFromDisk={() => void reloadFromDisk()}
          onJumpToIssue={(file, line) => {
            setShowReview(false);
            if (file && tabs.includes(file)) setActive(file);
            jump(line);
          }}
        />
      )}

      {showBackups && (
        <BackupsDrawer
          pak={pak}
          hasUnsavedChanges={dirty}
          onClose={() => setShowBackups(false)}
          onRestored={afterRestore}
        />
      )}

      {confirmRevert && (
        <ConfirmDialog
          title={`Revert ${baseName(confirmRevert)}?`}
          confirmLabel="Discard my edits to this file"
          danger
          onCancel={() => setConfirmRevert(null)}
          onConfirm={() => revert(confirmRevert)}
        >
          <p>All changes to this file since it was loaded will be discarded. This cannot be undone.</p>
        </ConfirmDialog>
      )}
    </div>
  );
}
