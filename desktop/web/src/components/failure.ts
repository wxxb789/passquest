// A typed failure in the window's words: the localized title, the localized next step and the
// buttons that act on it. The core's English message and its CLI next step stay available, folded
// into a "details" disclosure marked as raw text (for a bug report), never as the primary text.
import type { StudioError } from '../api/types';
import { html, type Html, type Interp } from '../core/html';
import { errorLabel, nextStep, SETTINGS_CODES, T } from '../core/i18n';
import { ic } from '../icons';

/** "Open Settings" for a failure whose next step is there; nothing otherwise. */
export function settingsLink(code: string): Html {
  if (!SETTINGS_CODES.has(code)) return html``;
  return html`<a class="btn quiet sm" href="#/settings">${T('打开设置', 'Open Settings')}${ic('arrow', 'xs')}</a>`;
}

/** The core's message, code and CLI step, folded away. */
export function rawDetail(e: { code: string; message?: string; action?: string | null }): Html {
  if (!e.message && !e.action) return html``;
  return html`<details class="raw"><summary>${T('技术细节', 'Details')}</summary><p data-raw translate="no" lang="en"><code>${e.code}</code>${
    e.message ? html` ${e.message}` : ''
  }</p>${e.action ? html`<p data-raw translate="no" lang="en"><code>${e.action}</code></p>` : ''}</details>`;
}

export interface FailureOptions {
  testid: string;
  /** Replaces the localized error title (e.g. "the call did not succeed"). */
  title?: Interp;
  retry?: Html | '';
  /** Another decision the failure offers (e.g. a consent grant), after the buttons. */
  extra?: Html | '';
}

export function failureNote(e: StudioError | { code: string; message: string; action?: string | null }, o: FailureOptions): Html {
  const next = nextStep(e.code);
  const acts = html`${o.retry ?? ''}${settingsLink(e.code)}`;
  return html`<div class="pending held fail" data-testid="${o.testid}" data-code="${e.code}"><b>${o.title ?? errorLabel(e.code)}</b>${
    o.title ? html` <span>${errorLabel(e.code)}</span>` : ''
  }${next ? html`<span class="next">${next}</span>` : ''}${acts.value ? html`<div class="acts-row">${acts}</div>` : ''}${o.extra ?? ''}${rawDetail(e)}</div>`;
}
