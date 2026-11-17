// What the controllers share about the DOM: the delegated action signature and the targeted updates
// (no full re-render) more than one room uses.
import { ui } from '../core/state';

/** A `data-act` handler: the element's `data-v` and the element itself. */
export type Act = (v: string | undefined, el: HTMLElement) => void;

export const $ = <E extends Element = HTMLElement>(sel: string) => document.querySelector<E>(sel);
export const reduced = () => ui.shot || matchMedia('(prefers-reduced-motion: reduce)').matches;

export function setChecked(v: string): void {
  document.querySelectorAll<HTMLElement>('.opt').forEach((o) => {
    o.setAttribute('aria-checked', String(o.dataset.v === v));
    // A long question scrolls its options: the one chosen by key comes into view.
    if (o.dataset.v === v) o.scrollIntoView({ block: 'nearest' });
  });
}

export function syncPressed(act: string, v: string): void {
  document.querySelectorAll<HTMLElement>(`[data-act="${act}"]`).forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.v === v)));
}
