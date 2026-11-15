// Display-only clocks. The window never counts time itself: every clock shows the distance between
// "now" and a server timestamp written into the DOM (presented_at, item and run deadlines). A view
// marks an element with
//   data-clock="since" data-t=<iso>                  → elapsed m:ss since t (practice: your time)
//   data-clock="until" data-t=<iso>                  → remaining m:ss until t (assessment)
//   data-clock="ring"  data-from=<iso> data-t=<iso>  → SVG circle progress between from and t
// and the clock rewrites those elements once per second. When an `until` element carrying
// `data-notify` reaches its deadline, `onDeadline` fires once so the app can ask the core what
// happened (the core decides timeouts; the window only notices that one is due).
import { mmss } from './html';

const RING_C = 2 * Math.PI * 25;
const notified = new Set<string>();
let interval: ReturnType<typeof setInterval> | undefined;
let handler: (key: string) => void = () => {};

function ms(iso: string | undefined): number {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isNaN(t) ? NaN : t;
}

export function tickClocks(): void {
  const now = Date.now();
  document.querySelectorAll<HTMLElement | SVGElement>('[data-clock]').forEach((el) => {
    const kind = el.dataset.clock;
    const t = ms(el.dataset.t);
    if (Number.isNaN(t)) return;
    if (kind === 'since') {
      el.textContent = mmss((now - t) / 1000);
    } else if (kind === 'until') {
      const left = (t - now) / 1000;
      el.textContent = mmss(Math.ceil(left));
      const key = el.dataset.notify;
      if (key && left <= 0 && !notified.has(key)) {
        notified.add(key);
        handler(key);
      }
    } else if (kind === 'ring') {
      const from = ms(el.dataset.from);
      if (Number.isNaN(from) || t <= from) return;
      const frac = Math.min(1, Math.max(0, (now - from) / (t - from)));
      el.setAttribute('stroke-dashoffset', (RING_C * (1 - frac)).toFixed(1));
    }
  });
}

/** Starts the once-per-second display refresh. Shot mode renders a single frame and never ticks. */
export function startClock(shot: boolean, onDeadline: (key: string) => void): void {
  handler = onDeadline;
  if (shot || interval !== undefined) return;
  interval = setInterval(tickClocks, 1000);
}

export const RING_CIRCUMFERENCE = RING_C;
