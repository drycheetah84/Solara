// Shared CodeMirror 6 setup: UE ini language, dark theme, highlight style.
import { type Extension } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { HighlightStyle, StreamLanguage, syntaxHighlighting, type StreamParser } from '@codemirror/language';
import { properties } from '@codemirror/legacy-modes/mode/properties';
import { tags as t } from '@lezer/highlight';

type PropState = Record<string, unknown>;
const base = properties as unknown as StreamParser<PropState>;

/**
 * Unreal ini flavour of the legacy "properties" mode: the UE array operators
 * (+Key, -Key, .Key, !Key, @Key) are highlighted as operators instead of
 * being treated as comments ("!") or part of the key.
 */
const ueIni: StreamParser<PropState> = {
  name: 'ini',
  startState: (indentUnit: number) => (base.startState ? base.startState(indentUnit) : {}),
  token(stream, state) {
    if (stream.sol() && stream.match(/^\s*[+\-.!@](?=[^\s=;#])/)) {
      state.position = 'def';
      state.afterSection = false;
      state.inMultiline = false;
      state.nextMultiline = false;
      return 'operator';
    }
    return base.token(stream, state);
  },
  languageData: { commentTokens: { line: ';' } },
};

export const iniLanguage = StreamLanguage.define(ueIni);

const highlight = HighlightStyle.define([
  { tag: t.comment, color: '#6f7f95', fontStyle: 'italic' },
  { tag: t.heading, color: '#7cc4ff', fontWeight: '600' },
  { tag: t.definition(t.variableName), color: '#e6c07b' },
  { tag: t.variableName, color: '#e6c07b' },
  { tag: [t.string, t.quote], color: '#a8d4a0' },
  { tag: t.operator, color: '#ff9e64', fontWeight: '600' },
]);

export const darkTheme = EditorView.theme(
  {
    '&': { color: '#d7dde8', backgroundColor: '#14181f', height: '100%', fontSize: '13px' },
    '.cm-scroller': { fontFamily: 'var(--mono)', lineHeight: '1.55' },
    '.cm-content': { caretColor: '#ffffff' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#ffffff' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: '#2f4a6e',
    },
    '.cm-gutters': { backgroundColor: '#11151b', color: '#5d6878', border: 'none', borderRight: '1px solid #232a35' },
    '.cm-activeLine': { backgroundColor: '#1a2029' },
    '.cm-activeLineGutter': { backgroundColor: '#1a2029', color: '#aeb8c8' },
    '.cm-matchingBracket': { backgroundColor: '#35506f', outline: '1px solid #5a86b8' },
    '.cm-nonmatchingBracket': { backgroundColor: '#6b2a2a' },
    '.cm-searchMatch': { backgroundColor: '#5a4a1a', outline: '1px solid #8a7330' },
    '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: '#8a6d1a' },
    '.cm-selectionMatch': { backgroundColor: '#2a3a2a' },
    '.cm-panels': { backgroundColor: '#1b212b', color: '#d7dde8', borderBottom: '1px solid #2b3442' },
    '.cm-panels input, .cm-panels button': { fontSize: '12px' },
    '.cm-textfield': {
      backgroundColor: '#0f1318',
      color: '#d7dde8',
      border: '1px solid #3a4556',
      borderRadius: '4px',
    },
    '.cm-button': { backgroundImage: 'none', backgroundColor: '#273041', color: '#d7dde8', border: '1px solid #3a4556' },
    '.cm-issue-error': { backgroundColor: 'rgba(255, 85, 85, 0.13)' },
    '.cm-issue-warning': { backgroundColor: 'rgba(240, 190, 60, 0.10)' },
    '.cm-flash-line': { backgroundColor: 'rgba(124, 196, 255, 0.18)' },
  },
  { dark: true },
);

export const iniExtensions: Extension[] = [iniLanguage, syntaxHighlighting(highlight), darkTheme];
