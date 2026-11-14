// Satellite capsule on an orbit rail (prototype sat()). The tether is drawn by CSS
// (.rail.l/.rail.r .sat::after). A satellite opens the bottom sheet named by `key`.
import { html, Html, type Interp } from '../core/html';
import { ic, type IconName } from '../icons';

export type Flag = '' | 'bad' | 'warm' | 'ok';

export function sat(key: string, ico: IconName | Html, lb: string, vl: Interp, flag: Flag = '', i = 0): Html {
  const icon = ico instanceof Html ? ico : html`<span class="ico">${ic(ico, 's')}</span>`;
  return html`<button class="sat settle" style="--i:${i}" data-act="sheet" data-v="${key}" aria-haspopup="dialog">${icon}${
    flag ? html`<span class="flag ${flag}"></span>` : ''
  }<span class="tx"><span class="lb">${lb}</span><span class="vl">${vl}</span></span></button>`;
}
