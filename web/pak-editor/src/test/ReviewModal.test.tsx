import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { PakApi, PakInfo } from '../api';
import { ApiContext } from '../apiContext';
import { ReviewModal } from '../components/ReviewModal';

// CodeMirror's merge view needs real layout; the diff itself is not under test here.
vi.mock('../components/DiffView', () => ({
  DiffView: () => <div data-testid="diff" />,
}));

const ENGINE = 'A2/Config/DefaultEngine.ini';

const pak: PakInfo = {
  path: 'C:/Game/Content/Paks/A2-Windows.pak',
  name: 'A2-Windows.pak',
  size: 221 * 1024 * 1024,
  version: 11,
  entries: 100,
  supported: true,
  configFiles: [ENGINE],
};

function fakeApi(overrides: Partial<PakApi>): PakApi {
  const fail = () => Promise.reject(new Error('not expected'));
  return {
    scan: fail,
    getFile: fail,
    validate: fail,
    save: fail,
    getJob: fail,
    listBackups: fail,
    restore: fail,
    getSettings: fail,
    saveSettings: fail,
    ...overrides,
  };
}

function renderModal(api: PakApi, onSaved = vi.fn()) {
  render(
    <ApiContext.Provider value={api}>
      <ReviewModal
        pak={pak}
        changes={[{ name: ENGINE, original: '[A]\nx=1\n', modified: '[A\nx=2\n', sha1: 'abc' }]}
        onClose={() => {}}
        onSaved={onSaved}
        onReloadFromDisk={() => {}}
        onJumpToIssue={() => {}}
      />
    </ApiContext.Provider>,
  );
  return onSaved;
}

const SAVE_LABEL = 'Rebuild A2-Windows.pak (221 MB) — a backup is kept';

describe('ReviewModal save flow', () => {
  it('blocks saving while validation errors exist', async () => {
    const save = vi.fn();
    const api = fakeApi({
      validate: async () => ({
        issues: [{ line: 1, severity: 'error', message: 'Section header is missing closing bracket' }],
        sections: [],
      }),
      save,
    });
    renderModal(api);

    expect(await screen.findByTestId('blocking-errors')).toHaveTextContent('Saving is blocked: 1 error');
    expect(screen.getByText(/missing closing bracket/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: SAVE_LABEL })).toBeDisabled();
    expect(screen.queryByLabelText(/I understand these warnings/)).not.toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
  });

  it('requires acknowledging warnings, then saves and shows the backup path', async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => ({ jobId: 'j1' }));
    const getJob = vi.fn(async () => ({
      state: 'done' as const,
      phase: 'swapping' as const,
      progress: 1,
      result: { backup: 'C:/b/A2-Windows.pak.bak', originalBackup: 'C:/b/orig.pak', newSize: 1, changed: [ENGINE] },
    }));
    const api = fakeApi({
      validate: async () => ({ issues: [{ line: 2, severity: 'warning', message: 'Odd value' }], sections: [] }),
      save,
      getJob,
    });
    const onSaved = renderModal(api);

    await screen.findByText('Odd value');
    const btn = screen.getByRole('button', { name: SAVE_LABEL });
    expect(btn).toBeDisabled();
    await user.click(screen.getByLabelText(/I understand these warnings/));
    expect(btn).toBeEnabled();
    await user.click(btn);

    await waitFor(() => expect(screen.getByText('C:/b/A2-Windows.pak.bak')).toBeInTheDocument());
    expect(save).toHaveBeenCalledWith({ pak: pak.path, files: { [ENGINE]: '[A\nx=2\n' }, expectSha1: { [ENGINE]: 'abc' } });
    expect(onSaved).toHaveBeenCalledTimes(1);
  });
});
