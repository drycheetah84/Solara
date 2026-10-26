import type { JobPhase, JobStatus } from '../api';

const PHASES: { id: JobPhase; label: string }[] = [
  { id: 'writing', label: 'Writing new pak' },
  { id: 'verifying', label: 'Verifying' },
  { id: 'backing-up', label: 'Backing up' },
  { id: 'swapping', label: 'Swapping in' },
];

interface Props {
  status: JobStatus | null;
}

export function JobProgress({ status }: Props) {
  const pct = Math.round(Math.min(1, Math.max(0, status?.progress ?? 0)) * 100);
  const currentIdx = status ? PHASES.findIndex((p) => p.id === status.phase) : -1;
  const done = status?.state === 'done';
  return (
    <div className="job-progress">
      <ol className="phases" aria-label="Phases">
        {PHASES.map((p, i) => {
          const state = done || i < currentIdx ? 'done' : i === currentIdx ? 'active' : 'pending';
          return (
            <li key={p.id} className={`phase phase-${state}`} aria-current={state === 'active' ? 'step' : undefined}>
              {p.label}
            </li>
          );
        })}
      </ol>
      <div
        className="progress"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label="Progress"
      >
        <div className="progress-fill" style={{ width: `${pct}%` }} />
      </div>
      <div className="muted small">{done ? 'Finished' : `${pct}%`}</div>
    </div>
  );
}
