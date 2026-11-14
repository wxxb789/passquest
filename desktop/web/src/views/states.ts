// Empty, unknown and error states in the stage's own visual language: one centered object card.
import type { StudioError } from '../api/types';
import { html, type Html, type Interp } from '../core/html';
import { T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { ic, type IconName } from '../icons';

export interface StateCard {
  ico: IconName;
  eyebrow: string;
  title: string;
  body: Interp;
  code?: string | null;
  action?: Interp;
}

export function stateCard(s: StateCard): Html {
  return html`<div class="scene orbit">
    <article class="obj hero card rise" style="--i:0" aria-labelledby="state-t">
      <div class="obj-top"><span class="eyebrow">${s.eyebrow}</span>${s.code ? html`<span class="r">${ic(s.ico, 'xs')}<span class="id">${s.code}</span></span>` : ''}</div>
      <h1 class="hero-title h-disp" id="state-t" style="font-size:var(--t-2xl)">${s.title}</h1>
      <p class="reason">${s.body}</p>
      ${s.action ? html`<div class="obj-foot"><span></span>${s.action}</div>` : ''}
    </article>
  </div>`;
}

export function loadingCard(): Html {
  return html`<div class="scene orbit" aria-busy="true"><article class="obj hero card" aria-label="${T('加载中', 'Loading')}"><p class="reason muted">${T('正在读取本地学习记录……', 'Reading local study records…')}</p></article></div>`;
}

/** Error card for a failed or unavailable call. The message is core text: rendered escaped. */
export function errorCard(e: StudioError, retry: Interp = ''): Html {
  if (e.code === 'not_connected')
    return stateCard({
      ico: 'server',
      eyebrow: T('未连接', 'Not connected'),
      title: T('学习服务未连接', 'The study service is not connected'),
      body: T('窗口没有拿到本地学习服务，因此不显示任何数字。', 'This window has no local study service, so it shows no numbers at all.'),
      code: e.code,
    });
  return stateCard({
    ico: 'info',
    eyebrow: T('无法完成', 'Could not complete'),
    title: errorTitle(e),
    body: html`${e.message}${e.action ? html`<br><span class="muted">${T('下一步：', 'Next: ')}<code>${e.action}</code></span>` : ''}`,
    code: e.code,
    action: retry,
  });
}

export function errorTitle(e: StudioError): string {
  switch (e.code) {
    case 'assessment_active':
      return T('测评进行中', 'An assessment is in progress');
    case 'pool_floor':
      return T('未见测评池不足', 'The unseen pool is too small');
    case 'no_credential':
      return T('没有密钥', 'No credential');
    case 'no_consent':
      return T('未授权', 'No consent');
    case 'node_unavailable':
      return T('本地 Node 不可用', 'Node is not available');
    case 'anki_unreachable':
      return T('无法连接 Anki', 'Anki is unreachable');
    case 'budget_exhausted':
      return T('今日预算已用完', "Today's budget is spent");
    default:
      return T('出现问题', 'Something went wrong');
  }
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
