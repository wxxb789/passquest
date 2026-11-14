// The coach sheet that grows from the orb on Feedback (prototype coachHTML). Every send is
// previewed first: the call strip names local vs external, provider and model, the exact request,
// the consent state and the budget slot. Replies are advisory model text, rendered as text only,
// and never change the grade.
import type { CoachMode, CoachPreview, CoachReply, ModelList, StudioError } from '../api/types';
import { html, type Html } from '../core/html';
import { COACH_MODES, errorTitle, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { callStrip } from '../components/callstrip';
import { ic } from '../icons';

export interface CoachData {
  preview: Loadable<CoachPreview>;
  replies: Partial<Record<CoachMode, CoachReply>>;
  failure: StudioError | null;
  sending: boolean;
  busy: boolean;
  models: ModelList | null;
}

/** Splits advisory text into paragraphs; each is inserted as escaped text. */
function replyParagraphs(text: string): Html[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => html`<p>${p.split('\n').map((line, i) => html`${i ? html`<br>` : ''}${line}`)}</p>`);
}

/** Q-0011-09: one aggregated line when no provider can answer, counted from the registry's facts. */
export function unusableLine(m: ModelList): string | null {
  if (m.usable) return null;
  let missing = 0;
  let unverified = 0;
  for (const p of m.providers) {
    const verified = p.models.some((x) => x.capabilities.some((c) => c.id === 'text.chat' && (c.verified || c.state === 'legacy')));
    if (p.auth !== 'present') missing++;
    else if (!verified) unverified++;
  }
  return T(
    `所有提供方都不能回答：${missing} 个没有密钥，${unverified} 个没有已验证的 text.chat 模型。先在设置里添加密钥，再测试模型。`,
    `No provider can answer: ${missing} have no credential and ${unverified} have no verified text.chat model. Add a key in Settings, then test the model.`,
  );
}

function settingsLink(code: string): Html {
  if (['no_credential', 'not_configured', 'capability_unverified', 'budget_exhausted'].includes(code))
    return html`<a class="btn quiet sm" href="#/settings">${T('打开设置', 'Open Settings')}${ic('arrow', 'xs')}</a>`;
  return html``;
}

function held(code: string, title: string, message: string, action: string | null | undefined): Html {
  return html`<div class="pending held" data-testid="coach-failure" data-code="${code}"><b>${title}</b> <span class="muted" lang="en">${message}</span>${
    action ? html`<div class="muted">${T('下一步：', 'Next: ')}${action}</div>` : ''
  }
        <div style="display:flex;gap:8px;flex-wrap:wrap">${settingsLink(code)}</div></div>`;
}

function body(d: CoachData, mode: CoachMode, lang: string, unusable: string | null): Html {
  const reply = d.replies[mode];
  if (reply?.state === 'advisory')
    return html`<div class="reply" lang="${lang}" data-testid="coach-reply" data-call="${reply.call_id}">${replyParagraphs(reply.text)}
        <div class="rmeta">${ic('adv', 'xs')}${T('建议性 · 不改变评分', 'Advisory · never grades')}<span class="sep"></span>${reply.provider} · ${reply.model}${
          reply.failover?.applied ? html`<span class="sep"></span>${T(`由 ${reply.failover.from_provider} 转移`, `failover from ${reply.failover.from_provider}`)}` : ''
        }<span class="sep"></span><span class="num nw">${(reply.latency_ms / 1000).toFixed(1)} s</span></div></div>`;
  if (reply?.state === 'failed' && reply.failure)
    return held(reply.failure.code, T('这次调用没有成功，评分不变。', 'The call did not succeed; the grade is unchanged.'), reply.failure.message, reply.failure.action);
  if (d.failure) return held(d.failure.code, errorTitle(d.failure.code), d.failure.message, d.failure.action);
  const p = d.preview;
  if (p.status === 'error') {
    // With no usable provider the aggregated line above already says what to do.
    if (unusable && (p.error.code === 'not_configured' || p.error.code === 'no_credential')) return html``;
    return held(p.error.code, errorTitle(p.error.code), p.error.message, p.error.action);
  }
  if (p.status !== 'ready') return html`<div class="pending">${T('正在准备发送预览……', 'Preparing the send preview…')}</div>`;
  const v = p.value;
  if (!v.provider) return held('not_configured', errorTitle('not_configured'), T('先在设置里选择一个模型。', 'Choose a model in Settings first.'), null);
  const where = v.external ? T('外部调用', 'external call') : T('本地调用', 'local call');
  return html`<div class="pending">${T(
    `这一模式还没有回复。发送将进行一次${where}，消耗 1 次预算`,
    `No reply in this mode yet. Sending makes one ${where} and spends 1 budget slot`,
  )} <span class="num nw">(${v.budget.calls} → ${v.budget.calls + 1} / ${v.budget.limit})</span>${T('。', '.')}
        <div><button class="btn adv sm" data-act="coach-send" ${d.sending || d.busy ? 'disabled' : ''}>${ic(v.external ? 'ext' : 'local', 'xs')}${
          d.sending ? T('发送中……', 'Sending…') : T(`发送到 ${v.provider}`, `Send to ${v.provider}`)
        }</button></div></div>`;
}

export function coachHTML(d: CoachData, mode: CoachMode, lang: 'zh' | 'en'): Html {
  const unusable = d.models ? unusableLine(d.models) : null;
  return html`<aside class="csheet" id="coach" aria-label="${T('导师 · 建议性', 'Coach · advisory')}">
    <div class="co-head"><span class="o"></span><span class="t">${T('导师', 'Coach')}</span><span class="chip adv sm">${ic('adv', 'xs')}${T('建议性 · 不改变评分', 'Advisory · never grades')}</span><button class="icon-btn" data-act="coach" aria-label="${T('收起导师', 'Collapse coach')}">${ic('chev', 's')}</button></div>
    <div class="modes" role="group" aria-label="${T('模式', 'Mode')}">
      ${COACH_MODES.map(
        ([k, z, e, n]) => html`<button data-act="mode" data-v="${k}" aria-pressed="${mode === k}" aria-keyshortcuts="${n}">${T(z, e)}<kbd aria-hidden="true">${n}</kbd></button>`,
      )}
    </div>
    <div class="co-body">
      <div class="co-scroll" id="coscroll">
        ${unusable ? html`<div class="pending held" data-testid="providers-unusable">${unusable} <a class="btn quiet sm" href="#/settings">${T('打开设置', 'Open Settings')}</a></div>` : ''}
        ${d.preview.status === 'ready' && d.preview.value.provider ? callStrip(d.preview.value, d.busy) : ''}
        ${body(d, mode, lang === 'zh' ? 'zh-CN' : 'en', unusable)}
      </div>
      <button class="more" id="more" data-act="more" hidden>${T('更多', 'More')}${ic('down', 'xs')}</button>
    </div>
  </aside>`;
}
