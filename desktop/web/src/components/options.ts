// Answer options as a radio group (prototype optsHTML). Option text is exam content: escaped, lang=en.
import type { Option } from '../api/types';
import { html, type Html } from '../core/html';
import { T } from '../core/i18n';

export function optsHTML(options: readonly Option[], sel: string | null, act: 'pick' | 'apick'): Html {
  return html`<div class="opts" role="radiogroup" aria-label="${T('选项', 'Options')}">${options.map(
    (o) =>
      html`<button class="opt" role="radio" aria-checked="${sel === o.label}" data-act="${act}" data-v="${o.label}"><span class="key" aria-hidden="true">${o.label}</span><span class="txt exam" lang="en"><span class="sr">${o.label}. </span>${o.text}</span></button>`,
  )}</div>`;
}

/** Keyboard letter/digit → option label, by position. */
export function labelForKey(options: readonly Option[], key: string): string | null {
  const up = key.toUpperCase();
  const byLetter = options.find((o) => o.label.toUpperCase() === up);
  if (byLetter && key.length === 1) return byLetter.label;
  const n = '123456789'.indexOf(key);
  return key.length === 1 && n >= 0 && n < options.length ? options[n]!.label : null;
}
