// Dock (prototype dockHTML). In the assessment it is replaced by the item navigator.
import type { ActiveRun } from '../api/types';
import { html, type Html } from '../core/html';
import { DOCK, T } from '../core/i18n';
import type { Route } from '../core/state';
import { ic } from '../icons';
import { navigatorHTML } from '../views/assess';

export function dockHTML(route: Route, run: ActiveRun | null, ankiConflicts: number): Html {
  if (route === 'assess' && run) return navigatorHTML(run);
  const cur = route === 'feedback' ? 'practice' : route;
  return html`<div class="dock" role="navigation" aria-label="${T('主导航', 'Main')}">${DOCK.map(([r, zh, en]) => {
    const conflict = r === 'anki' && ankiConflicts > 0;
    return html`<a href="#/${r}" ${r === cur ? html`aria-current="page"` : ''} aria-label="${T(zh, en)}${conflict ? T(` · ${ankiConflicts} 个冲突`, ` · ${ankiConflicts} conflict`) : ''}">${ic(r)}<span class="lbl">${T(
      zh,
      en,
    )}</span><span class="tip" aria-hidden="true">${T(zh, en)}</span>${conflict ? html`<span class="badge" aria-hidden="true"></span>` : ''}</a>`;
  })}</div>`;
}
