import type { Issue } from '../api';

interface Props {
  issues: Issue[];
  validating: boolean;
  onJump: (line: number) => void;
}

export function ProblemsPanel({ issues, validating, onJump }: Props) {
  const errors = issues.filter((i) => i.severity === 'error').length;
  const warnings = issues.length - errors;
  const sorted = [...issues].sort((a, b) => a.line - b.line);
  return (
    <section className="problems" aria-label="Problems">
      <header className="problems-header">
        <span className="section-title">Problems</span>
        <span className={`badge ${errors ? 'badge-error' : 'badge-muted'}`}>{errors} error{errors === 1 ? '' : 's'}</span>
        <span className={`badge ${warnings ? 'badge-warning' : 'badge-muted'}`}>
          {warnings} warning{warnings === 1 ? '' : 's'}
        </span>
        {validating && <span className="muted small" aria-live="polite">checking…</span>}
      </header>
      {sorted.length === 0 ? (
        <p className="muted small problems-empty">No problems detected.</p>
      ) : (
        <ul className="problems-list">
          {sorted.map((i, idx) => (
            <li key={`${i.line}-${idx}`}>
              <button type="button" className={`problem problem-${i.severity}`} onClick={() => onJump(i.line)}>
                <span className="problem-sev">{i.severity === 'error' ? 'Error' : 'Warning'}</span>
                <span className="problem-line">Ln {i.line}</span>
                <span className="problem-msg">{i.message}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
