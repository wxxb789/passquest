import { html, type Html, type Interp } from '../core/html';
import { T } from '../core/i18n';
import { ic, type IconName } from '../icons';

/** `primaryClose`: closing is the sheet's one primary action (a read-only sheet such as annotations). */
export function sheetWrap(ico: IconName, title: string, sub: Interp, body: Interp, wide = false, primaryClose = false): Html {
  return html`<dialog class="sheet in" role="dialog" aria-modal="true" aria-labelledby="sh-t" ${
    wide ? html`style="width:min(880px, calc(100vw - 64px))"` : ''
  }>
    <div class="grab"></div>
    <header class="sh-h"><span class="ico">${ic(ico, 's')}</span><div><h2 class="h-disp" id="sh-t">${title}</h2>${
      sub ? html`<div class="sub">${sub}</div>` : ''
    }</div><button class="icon-btn" data-act="close" ${primaryClose ? 'data-primary' : ''} autofocus aria-label="${T('关闭', 'Close')}">${ic('x')}</button></header>
    <div class="sh-b">${body}</div></dialog>`;
}

/** A key/value row (.kv). */
export function kv(k: Interp, v: Interp, big = false): Html {
  return html`<div class="kv"><span>${k}</span><b class="${big ? 'big num' : ''}">${v}</b></div>`;
}
