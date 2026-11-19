// Uncaught-error handling for the desktop window: the red bar the prototype draws for any error, and
// the one deliberate exception (Q-0013-11).
//
// On macOS, WKWebView reports errors from scripts MyGo injects or evaluates as an opaque
// `Script error.` with no source, line, column or error object. That is not an app error, so it is
// counted in `window.__pqErrors.opaque` for diagnosis instead of shown; anything else — including an
// unhandled rejection — draws the bar exactly as before.
//
// `startProbe()` is the guard: the desktop smoke calls it on every boot on every OS, and it throws an
// error defined in this bundle. The smoke fails unless that error reaches `onerror` in full; were our
// own error ever muted to `Script error.`, the classification would hide real app errors.

/** The probe error's text, matched in the `onerror` message by this module and awaited by the smoke. */
const PROBE_MESSAGE = 'passquest-desktop-probe';

declare global {
  interface Window {
    /** Diagnostic error state the desktop smoke reads; `installErrorBar()` installs it. */
    __pqErrors: {
      /** Opaque, source-less errors counted since boot; the app never shows a bar for them. */
      opaque: number;
      /** `pending` from `startProbe()` until the probe reaches `onerror` in full, then `clear`. */
      probe: 'idle' | 'pending' | 'clear';
      /** Plants one error thrown by this bundle and waits to see whether `onerror` gets it whole. */
      startProbe(): void;
    };
  }
}

/** True only for an error with no origin at all: the exact message, no source, position or object. */
export function isOpaqueScriptError(msg: unknown, src: unknown, line: unknown, col: unknown, error: unknown): boolean {
  if (msg !== 'Script error.' && msg !== 'Script error') return false;
  if (src !== undefined && src !== '') return false;
  if (line !== undefined && line !== 0) return false;
  if (col !== undefined && col !== 0) return false;
  return error === undefined || error === null;
}

/** A visible red bar for any uncaught error, as in the prototype (text only). */
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

/** Installs the global error handling; called once, before the first screen renders. */
export function installErrorBar(): void {
  const state: Window['__pqErrors'] = {
    opaque: 0,
    probe: 'idle',
    startProbe() {
      state.probe = 'pending';
      setTimeout(() => {
        throw new Error(PROBE_MESSAGE);
      }, 0);
    },
  };
  window.__pqErrors = state;

  window.onerror = (msg, src, line, col, error) => {
    if (typeof msg === 'string' && msg.includes(PROBE_MESSAGE)) {
      state.probe = 'clear';
      return true;
    }
    if (isOpaqueScriptError(msg, src, line, col, error)) {
      state.opaque += 1;
      console.warn(`passquest: opaque script error counted (${state.opaque}): ${msg}`);
      return true;
    }
    errorBar(`${String(msg)} @${line ?? 0}:${col ?? 0}`);
  };
  window.addEventListener('unhandledrejection', (e) => errorBar(String(e.reason instanceof Error ? e.reason.message : e.reason)));
}
