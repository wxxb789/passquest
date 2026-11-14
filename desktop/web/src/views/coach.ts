// The coach sheet that grows from the orb on Feedback (prototype coachHTML). Every send is
// previewed first: the call strip names local vs external, provider and model, what is sent, the
// consent state and the budget slot. Replies are advisory model text and are rendered as text.
import type { CoachMode, CoachPreview, CoachReply, ConsentState, StudioError } from '../api/types';
import { html, type Html } from '../core/html';
import { COACH_MODES, shortStamp, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { callStrip } from '../components/callstrip';
import { ic } from '../icons';

export interface CoachData {
  preview: Loadable<CoachPreview>;
  replies: Partial<Record<CoachMode, CoachReply>>;
  failure: StudioError | null;
  sending: boolean;
  /** Aggregated "every provider is unusable" line (Q-0011-09). */
  unusable: StudioError | null;
}

/** Splits advisory text into paragraphs; each is inserted as escaped text. */
function replyParagraphs(text: string): Html[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => html`<p>${p.split('\n').map((line, i) => html`${i ? html`<br>` : ''}${line}`)}</p>`);
}

function actionLink(e: StudioError, consent?: ConsentState): Html {
  if (e.code === 'no_consent')
    return consent?.source_id
      ? html`<button class="btn quiet sm" data-act="consent-ask" data-v="${consent.source_id}|${consent.provider}">${T('去设置里授权', 'Grant in Settings')}${ic('arrow', 'xs')}</button>`
      : html`<a class="btn quiet sm" href="#/settings/consent">${T('去设置里授权', 'Grant in Settings')}${ic('arrow', 'xs')}</a>`;
  if (e.code === 'no_credential' || e.code === 'provider_unusable')
    return html`<a class="btn quiet sm" href="#/settings">${T('打开设置', 'Open Settings')}${ic('arrow', 'xs')}</a>`;
  return e.action ? html`<span class="muted"><code>${e.action}</code></span>` : html``;
}

function held(e: StudioError, lead: string, consent?: ConsentState): Html {
  return html`<div class="pending held" data-code="${e.code}">${lead} ${e.message}
        <div style="display:flex;gap:8px;flex-wrap:wrap"><button class="btn adv sm" disabled>${ic('ext', 'xs')}${T('发送', 'Send')}</button>${actionLink(e, consent)}</div></div>`;
}

function body(d: CoachData, mode: CoachMode, lang: string): Html {
  const reply = d.replies[mode];
  if (reply)
    return html`<div class="reply" lang="${lang}">${replyParagraphs(reply.text)}
        <div class="rmeta">${ic('adv', 'xs')}${T('建议性', 'Advisory')}<span class="sep"></span>${reply.model}${
          reply.failover_from ? html`<span class="sep"></span>${T(`由 ${reply.failover_from} 转移`, `failover from ${reply.failover_from}`)}` : ''
        }<span class="sep"></span><span class="num nw">${shortStamp(reply.at)}</span></div></div>`;
  if (d.failure) return held(d.failure, T('这次调用没有成功，评分不变。', 'The call did not succeed; the grade is unchanged.'));
  if (d.unusable) return held(d.unusable, '');
  const p = d.preview;
  if (p.status === 'error') return held(p.error, '');
  if (p.status !== 'ready') return html`<div class="pending">${T('正在准备发送预览……', 'Preparing the send preview…')}</div>`;
  const v = p.value;
  if (v.blocked) return held(v.blocked, T('这一模式还没有回复。', 'No reply in this mode yet.'), v.consent);
  const where = v.external ? T('外部调用', 'external call') : T('本地调用', 'local call');
  return html`<div class="pending">${T(
    `这一模式还没有回复。发送将进行一次${where}，消耗 1 次预算`,
    `No reply in this mode yet. Sending makes one ${where} and spends 1 budget slot`,
  )} <span class="num nw">(${v.budget.used} → ${v.budget.used + 1} / ${v.budget.limit})</span>${T('。', '.')}
        <div><button class="btn adv sm" data-act="coach-send" ${d.sending ? 'disabled' : ''}>${ic(v.external ? 'ext' : 'local', 'xs')}${
          d.sending ? T('发送中……', 'Sending…') : T(`发送到 ${v.provider_name}`, `Send to ${v.provider_name}`)
        }</button></div></div>`;
}

export function coachHTML(d: CoachData, mode: CoachMode, lang: 'zh' | 'en'): Html {
  return html`<aside class="csheet" id="coach" aria-label="${T('导师 · 建议性', 'Coach · advisory')}">
    <div class="co-head"><span class="o"></span><span class="t">${T('导师', 'Coach')}</span><span class="chip adv sm">${ic('adv', 'xs')}${T('建议性 · 不改变评分', 'Advisory · never grades')}</span><button class="icon-btn" data-act="coach" aria-label="${T('收起导师', 'Collapse coach')}">${ic('chev', 's')}</button></div>
    <div class="modes" role="group" aria-label="${T('模式', 'Mode')}">
      ${COACH_MODES.map(
        ([k, z, e, n]) => html`<button data-act="mode" data-v="${k}" aria-pressed="${mode === k}" aria-keyshortcuts="${n}">${T(z, e)}<kbd aria-hidden="true">${n}</kbd></button>`,
      )}
    </div>
    <div class="co-body">
      <div class="co-scroll" id="coscroll">
        ${d.preview.status === 'ready' ? callStrip(d.preview.value) : ''}
        ${body(d, mode, lang === 'zh' ? 'zh-CN' : 'en')}
      </div>
      <button class="more" id="more" data-act="more" hidden>${T('更多', 'More')}${ic('down', 'xs')}</button>
    </div>
  </aside>`;
}
