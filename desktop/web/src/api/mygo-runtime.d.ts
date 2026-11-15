// Types for the `mygo-runtime` import of the generated ./mygo.ts, and for the runtime MyGo injects
// into the page as `window.mygo` (the subset this window uses).
//
// The npm package `mygo-runtime` is not a dependency: its functions are thin wrappers over
// `window.mygo`, so ./client.ts imports ./mygo.ts for its types only (the import is erased by the
// bundler) and calls `window.mygo` directly. These declarations mirror mygo-runtime v0.9.0
// (packages/runtime/src/index.ts in github.com/egoist/mygo).

interface MygoRuntime {
  /** Calls a bound Go method, e.g. `call("Study.Today")`; a Go error rejects with a CallError. */
  call<T = unknown>(method: string, ...args: unknown[]): Promise<T>;
  /** Subscribes to a Go event by name; returns a function that unsubscribes. */
  on<T = unknown>(name: string, listener: (payload: T) => void): () => void;
  once<T = unknown>(name: string, listener: (payload: T) => void): () => void;
}

interface Window {
  /** The MyGo runtime; missing outside MyGo windows. */
  mygo?: MygoRuntime;
}

declare module 'mygo-runtime' {
  export interface GoEvent<T> {
    readonly name: string;
    on(listener: (payload: T) => void): () => void;
    once(listener: (payload: T) => void): () => void;
  }
  export function call<T = unknown>(method: string, ...args: unknown[]): Promise<T>;
  export function event<T>(name: string): GoEvent<T>;
}
