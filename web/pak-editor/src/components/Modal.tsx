import { useEffect, useRef, type ReactNode } from 'react';

interface Props {
  title: string;
  onClose?: () => void;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'small' | 'large' | 'drawer';
}

/** Accessible modal: focus is moved inside, Escape closes (when closable), focus is restored on unmount. */
export function Modal({ title, onClose, children, footer, size = 'small' }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useRef(`modal-${Math.random().toString(36).slice(2)}`).current;

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const el = ref.current;
    const first = el?.querySelector<HTMLElement>('[data-autofocus], button:not([disabled]), input, select, textarea');
    (first ?? el)?.focus();
    return () => prev?.focus?.();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const all = document.querySelectorAll('.modal');
      const topmost = all[all.length - 1] === ref.current;
      if (e.key === 'Escape' && onClose && topmost) {
        e.stopPropagation();
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div className={`modal-backdrop ${size === 'drawer' ? 'drawer-backdrop' : ''}`}>
      <div
        ref={ref}
        className={`modal modal-${size}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <header className="modal-header">
          <h2 id={titleId}>{title}</h2>
          {onClose && (
            <button type="button" className="icon-btn" onClick={onClose} aria-label="Close">
              ×
            </button>
          )}
        </header>
        <div className="modal-body">{children}</div>
        {footer && <footer className="modal-footer">{footer}</footer>}
      </div>
    </div>
  );
}
