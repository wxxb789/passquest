// DIAGNOSTIC (not for merge). Imported first by main.ts, so it runs before any other module.
// 1. A probe: a known error thrown from this module's own timer. If window.onerror receives it as
//    the opaque "Script error.", WebKit mutes this bundle's own errors too (DIAG probe muted).
// 2. Wrappers around listener and observer callbacks registered from now on: a synchronous throw
//    is named with its stack in the error bar (DIAG listener/observer) instead of reaching onerror.
const bar = (text: string) => {
  let b = document.getElementById('errbar');
  if (!b) {
    b = document.createElement('div');
    b.id = 'errbar';
    document.documentElement.appendChild(b);
  }
  b.textContent = (b.textContent ? b.textContent + ' || ' : '') + text;
};
const describe = (error: unknown) =>
  error instanceof Error ? `${error.name}: ${error.message} | ${(error.stack ?? '').replace(/\s+/g, ' ').slice(0, 260)}` : String(error);

const PROBE = 'pq-diag-probe';
(window as unknown as { __pqProbe: string }).__pqProbe = 'pending';
window.addEventListener('error', (event) => {
  if (event.message?.includes(PROBE)) {
    (window as unknown as { __pqProbe: string }).__pqProbe = 'clear';
    event.preventDefault();
    event.stopImmediatePropagation();
  }
});
window.setTimeout(() => {
  throw new Error(PROBE);
}, 0);
window.setTimeout(() => {
  if ((window as unknown as { __pqProbe: string }).__pqProbe !== 'clear') bar('DIAG probe muted: this bundle\'s own errors reach onerror as "Script error."');
}, 500);

const wrapCallback = (kind: string, fn: unknown) =>
  typeof fn === 'function'
    ? function (this: unknown, ...a: unknown[]) {
        try {
          return (fn as (...b: unknown[]) => unknown).apply(this, a);
        } catch (error) {
          bar(`DIAG ${kind}: ${describe(error)}`);
          return undefined;
        }
      }
    : fn;

const wrapped = new WeakMap<object, unknown>();
const nativeAdd = EventTarget.prototype.addEventListener;
const nativeRemove = EventTarget.prototype.removeEventListener;
EventTarget.prototype.addEventListener = function (type: string, listener: unknown, options?: unknown) {
  if (typeof listener !== 'function') return nativeAdd.call(this, type, listener as EventListener, options as AddEventListenerOptions);
  let w = wrapped.get(listener as object);
  if (!w) {
    w = wrapCallback(`listener ${type}`, listener);
    wrapped.set(listener as object, w);
  }
  return nativeAdd.call(this, type, w as EventListener, options as AddEventListenerOptions);
};
EventTarget.prototype.removeEventListener = function (type: string, listener: unknown, options?: unknown) {
  const w = typeof listener === 'function' ? (wrapped.get(listener as object) ?? listener) : listener;
  return nativeRemove.call(this, type, w as EventListener, options as EventListenerOptions);
};
const NativeMO = window.MutationObserver;
window.MutationObserver = class extends NativeMO {
  constructor(cb: MutationCallback) {
    super(wrapCallback('mutation-observer', cb) as MutationCallback);
  }
};
const NativeRO = window.ResizeObserver;
window.ResizeObserver = class extends NativeRO {
  constructor(cb: ResizeObserverCallback) {
    super(wrapCallback('resize-observer', cb) as ResizeObserverCallback);
  }
};
export {};
