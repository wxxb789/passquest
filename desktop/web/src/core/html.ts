// Escaping HTML templates. Every interpolated value is escaped unless it is already an `Html` produced
// by `html` itself; `raw()` exists only for the static, hand-written SVG path table in icons.ts.
// Model text, learner text and imported source text must never reach `raw()`.

const ESCAPES: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export function esc(value: string): string {
  return value.replace(/[&<>"']/g, (c) => ESCAPES[c] ?? c);
}

export class Html {
  constructor(readonly value: string) {}
  toString(): string {
    return this.value;
  }
}

export type Interp = Html | string | number | boolean | null | undefined | readonly Interp[];

function part(v: Interp): string {
  if (v === null || v === undefined) return '';
  if (v instanceof Html) return v.value;
  if (Array.isArray(v)) return v.map(part).join('');
  return esc(String(v));
}

export function html(strings: TemplateStringsArray, ...values: Interp[]): Html {
  let out = strings[0] ?? '';
  for (let i = 0; i < values.length; i++) out += part(values[i]) + (strings[i + 1] ?? '');
  return new Html(out);
}

/** Trusted static markup only (the icon path table). Never pass data through this. */
export function raw(trustedStatic: string): Html {
  return new Html(trustedStatic);
}

export const EMPTY = new Html('');

export function join(items: readonly Html[], sep = ''): Html {
  return new Html(items.map((h) => h.value).join(esc(sep)));
}

/** Seconds → m:ss, as the prototype's mmss(). Negative values clamp to 0:00. */
export function mmss(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}
