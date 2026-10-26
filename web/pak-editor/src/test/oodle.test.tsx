import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../App';
import type { PakApi, PakInfo } from '../api';
import { ApiContext } from '../apiContext';
import { createMockApi } from '../mockApi';
import { ReviewModal } from '../components/ReviewModal';

vi.mock('../components/DiffView', () => ({
  DiffView: () => <div data-testid="diff" />,
}));

describe('Oodle support', () => {
  it('shows version + compression, auto-opens the Oodle dialog on oodle_required and retries after setup', async () => {
    const user = userEvent.setup();
    const api = createMockApi();
    render(
      <ApiContext.Provider value={api}>
        <App hasToken mock />
      </ApiContext.Provider>,
    );
    await user.type(screen.getByLabelText('Game Content folder'), 'C:/Games/A2/Content');
    await user.click(screen.getByRole('button', { name: 'Scan' }));

    const quest = await screen.findByRole('button', { name: /A2-Android_ASTC\.pak/ });
    expect(quest).toHaveTextContent('v11');
    expect(quest).toHaveTextContent('Oodle');
    expect(quest).toHaveTextContent(/Oodle-compressed/);
    expect(screen.getByRole('button', { name: /A2-Windows\.pak/ })).toHaveTextContent('uncompressed');

    await user.click(quest);
    const dialog = await screen.findByRole('dialog', { name: 'Oodle settings' });
    expect(within(dialog).getByRole('alert')).toHaveTextContent('DefaultEngine.ini is Oodle-compressed');
    expect(await within(dialog).findByTestId('oodle-status')).toHaveTextContent('not available');

    const input = within(dialog).getByLabelText('Oodle DLL path');
    await user.clear(input);
    await user.type(input, 'C:/tools/notoodle.dll');
    await user.click(within(dialog).getByRole('button', { name: 'Validate and save' }));
    expect(await within(dialog).findByText(/not an Oodle DLL/)).toBeInTheDocument();

    await user.clear(input);
    await user.type(input, 'C:/UE/Engine/Binaries/ThirdParty/Oodle/Win64/oo2core_9_win64.dll');
    await user.click(within(dialog).getByRole('button', { name: 'Validate and save' }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Oodle settings' })).not.toBeInTheDocument());
    expect(await screen.findByRole('tab', { name: /DefaultEngine\.ini/ })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText(/Stored Oodle-compressed/)).toBeInTheDocument();
    expect((await api.getSettings()).oodleAvailable).toBe(true);
  });

  it('opens the Oodle dialog from the header', async () => {
    const user = userEvent.setup();
    render(
      <ApiContext.Provider value={createMockApi()}>
        <App hasToken mock />
      </ApiContext.Provider>,
    );
    await user.click(screen.getByRole('button', { name: 'Oodle settings' }));
    expect(await screen.findByRole('dialog', { name: 'Oodle settings' })).toBeInTheDocument();
  });

  it('offers recompression for v11 Oodle paks (default off) and sends it when checked', async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => ({ jobId: 'j' }));
    const fail = () => Promise.reject(new Error('not expected'));
    const api: PakApi = {
      scan: fail,
      getFile: fail,
      validate: async () => ({ issues: [], sections: [] }),
      save,
      getJob: async () => ({
        state: 'done',
        phase: 'swapping',
        progress: 1,
        result: { backup: 'b', originalBackup: 'o', newSize: 1, changed: ['A2/Config/DefaultGame.ini'], recompressed: ['A2/Config/DefaultGame.ini'] },
      }),
      listBackups: fail,
      restore: fail,
      getSettings: fail,
      saveSettings: fail,
    };
    const quest: PakInfo = {
      path: 'C:/q/A2-Android_ASTC.pak',
      name: 'A2-Android_ASTC.pak',
      size: 74_018_153,
      version: 11,
      entries: 2832,
      supported: true,
      configFiles: ['A2/Config/DefaultGame.ini'],
      compressionMethods: ['Oodle'],
      oodleRequired: true,
      oodleAvailable: true,
    };
    render(
      <ApiContext.Provider value={api}>
        <ReviewModal
          pak={quest}
          changes={[{ name: 'A2/Config/DefaultGame.ini', original: '[A]\n', modified: '[A]\nx=1\n', sha1: 's' }]}
          onClose={() => {}}
          onSaved={() => {}}
          onReloadFromDisk={() => {}}
          onJumpToIssue={() => {}}
        />
      </ApiContext.Provider>,
    );
    const box = screen.getByLabelText(/Recompress edited files with Oodle/);
    expect(box).not.toBeChecked();
    expect(box.closest('label')).toHaveAttribute('title', expect.stringMatching(/uncompressed.*safest/));
    await user.click(box);
    const btn = await screen.findByRole('button', { name: /Rebuild A2-Android_ASTC\.pak/ });
    await waitFor(() => expect(btn).toBeEnabled());
    await user.click(btn);
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({
        pak: quest.path,
        files: { 'A2/Config/DefaultGame.ini': '[A]\nx=1\n' },
        expectSha1: { 'A2/Config/DefaultGame.ini': 's' },
        recompress: true,
      }),
    );
    expect(await screen.findByText(/recompressed with Oodle: DefaultGame\.ini/)).toBeInTheDocument();
  });

  it('hides the recompress option for v8 paks', async () => {
    const api = { validate: async () => ({ issues: [], sections: [] }) } as unknown as PakApi;
    render(
      <ApiContext.Provider value={api}>
        <ReviewModal
          pak={{ path: 'p', name: 'A2-Windows.pak', size: 1, version: 8, entries: 1, supported: true, configFiles: [], compressionMethods: [] }}
          changes={[{ name: 'A/DefaultGame.ini', original: 'a', modified: 'b', sha1: 's' }]}
          onClose={() => {}}
          onSaved={() => {}}
          onReloadFromDisk={() => {}}
          onJumpToIssue={() => {}}
        />
      </ApiContext.Provider>,
    );
    await waitFor(() => expect(screen.getByRole('button', { name: /Rebuild A2-Windows.pak/ })).toBeEnabled());
    expect(screen.queryByLabelText(/Recompress edited files/)).not.toBeInTheDocument();
  });
});
