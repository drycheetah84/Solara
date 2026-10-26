import { useMemo, useState } from 'react';
import type { Section } from '../api';

interface Props {
  sections: Section[];
  onJump: (line: number) => void;
}

export function SectionNav({ sections, onJump }: Props) {
  const [filter, setFilter] = useState('');
  const shown = useMemo(() => {
    const f = filter.trim().toLowerCase();
    return f ? sections.filter((s) => s.name.toLowerCase().includes(f)) : sections;
  }, [sections, filter]);

  return (
    <nav className="section-nav" aria-label="Sections">
      <label htmlFor="section-filter" className="section-title">
        Sections <span className="count">{sections.length}</span>
      </label>
      <input
        id="section-filter"
        type="search"
        placeholder="Filter sections…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        spellCheck={false}
      />
      <ul>
        {shown.map((s) => (
          <li key={`${s.line}:${s.name}`}>
            <button type="button" className="section-link" onClick={() => onJump(s.line)} title={`[${s.name}] — line ${s.line}`}>
              <span className="section-name">{s.name}</span>
              <span className="section-line">{s.line}</span>
            </button>
          </li>
        ))}
        {shown.length === 0 && <li className="muted small empty">{sections.length ? 'No matching sections' : 'No sections'}</li>}
      </ul>
    </nav>
  );
}
