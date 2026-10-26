import { useEffect, useRef } from 'react';
import { EditorState } from '@codemirror/state';
import { EditorView, lineNumbers } from '@codemirror/view';
import { MergeView } from '@codemirror/merge';
import { iniExtensions } from '../cmSetup';

interface Props {
  original: string;
  modified: string;
}

/** Side-by-side read-only diff (left: as loaded from the pak, right: your edits). */
export function DiffView({ original, modified }: Props) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!host.current) return;
    const ro = [EditorState.readOnly.of(true), EditorView.editable.of(false), lineNumbers(), ...iniExtensions];
    const mv = new MergeView({
      a: { doc: original, extensions: ro },
      b: { doc: modified, extensions: ro },
      parent: host.current,
      collapseUnchanged: { margin: 3, minSize: 6 },
      highlightChanges: true,
      gutter: true,
    });
    return () => mv.destroy();
  }, [original, modified]);
  return (
    <div className="diff">
      <div className="diff-labels">
        <span>In pak (loaded)</span>
        <span>Your version</span>
      </div>
      <div className="diff-host" ref={host} />
    </div>
  );
}
