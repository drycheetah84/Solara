import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { Compartment, EditorState, RangeSetBuilder, StateEffect, StateField } from '@codemirror/state';
import {
  Decoration,
  type DecorationSet,
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search';
import { bracketMatching } from '@codemirror/language';
import type { Issue } from '../api';
import { iniExtensions } from '../cmSetup';

export interface IniEditorHandle {
  goToLine(line: number): void;
  /** Replaces the document for `key` (drops its undo history). */
  resetDoc(key: string, text: string): void;
  focus(): void;
}

interface Props {
  docKey: string;
  initialText: string;
  readOnly: boolean;
  issues: Issue[];
  onChange: (key: string, text: string) => void;
}

const setIssues = StateEffect.define<Issue[]>();

const issueField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    deco = deco.map(tr.changes);
    for (const e of tr.effects) {
      if (e.is(setIssues)) {
        const byLine = new Map<number, 'error' | 'warning'>();
        for (const i of e.value) {
          if (i.line < 1 || i.line > tr.state.doc.lines) continue;
          if (byLine.get(i.line) !== 'error') byLine.set(i.line, i.severity);
        }
        const b = new RangeSetBuilder<Decoration>();
        [...byLine.keys()]
          .sort((a, c) => a - c)
          .forEach((ln) => {
            const pos = tr.state.doc.line(ln).from;
            b.add(pos, pos, Decoration.line({ class: `cm-issue-${byLine.get(ln)}` }));
          });
        deco = b.finish();
      }
    }
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

export const IniEditor = forwardRef<IniEditorHandle, Props>(function IniEditor(
  { docKey, initialText, readOnly, issues, onChange },
  ref,
) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const states = useRef(new Map<string, EditorState>());
  const currentKey = useRef(docKey);
  const roCompartment = useRef(new Compartment());
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const issuesRef = useRef(issues);
  issuesRef.current = issues;
  const readOnlyRef = useRef(readOnly);
  readOnlyRef.current = readOnly;

  function makeState(key: string, text: string): EditorState {
    return EditorState.create({
      doc: text,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        history(),
        drawSelection(),
        highlightActiveLine(),
        bracketMatching(),
        highlightSelectionMatches(),
        search({ top: true }),
        keymap.of([...searchKeymap, ...historyKeymap, ...defaultKeymap, indentWithTab]),
        roCompartment.current.of(EditorState.readOnly.of(readOnlyRef.current)),
        issueField,
        EditorView.contentAttributes.of({ 'aria-label': `Editor for ${key}` }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current(key, u.state.doc.toString());
        }),
        ...iniExtensions,
      ],
    });
  }

  function syncDecorations(v: EditorView) {
    v.dispatch({
      effects: [
        setIssues.of(issuesRef.current),
        roCompartment.current.reconfigure(EditorState.readOnly.of(readOnlyRef.current)),
      ],
    });
  }

  // Create the view once.
  useEffect(() => {
    if (!host.current) return;
    const v = new EditorView({ parent: host.current, state: makeState(docKey, initialText) });
    view.current = v;
    currentKey.current = docKey;
    syncDecorations(v);
    return () => {
      v.destroy();
      view.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Switch documents, keeping each tab's state (undo history, selection, scroll anchor).
  useEffect(() => {
    const v = view.current;
    if (!v || currentKey.current === docKey) return;
    states.current.set(currentKey.current, v.state);
    currentKey.current = docKey;
    v.setState(states.current.get(docKey) ?? makeState(docKey, initialText));
    syncDecorations(v);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [docKey]);

  useEffect(() => {
    if (view.current) syncDecorations(view.current);
  }, [issues, readOnly]);

  useImperativeHandle(ref, () => ({
    goToLine(line: number) {
      const v = view.current;
      if (!v) return;
      const ln = v.state.doc.line(Math.min(Math.max(1, line), v.state.doc.lines));
      v.dispatch({
        selection: { anchor: ln.from },
        effects: EditorView.scrollIntoView(ln.from, { y: 'start', yMargin: 48 }),
      });
      v.focus();
    },
    resetDoc(key: string, text: string) {
      states.current.delete(key);
      const v = view.current;
      if (v && currentKey.current === key) {
        v.setState(makeState(key, text));
        syncDecorations(v);
      }
    },
    focus() {
      view.current?.focus();
    },
  }));

  return <div className="cm-host" ref={host} />;
});
