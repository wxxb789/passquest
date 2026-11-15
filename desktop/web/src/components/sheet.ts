// Bottom sheet shell (prototype sheetWrap). The scrim and the close button carry data-act="close".
import { html, type Html, type Interp } from '../core/html';
import { T } from '../core/i18n';
import { ic, type IconName } from '../icons';

export function sheetWrap(ico: IconName, title: string, sub: Interp, body: Interp, wide = false): Html {
  return html`<div class="scrim" data-act="close"></div><section class="sheet in" role="dialog" aria-modal="true" aria-labelledby="sh-t" ${
    wide ? html`style="width:min(880px, calc(100vw - 64px))"` : ''
  }>
    <div class="grab"></div>
    <header class="sh-h"><span class="ico">${ic(ico, 's')}</span><div><h2 class="h-disp" id="sh-t">${title}</h2>${
      sub ? html`<div class="sub">${sub}</div>` : ''
    }</div><button class="icon-btn" data-act="close" aria-label="${T('关闭', 'Close')}">${ic('x')}</button></header>
    <div class="sh-b">${body}</div></section>`;
}

/** A key/value row (.kv). */
export function kv(k: Interp, v: Interp, big = false): Html {
  return html`<div class="kv"><span>${k}</span><b class="${big ? 'big num' : ''}">${v}</b></div>`;
}
