// Empty, unknown and error states in the stage's own visual language: one centered object card.
import type { StudioError } from '../api/types';
import { html, type Html, type Interp } from '../core/html';
import { errorLabel, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { ic, type IconName } from '../icons';

export interface StateCard {
  ico: IconName;
  eyebrow: string;
  title: Interp;
  body: Interp;
  /** The typed error code, kept as a data attribute for tests; never shown as text. */
  code?: string | null;
  action?: Interp;
}

export function stateCard(s: StateCard): Html {
  return html`<div class="scene orbit">
    <article class="obj hero card rise" style="--i:0" aria-labelledby="state-t" data-testid="state" ${s.code ? html`data-code="${s.code}"` : ''}>
      <div class="obj-top"><span class="eyebrow">${s.eyebrow}</span><span class="r">${ic(s.ico, 'xs')}</span></div>
      <h1 class="hero-title h-disp" id="state-t" style="font-size:var(--t-2xl)">${s.title}</h1>
      <p class="reason">${s.body}</p>
      ${s.action ? html`<div class="obj-foot"><span></span><span class="acts-row">${s.action}</span></div>` : ''}
    </article>
  </div>`;
}

export function loadingCard(): Html {
  return html`<div class="scene orbit" aria-busy="true"><article class="obj hero card" aria-label="${T('加载中', 'Loading')}" data-testid="loading"><p class="reason muted">${T('正在读取本地学习记录……', 'Reading local study records…')}</p></article></div>`;
}

/** The core's message and next step for a typed failure, as one line of escaped text. */
export function errorDetail(e: StudioError): Html {
  return html`<span data-testid="error-message" lang="en">${e.message}</span>${
    e.action ? html`<br><span class="muted">${T('下一步：', 'Next: ')}<span data-testid="error-action">${e.action}</span></span>` : ''
  }`;
}

/** Error card for a failed or unavailable call. The message is core text: rendered escaped. */
export function errorCard(e: StudioError, retry: Interp = ''): Html {
  if (e.code === 'not_connected')
    return stateCard({
      ico: 'server',
      eyebrow: T('未连接', 'Not connected'),
      title: errorLabel(e.code),
      body: T('窗口没有拿到本地学习服务，因此不显示任何数字。', 'This window has no local study service, so it shows no numbers at all.'),
      code: e.code,
    });
  return stateCard({
    ico: 'info',
    eyebrow: T('无法完成', 'Could not complete'),
    title: errorLabel(e.code),
    body: errorDetail(e),
    code: e.code,
    action: retry,
  });
}

/** Renders a loadable with a ready callback; idle/loading → loading card, error → error card. */
export function withLoadable<T>(l: Loadable<T>, ready: (v: T) => Html, retry: Interp = ''): Html {
  if (l.status === 'ready') return ready(l.value);
  if (l.status === 'error') return errorCard(l.error, retry);
  return loadingCard();
}

export function retryButton(act: string): Html {
  return html`<button class="btn ghost" data-act="${act}">${T('重试', 'Retry')}</button>`;
}
