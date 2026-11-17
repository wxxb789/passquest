// The coach sheet that grows from the orb on Feedback (prototype coachHTML). Every send is
// previewed first: the call strip names local vs external, provider and model, the exact request,
// the consent state and the budget slot. Replies are advisory model text, rendered as text only,
// and never change the grade. The history view lists this attempt's earlier calls, replies as text.
import type { CoachCall, CoachHistory, CoachMode, CoachPreview, CoachReply, ModelList, StudioError } from '../api/types';
import { html, type Html } from '../core/html';
import { COACH_MODES, errorLabel, langOf, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { callStrip, callTarget, providerName } from '../components/callstrip';
import { failureNote, rawDetail } from '../components/failure';
import { ic } from '../icons';

export interface CoachData {
  preview: Loadable<CoachPreview>;
  replies: Partial<Record<CoachMode, CoachReply>>;
  failed: CoachReply | null;
  failure: StudioError | null;
  sending: boolean;
  busy: boolean;
  models: ModelList | null;
  /** The attempt's recorded calls; shown instead of the mode body while `showHistory`. */
  history: Loadable<CoachHistory>;
  showHistory: boolean;
}

/** Splits advisory text into paragraphs; each is inserted as escaped text. */
function replyParagraphs(text: string): Html[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => html`<p>${p.split('\n').map((line, i) => html`${i ? html`<br>` : ''}${line}`)}</p>`);
}

/**
 * Q-0011-09: one aggregated line when no provider can answer, counted from the registry's facts.
 * `inSettings`: the line is shown on Settings itself, so it points at the rows below.
 */
export function unusableLine(m: ModelList, inSettings = false): string | null {
  if (m.usable) return null;
  let missing = 0;
  let unverified = 0;
  for (const p of m.providers) {
    const verified = p.models.some((x) => x.capabilities.some((c) => c.id === 'text.chat' && (c.verified || c.state === 'legacy')));
    if (p.auth !== 'present') missing++;
    else if (!verified) unverified++;
  }
  const facts = T(
    `还没有提供方能回答：${missing} 个没有密钥，${unverified} 个有密钥但没有通过验证的对话模型。`,
    `No provider can answer yet: without a key ${missing}, with a key but no verified chat model ${unverified}.`,
  );
  return (
    facts +
    (inSettings
      ? T('在下方为一个提供方添加密钥，再测试它的模型。', ' Add a key to a provider below, then test its model.')
      : T('打开设置添加密钥并测试模型。', ' Open Settings to add a key and test a model.'))
  );
}

/** Retry clears a send's failure and reloads the preview; it never sends by itself. */
const retryButton = (busy: boolean): Html =>
  html`<button class="btn quiet sm" data-act="coach-retry" ${busy ? 'disabled' : ''}>${T('重试', 'Retry')}</button>`;

/** Who answered: the registry's provider name and the model id (names, not translated). */
export function whoMeta(provider: string, model: string, models: ModelList | null): Html {
  return html`<span translate="no">${providerName(models, provider)} · ${model}</span>`;
}

function body(d: CoachData, mode: CoachMode, unusable: string | null): Html {
  const reply = d.replies[mode];
  if (reply?.state === 'advisory')
    return html`<div class="reply" lang="${langOf(reply.text)}" data-testid="coach-reply" data-call="${reply.call_id}">${replyParagraphs(reply.text)}
        <div class="rmeta">${ic('adv', 'xs')}<span>${T('建议性 · 不改变评分', 'Advisory · never grades')}</span><span class="sep"></span>${whoMeta(reply.provider, reply.model, d.models)}${
          reply.failover?.applied
            ? html`<span class="sep"></span><span>${T('由', 'failover from')} <span translate="no">${providerName(d.models, reply.failover.from_provider)}</span>${T(' 转移', '')}</span>`
            : ''
        }<span class="sep"></span><span class="num nw">${(reply.latency_ms / 1000).toFixed(1)} s</span></div></div>`;
  const failed = d.failed?.failure;
  if (failed)
    return failureNote(failed, { testid: 'coach-failure', title: T('这次调用没有成功，评分不变。', 'The call did not succeed; the grade is unchanged.'), retry: retryButton(d.busy) });
  if (d.failure) return failureNote(d.failure, { testid: 'coach-failure', retry: retryButton(d.busy) });
  const p = d.preview;
  if (p.status === 'error') {
    // With no usable provider the aggregated line above already says what to do.
    if (unusable && (p.error.code === 'not_configured' || p.error.code === 'no_credential')) return html``;
    return failureNote(p.error, { testid: 'coach-failure' });
  }
  if (p.status !== 'ready') return html`<div class="pending">${T('正在准备发送预览……', 'Preparing the send preview…')}</div>`;
  const v = p.value;
  // A hand-configured endpoint has no registry provider id; it is configured all the same.
  if (!v.endpoint) return failureNote({ code: 'not_configured', message: '', action: null }, { testid: 'coach-failure' });
  const where = v.external ? T('外部调用', 'external call') : T('本地调用', 'local call');
  return html`<div class="pending">${T(
    `这一模式还没有回复。发送将进行一次${where}，消耗 1 次预算`,
    `No reply in this mode yet. Sending makes one ${where} and spends 1 budget slot`,
  )} <span class="num nw">(${v.budget.calls} → ${v.budget.calls + 1} / ${v.budget.limit})</span>${T('。', '.')}
        <div><button class="btn adv sm" data-act="coach-send" ${d.sending || d.busy ? 'disabled' : ''}>${ic(v.external ? 'ext' : 'local', 'xs')}${
          d.sending ? T('发送中……', 'Sending…') : html`${T('发送到 ', 'Send to ')}<span translate="no">${callTarget(d.models, v)}</span>`
        }</button></div></div>`;
}

