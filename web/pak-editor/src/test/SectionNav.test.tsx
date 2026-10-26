import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SectionNav } from '../components/SectionNav';

const sections = [
  { name: '/Script/Engine.Engine', line: 1 },
  { name: '/Script/Engine.RendererSettings', line: 12 },
  { name: 'Core.Log', line: 30 },
];

describe('SectionNav', () => {
  it('filters sections case-insensitively and jumps on click', async () => {
    const onJump = vi.fn();
    const user = userEvent.setup();
    render(<SectionNav sections={sections} onJump={onJump} />);

    expect(screen.getAllByRole('button')).toHaveLength(3);
    const filter = screen.getByRole('searchbox');
    await user.type(filter, 'RENDERER');
    const buttons = screen.getAllByRole('button');
    expect(buttons).toHaveLength(1);
    expect(buttons[0]).toHaveTextContent('/Script/Engine.RendererSettings');

    await user.click(buttons[0]);
    expect(onJump).toHaveBeenCalledWith(12);

    await user.clear(filter);
    await user.type(filter, 'zzz');
    expect(screen.queryAllByRole('button')).toHaveLength(0);
    expect(screen.getByText('No matching sections')).toBeInTheDocument();
  });
});
