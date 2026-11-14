// Pass Quest desktop window — entry point. Installs the error bar, wires the delegated events, the
// router, the keyboard and the display clock, then renders the current route.
//
// Inside a MyGo window the study service is bound at boot (`connectStudy()`, src/api/client.ts).
// Elsewhere every screen shows its "service not connected" state; nothing is invented.
import '../styles/index.css';
import { bindApi, connectStudy } from './api/client';
import { ACT, doMark, keyActions, onDeadline, onFieldInput, onRoute, setEv } from './actions';
import { checkSelection, hideMarkPop, onMark } from './components/passage';
import { startClock } from './core/clock';
import { installKeyboard } from './core/keyboard';
import { moreCue, render, setScreens } from './core/render';
import { installRouter, parseHash } from './core/router';
import { ui } from './core/state';
import { screens } from './screens';

// A visible red bar for any uncaught error, as in the prototype (text only).
function errorBar(message: string): void {
  let b = document.getElementById('errbar');
  if (!b) {
    b = document.createElement('div');
    b.id = 'errbar';
    document.documentElement.appendChild(b);
  }
  b.textContent = 'JS error: ' + message;
  b.style.cssText = 'position:fixed;left:0;right:0;top:0;z-index:99999;background:#c4122f;color:#fff;font:600 14px/1.4 Consolas,monospace;padding:10px 16px';
}
window.onerror = (msg, _src, line, col) => {
  errorBar(`${String(msg)} @${line ?? 0}:${col ?? 0}`);
};
window.addEventListener('unhandledrejection', (e) => errorBar(String(e.reason instanceof Error ? e.reason.message : e.reason)));

function delegate(): void {
  document.addEventListener('click', (e) => {
    const el = e.target instanceof Element ? e.target.closest<HTMLElement>('[data-act]') : null;
    if (!el || (el as HTMLButtonElement).disabled) return;
    const fn = ACT[el.dataset.act ?? ''];
    if (fn) {
      e.preventDefault();
      fn(el.dataset.v, el);
    }
  });
  // Enter inside a dialog form saves instead of navigating the page.
  document.addEventListener('submit', (e) => {
    e.preventDefault();
    const form = e.target instanceof HTMLFormElement ? e.target.dataset.form : undefined;
    if (form === 'learner') ACT['learner-save']?.(undefined, document.body);
    else if (form === 'credential') ACT['credential-save']?.(undefined, document.body);
  });
  document.addEventListener('input', (e) => {
    const t = e.target;
    if (t instanceof HTMLTextAreaElement && t.dataset.field) onFieldInput(t.dataset.field, t.value);
  });
  document.addEventListener(
    'scroll',
    (e) => {
      if (e.target instanceof HTMLElement && e.target.id === 'coscroll') moreCue();
    },
    true,
  );
  // feedback: hover or focus an option → its evidence lights
  const evFrom = (e: Event) => {
    const fo = e.target instanceof Element ? e.target.closest<HTMLElement>('.fo[data-ev]') : null;
    if (fo?.dataset.ev) setEv(fo.dataset.ev);
  };
  document.addEventListener('pointerover', evFrom);
  document.addEventListener('focusin', evFrom);
  // practice: selection → mark popover (mouse or keyboard)
  let pointerDown = false;
  document.addEventListener('pointerdown', (e) => {
    if (!(e.target instanceof Element && e.target.closest('.markpop'))) pointerDown = true;
  });
  document.addEventListener('pointerup', () => {
    if (pointerDown) {
      pointerDown = false;
      setTimeout(checkSelection, 0);
    }
  });
  document.addEventListener('selectionchange', () => {
    if (!pointerDown) checkSelection();
  });
  onMark(() => void doMark());
}

declare global {
  interface Window {
    /** Boot state read by the desktop smoke: ready once the first real Today has rendered. */
    __pq?: { ready: boolean };
  }
}

/** Binds the Go service; `__pq.ready` turns true after the first Today that loaded has rendered. */
function bindStudy(): void {
  const study = connectStudy();
  if (!study) return;
  window.__pq = { ready: false };
  bindApi({
    ...study,
    async today() {
      const value = await study.today();
      // The loader renders right after this promise settles; mark ready after that render.
      setTimeout(() => {
        if (window.__pq && document.querySelector('#stage')?.children.length) window.__pq.ready = true;
      }, 0);
      return value;
    },
  });
}

function boot(): void {
  bindStudy();
  setScreens(screens);
  delegate();
  installKeyboard({ ...keyActions, hideMarkPop });
  installRouter(onRoute);
  startClock(ui.shot, onDeadline);
  onRoute(parseHash(location.hash));
  render(true, false);
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
else boot();
