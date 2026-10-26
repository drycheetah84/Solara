import { useCallback, useEffect, useRef, useState } from 'react';
import { pollJob, type JobStarted, type JobStatus } from './api';
import { useApi } from './apiContext';

/** Starts a backend job (save/restore) and polls it, exposing live status. */
export function useJob() {
  const api = useApi();
  const [status, setStatus] = useState<JobStatus | null>(null);
  const [running, setRunning] = useState(false);
  const abort = useRef<AbortController | null>(null);

  useEffect(() => () => abort.current?.abort(), []);

  const run = useCallback(
    async (start: () => Promise<JobStarted>): Promise<JobStatus> => {
      abort.current?.abort();
      const ctrl = new AbortController();
      abort.current = ctrl;
      setStatus(null);
      setRunning(true);
      try {
        const { jobId } = await start();
        setStatus({ state: 'running', phase: 'writing', progress: 0 });
        return await pollJob(
          api,
          jobId,
          (s) => {
            if (!ctrl.signal.aborted) setStatus(s);
          },
          { signal: ctrl.signal },
        );
      } finally {
        if (!ctrl.signal.aborted) setRunning(false);
      }
    },
    [api],
  );

  return { status, running, run };
}