const modeName = (m: string): string => {
  const k = COACH_MODES.find(([id]) => id === m);
  return k ? T(k[1], k[2]) : m;
};

/** The local clock time of a recorded call (`HH:MM`), or the raw stamp when it does not parse. */
function callTime(at: string): string {
  const t = new Date(at);
  return Number.isNaN(t.getTime()) ? at : `${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}`;
}

function historyCall(c: CoachCall, models: ModelList | null): Html {
  const failed = c.state !== 'advisory';
  const state = failed
    ? html`<span class="chip warm sm">${T('未成功', 'Failed')}</span>`
    : html`<span class="chip adv sm">${ic('adv', 'xs')}${T('建议性', 'Advisory')}</span>`;
  const bodyOf = failed
    ? html`<p class="muted">${c.failure ? errorLabel(c.failure.code) : T('原因未记录', 'No reason recorded')}</p>${c.failure ? rawDetail(c.failure) : ''}`
    : html`<div lang="${langOf(c.text)}">${replyParagraphs(c.text)}</div>`;
  return html`<li class="hcall ${failed ? 'failed' : ''}" data-testid="coach-call" data-call="${c.call_id}" data-state="${c.state}" data-mode="${c.mode}">
      <div class="hmeta"><b>${modeName(c.mode)}</b><span class="sep"></span>${whoMeta(c.provider, c.model, models)}<span class="sep"></span><time class="num" datetime="${c.created_at}">${callTime(c.created_at)}</time>${state}</div>
      ${bodyOf}
    </li>`;
}

function historyBody(h: Loadable<CoachHistory>, models: ModelList | null): Html {
  if (h.status === 'error') return failureNote(h.error, { testid: 'coach-failure' });
  if (h.status !== 'ready') return html`<div class="pending">${T('正在读取这次作答的调用记录……', 'Reading the calls of this attempt…')}</div>`;
  const calls = h.value.calls;
  return html`<section class="hist" data-testid="coach-history" data-count="${calls.length}" aria-label="${T('这次作答的调用', 'Calls of this attempt')}">${
    calls.length
      ? html`<ol>${calls.map((c) => historyCall(c, models))}</ol>`
      : html`<p class="fine">${T('这次作答还没有调用过导师。', 'No coach call for this attempt yet.')}</p>`
  }<p class="fine">${T('都是建议性文字，不改变评分。', 'All advisory text; none changes the grade.')}</p></section>`;
}

export function coachHTML(d: CoachData, mode: CoachMode): Html {
  const unusable = d.models ? unusableLine(d.models) : null;
  return html`<aside class="csheet" id="coach" tabindex="-1" aria-label="${T('导师 · 建议性', 'Coach · advisory')}">
    <div class="co-head"><span class="o"></span><span class="t">${T('导师', 'Coach')}</span><span class="chip adv sm">${ic('adv', 'xs')}${T('建议性 · 不改变评分', 'Advisory · never grades')}</span><button class="icon-btn" data-act="coach-history" aria-pressed="${d.showHistory}" aria-label="${T('调用记录', 'Call history')}" title="${T('调用记录', 'Call history')}">${ic('clock', 's')}</button><button class="icon-btn" data-act="coach" aria-label="${T('收起导师', 'Collapse coach')}">${ic('chev', 's')}</button></div>
    <div class="modes" role="group" aria-label="${T('模式', 'Mode')}">
      ${COACH_MODES.map(
        ([k, z, e, n]) => html`<button data-act="mode" data-v="${k}" aria-pressed="${mode === k}" aria-keyshortcuts="${n}">${T(z, e)}<kbd aria-hidden="true">${n}</kbd></button>`,
      )}
    </div>
    <div class="co-body">
      <div class="co-scroll" id="coscroll">
        ${
          d.showHistory
            ? historyBody(d.history, d.models)
            : html`${unusable ? html`<div class="pending held" data-testid="providers-unusable">${unusable} <a class="btn quiet sm" href="#/settings">${T('打开设置', 'Open Settings')}</a></div>` : ''}
        ${d.preview.status === 'ready' && d.preview.value.endpoint ? callStrip(d.preview.value, d.busy, d.models) : ''}
        ${body(d, mode, unusable)}`
        }
      </div>
      <button class="more" id="more" data-act="more" hidden>${T('更多', 'More')}${ic('down', 'xs')}</button>
    </div>
  </aside>`;
}
