// The page's side of the hidden title bar (goal 0022). MyGo reports the room the window controls
// take as CSS variables on :root (mygo docs/frontend.md); the marker below turns on the CSS that
// reserves that room and drags the strip, and the caption buttons follow the page's own theme.
import { api } from '../api/client';
import type { Theme } from '../core/prefs';

const ROOM_VARS = ['--mygo-titlebar-height', '--mygo-titlebar-inset-left', '--mygo-titlebar-inset-right', '--mygo-titlebar-controls-bottom'];

const roomReported = (): boolean => {
  const style = getComputedStyle(document.documentElement);
  return ROOM_VARS.some((name) => parseFloat(style.getPropertyValue(name)) > 0);
};

/**
 * Sets `:root[data-titlebar]` while the window reports room for its controls, from boot on. The
 * read covers a room that arrived before this module ran; `mygo:title-bar` (not replayed to late
 * subscribers, mygo packages/bridge/src/bridge.ts) covers every change after it.
 */
export function watchTitleBar(): void {
  const apply = () => document.documentElement.toggleAttribute('data-titlebar', roomReported());
  window.mygo?.on('mygo:title-bar', apply);
  apply();
}

/** Tells the window's caption buttons which appearance the page draws (Q-0022-02); a failed cosmetic call is dropped. */
export function applyWindowButtonsTheme(theme: Theme): void {
  api()
    ?.setWindowButtonsTheme(theme)
    .catch(() => undefined);
}
