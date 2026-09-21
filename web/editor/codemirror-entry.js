import { basicSetup, EditorView } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { linter } from '@codemirror/lint';
import { javascript } from '@codemirror/lang-javascript';
import { json } from '@codemirror/lang-json';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { python } from '@codemirror/lang-python';
import { java } from '@codemirror/lang-java';
import { cpp } from '@codemirror/lang-cpp';
import { rust } from '@codemirror/lang-rust';
import { go } from '@codemirror/lang-go';
import { sql } from '@codemirror/lang-sql';
import { markdown } from '@codemirror/lang-markdown';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';

const languages = {
  javascript: () => javascript(),
  js: () => javascript(),
  mjs: () => javascript(),
  jsx: () => javascript({ jsx: true }),
  typescript: () => javascript({ typescript: true }),
  ts: () => javascript({ typescript: true }),
  tsx: () => javascript({ typescript: true, jsx: true }),
  json,
  html,
  htm: html,
  css,
  python,
  py: python,
  java,
  cpp,
  c: cpp,
  hpp: cpp,
  h: cpp,
  rust,
  rs: rust,
  go,
  sql,
  markdown,
  md: markdown,
  yaml,
  yml: yaml
};

function structuralLint(view) {
  const text = view.state.doc.toString();
  const diagnostics = [];
  const stack = [];
  const pairs = { ')': '(', ']': '[', '}': '{' };
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index];
    if ('([{'.includes(character)) stack.push({ character, index });
    if (!')]}'.includes(character)) continue;
    const expected = pairs[character];
    const previous = stack.pop();
    if (!previous || previous.character !== expected) {
      diagnostics.push({ from: index, to: index + 1, severity: 'error', message: `Unexpected ${character}` });
    }
  }
  while (stack.length) {
    const item = stack.pop();
    diagnostics.push({ from: item.index, to: item.index + 1, severity: 'error', message: `Unclosed ${item.character}` });
  }
  return diagnostics.slice(0, 100);
}

function jsonLint(view) {
  const text = view.state.doc.toString();
  try {
    JSON.parse(text);
    return [];
  } catch (error) {
    const match = String(error && error.message || '').match(/position (\d+)/i);
    const position = match ? Number(match[1]) : 0;
    return [{ from: position, to: Math.min(position + 1, text.length), severity: 'error', message: 'Invalid JSON: ' + String(error.message || 'parse error') }];
  }
}

function lintFor(languageName) {
  return languageName === 'json' ? jsonLint : structuralLint;
}

export function mount(container, options = {}) {
  const languageName = String(options.language || '').toLowerCase();
  const language = languages[languageName];
  const extensions = [basicSetup, oneDark, linter(lintFor(languageName), { delay: 500 })];
  if (language) extensions.push(language());
  if (typeof options.onChange === 'function') {
    extensions.push(EditorView.updateListener.of(update => {
      if (update.docChanged) options.onChange(update.state.doc.toString());
    }));
  }
  const view = new EditorView({
    state: EditorState.create({ doc: String(options.value || ''), extensions }),
    parent: container
  });
  return {
    getValue: () => view.state.doc.toString(),
    setValue: value => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: String(value || '') } }),
    focus: () => view.focus(),
    destroy: () => view.destroy()
  };
}
