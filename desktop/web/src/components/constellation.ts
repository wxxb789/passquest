// Evidence constellation icon (prototype cstIco): one dot per attempt on a ring, starting at 12
// o'clock — filled = correct, ring = miss, amber = assisted.
import { html, type Html } from '../core/html';

export function cstIco(ok: number, miss: number, as: number): Html {
  const kinds = [...Array<string>(ok).fill('ok'), ...Array<string>(miss).fill('miss'), ...Array<string>(as).fill('as')];
  const n = kinds.length;
  const dots = kinds.map((k, i) => {
    const t = -Math.PI / 2 + (i * 2 * Math.PI) / n;
    return html`<circle class="${k}" cx="${(18 + 11 * Math.cos(t)).toFixed(2)}" cy="${(18 + 11 * Math.sin(t)).toFixed(2)}" r="2.6"/>`;
  });
  return html`<span class="ico cst"><svg viewBox="0 0 36 36" aria-hidden="true">${dots}</svg></span>`;
}
