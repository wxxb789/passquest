// The prototype's hand-made 24×24 line icons (design/prototypes/c-stage.html lines 1045–1087).
// Static, trusted markup: this is the only module that may call raw().
import { html, raw, type Html } from './core/html';

const P = {
  today: '<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="2.6" fill="currentColor" stroke="none"/>',
  practice: '<path d="M6 3.5h8l4 4v13H6z"/><path d="M14 3.5v4h4"/><path d="M9 12h6M9 15.5h6"/>',
  assess: '<circle cx="12" cy="13.5" r="7"/><path d="M12 13.5V10M10 3.5h4M18.3 6.7l1.2-1.2"/>',
  report: '<path d="M4 19.5h16"/><path d="M7 16v-4M11 16V7.5M15 16v-6M19 16V9"/>',
  anki: '<rect x="4" y="7.5" width="12" height="12.5" rx="2"/><path d="M8 7.5V5.6A1.6 1.6 0 0 1 9.6 4h8.8A1.6 1.6 0 0 1 20 5.6v9.8a1.6 1.6 0 0 1-1.6 1.6H16"/>',
  settings: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  sun: '<circle cx="12" cy="12" r="3.8"/><path d="M12 3v1.8M12 19.2V21M3 12h1.8M19.2 12H21M5.6 5.6l1.3 1.3M17.1 17.1l1.3 1.3M5.6 18.4l1.3-1.3M17.1 6.9l1.3-1.3"/>',
  moon: '<path d="M19.5 14.6A7.8 7.8 0 0 1 9.4 4.5a7.9 7.9 0 1 0 10.1 10.1z"/>',
  help: '<circle cx="12" cy="12" r="8.5"/><path d="M9.7 9.6a2.4 2.4 0 1 1 3.2 2.3c-.6.2-.9.7-.9 1.3v.5"/><path d="M12 16.4v.2"/>',
  check: '<path d="M5 12.5l4.2 4.2L19 7"/>',
  x: '<path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/>',
  seal: '<circle cx="12" cy="12" r="8.5"/><path d="M8.4 12.2l2.5 2.5 4.8-5.1"/>',
  adv: '<circle cx="12" cy="12" r="8.5" stroke-dasharray="2.4 2.6"/><circle cx="12" cy="12" r="2" fill="currentColor" stroke="none"/>',
  ext: '<path d="M14 4h6v6M20 4l-8 8M18 14v4.5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 4 18.5v-11A1.5 1.5 0 0 1 5.5 6H10"/>',
  local: '<rect x="3.5" y="5" width="17" height="11" rx="1.8"/><path d="M8.5 19.5h7M12 16v3.5"/>',
  lock: '<rect x="5" y="10.5" width="14" height="9.5" rx="2"/><path d="M8 10.5V8a4 4 0 0 1 8 0v2.5"/>',
  clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
  pause: '<path d="M9.5 6.5v11M14.5 6.5v11"/>',
  play: '<path d="M8.5 6v12l9.5-6z"/>',
  arrow: '<path d="M5 12h14M13.5 6.5 19 12l-5.5 5.5"/>',
  chev: '<path d="M7 10l5 5 5-5"/>',
  down: '<path d="M12 5v14M6.5 13.5 12 19l5.5-5.5"/>',
  eye: '<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.8"/>',
  mark: '<path d="M4 20.5h8"/><path d="M14.6 4.6l4.8 4.8-9.2 9.2-5.4.9.9-5.4z"/>',
  conflict: '<path d="M7 4v11M4 12l3 3 3-3M17 20V9M14 12l3-3 3 3"/>',
  present: '<rect x="4" y="4" width="16" height="16" rx="3.5"/><path d="M8.5 12.2l2.4 2.4 4.6-4.9"/>',
  door: '<path d="M14 4h4.5A1.5 1.5 0 0 1 20 5.5v13a1.5 1.5 0 0 1-1.5 1.5H14"/><path d="M10 8l-4 4 4 4M6 12h10"/>',
  edit: '<path d="M4 20l1-5L15.5 4.5l4 4L9 19z"/>',
  phone: '<rect x="7" y="3" width="10" height="18" rx="2.5"/><path d="M11 18h2"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="M16 16l4.5 4.5"/>',
  shield: '<path d="M12 3.5l7 2.8v5.3c0 4.3-3 7.6-7 8.9-4-1.3-7-4.6-7-8.9V6.3z"/>',
  server: '<rect x="4" y="4.5" width="16" height="6" rx="1.6"/><rect x="4" y="13.5" width="16" height="6" rx="1.6"/><path d="M7.5 7.5h.01M7.5 16.5h.01"/>',
  key: '<circle cx="8" cy="15" r="3.6"/><path d="M10.6 12.4 19 4M16 7l2.5 2.5M13.8 9.2l2 2"/>',
  route: '<circle cx="6" cy="6" r="2.2"/><circle cx="18" cy="18" r="2.2"/><path d="M8.2 6H15a3 3 0 0 1 0 6H9a3 3 0 0 0 0 6h6.8"/>',
  book: '<path d="M4 5.5C6.5 4.5 9.5 4.5 12 6c2.5-1.5 5.5-1.5 8-.5v13c-2.5-1-5.5-1-8 .5-2.5-1.5-5.5-1.5-8-.5z"/><path d="M12 6v13.5"/>',
  stop: '<circle cx="12" cy="12" r="8.5"/><path d="M6 18L18 6"/>',
  minus: '<path d="M6 12h12"/>',
  info: '<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.5M12 7.8v.2"/>',
  pulse: '<path d="M3 12h4l2-5 4 10 2-5h6"/>',
  tout: '<path d="M7 3.5h10M7 20.5h10"/><path d="M8 3.5c0 4.6 8 4 8 8.5s-8 3.9-8 8.5M16 3.5c0 4.6-8 4-8 8.5s8 3.9 8 8.5"/>',
} as const;

export type IconName = keyof typeof P;
export type IconSize = '' | 's' | 'xs' | 'l';

export function ic(name: IconName, cls: IconSize = ''): Html {
  return html`<svg class="i ${cls}" viewBox="0 0 24 24" aria-hidden="true">${raw(P[name])}</svg>`;
}
