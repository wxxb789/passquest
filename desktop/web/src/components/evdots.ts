// Evidence dots (prototype evDots and the hero's .evid dots).
import { html, type Html } from '../core/html';

export type DotKind = 'ok' | 'miss' | 'as' | 're';

export function dots(kind: DotKind, n: number, small = false): Html[] {
  return Array.from({ length: Math.max(0, n) }, () => html`<span class="d ${kind}${small ? ' sm' : ''}"></span>`);
}

/** Independent attempts (correct first, then misses), then assisted, then re-exposure. */
export function evDots(ind: number, ok: number, as: number, re: number): Html {
  return html`${dots('ok', Math.min(ok, ind), true)}${dots('miss', Math.max(0, ind - ok), true)}${dots('as', as, true)}${dots('re', re, true)}`;
}
