interface Props {
  reason: 'missing' | 'expired';
}

export function SessionExpired({ reason }: Props) {
  return (
    <main className="center-screen" role="alert">
      <div className="card narrow">
        <h1>{reason === 'expired' ? 'Session expired — restart pak-editor' : 'No session token'}</h1>
        {reason === 'expired' ? (
          <p>
            The pak-editor server no longer accepts this page&apos;s session. This happens when pak-editor was closed
            or restarted. Run <code>pak-editor</code> again and use the browser window it opens.
          </p>
        ) : (
          <p>
            This page must be opened through the link printed by <code>pak-editor</code> (it contains a one-time
            session token). Start <code>pak-editor</code> and use the window it opens.
          </p>
        )}
      </div>
    </main>
  );
}
