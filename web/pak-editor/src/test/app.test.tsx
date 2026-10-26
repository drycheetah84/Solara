import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../App';
import { ApiContext } from '../apiContext';
import { createMockApi, mockValidate } from '../mockApi';
import { lineDiffStats, splitConfigFiles } from '../util';

describe('utilities', () => {
  it('counts added/removed lines', () => {
    expect(lineDiffStats('a\nb\nc', 'a\nB\nc\nd')).toEqual({ added: 2, removed: 1 });
    expect(lineDiffStats('x', 'x')).toEqual({ added: 0, removed: 0 });
  });

  it('puts DefaultEngine/DefaultGame first', () => {
    const r = splitConfigFiles(['A/DefaultInput.ini', 'A/DefaultGame.ini', 'A/DefaultEngine.ini']);
    expect(r.primary).toEqual(['A/DefaultEngine.ini', 'A/DefaultGame.ini']);
    expect(r.others).toEqual(['A/DefaultInput.ini']);
  });

  it('mock validator flags the sample warning', () => {
    const v = mockValidate('[S]\nfoo\n+Arr=(a\n');
    expect(v.sections).toEqual([{ name: 'S', line: 1 }]);
    expect(v.issues.map((i) => i.severity)).toEqual(['warning', 'error']);
  });
});

describe('app (mock backend)', () => {
  it('scans a folder, lists paks and opens the editor', async () => {
    const user = userEvent.setup();
    render(
      <ApiContext.Provider value={createMockApi()}>
        <App hasToken mock />
      </ApiContext.Provider>,
    );
    await user.type(screen.getByLabelText('Game Content folder'), 'C:/Games/A2/Content');
    await user.click(screen.getByRole('button', { name: 'Scan' }));

    const unsupported = await screen.findByRole('button', { name: /pakchunk1-Windows\.pak/ });
    expect(unsupported).toBeDisabled();
    expect(screen.getByText(/Encrypted pak index/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /A2-Windows\.pak/ }));
    expect(await screen.findByRole('tab', { name: /DefaultEngine\.ini/ })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: /DefaultGame\.ini/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Review changes/ })).toBeDisabled();
    expect(screen.getByText(/will be ignored/)).toBeInTheDocument();
  });

  it('shows the session screen without a token', () => {
    render(
      <ApiContext.Provider value={createMockApi()}>
        <App hasToken={false} mock={false} />
      </ApiContext.Provider>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('No session token');
  });
});
