// The call ledger shown before any provider call (prototype .callstrip): local vs external, provider
// and model with its verification, the exact request (size, every message as text), the consent
// state of the source with grant/revoke in place, the credential presence and the budget slot.
// The coach preview and the Anki refinement preview share it. The request's sha256 stays on the
// strip as data (and in the request disclosure's tooltip); it is not a sentence for the learner.
// The consent and budget rows are marked data-must-see: they are shown before the send, never
// scrolled away inside the strip or its panel (desktop-ux checks it).
import type { CoachConsent, CoachPreview, ModelList } from '../api/types';
import { html, type Html } from '../core/html';
import { T } from '../core/i18n';
import { ic } from '../icons';

/** The registry's display name of a provider id; the id itself when the registry is not loaded. */
export function providerName(models: ModelList | null, id: string): string {
  return models?.providers.find((p) => p.id === id)?.name || id;
}

/**
 * Who a previewed call goes to: the registry provider's name, or for a hand-configured endpoint
 * (`tutor config --endpoint`, which has no registry provider id) the endpoint's host.
 */
export function callTarget(models: ModelList | null, p: Pick<CallPreview, 'provider' | 'endpoint'>): string {
  if (p.provider) return providerName(models, p.provider);
  try {
    return new URL(p.endpoint).host || T('手动配置的端点', 'manual endpoint');
  } catch {
    return T('手动配置的端点', 'manual endpoint');
  }
}

function consentLine(c: CoachConsent, provider: string, providerId: string, busy: boolean): Html {
  if (!c.private_source) return html`<span data-consent="first_party">${T('第一方内容 · 无需授权', 'First-party content · no consent needed')}</span>`;
  const v = `${c.source_id ?? ''}|${providerId}`;
  const d = busy ? 'disabled' : '';
  // A manual endpoint has no provider id: its consent is the every-provider decision, so say so.
  const to = providerId ? html`<span translate="no">${provider}</span>` : html`${T('所有提供方', 'every provider')}`;
  if (c.granted)
    return html`<span data-consent="granted">${T('这份导入资料已授权给 ', 'This imported source is granted to ')}${to} <span class="muted">· ${T('可撤回', 'revocable')}</span>
      <button class="btn quiet sm" data-act="consent-revoke" data-v="${v}" ${d}>${T('撤回', 'Revoke')}</button></span>`;
  return html`<span data-consent="withheld"><span class="held">${T('这份导入资料未授权给 ', 'This imported source is not granted to ')}${to}${T(' · 原文已扣留', ' · its text is withheld')}</span>
      <button class="btn ghost sm" data-act="consent-grant" data-v="${v}" ${d}>${providerId ? T('授权发送原文', 'Grant consent') : T('授权原文（所有提供方）', 'Allow the text (every provider)')}</button></span>`;
}

/** What the strip reads: the fields CoachPreview and NoteRefinePreview have in common. */
export type CallPreview = Pick<
  CoachPreview,
  'provider' | 'model' | 'endpoint' | 'external' | 'verified' | 'credential' | 'consent' | 'messages' | 'request_sha256' | 'request_bytes' | 'budget'
>;

/** Bytes in the learner's unit: B under 1 KB, else KB with one decimal. */
const size = (bytes: number): string => (bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KB`);

const WITHHELD: Record<string, readonly [string, string]> = {
  passage: ['文章', 'passage'],
  question_stem: ['题干', 'stem'],
  options: ['选项', 'options'],
  evidence: ['证据句', 'evidence'],
  reasoning: ['解析', 'reasoning'],
  selected_option_text: ['所选选项原文', 'chosen option text'],
  correct_option_text: ['答案选项原文', 'key option text'],
};

function payload(p: CallPreview): Html {
  const withheld = p.consent.withheld.length
    ? html` <span class="held">· ${T('扣留：', 'withheld: ')}${p.consent.withheld.map((w) => (WITHHELD[w] ? T(WITHHELD[w][0], WITHHELD[w][1]) : w)).join(T('、', ', '))}</span>`
    : '';
  return html`<span>${T(`${p.messages.length} 条消息`, `${p.messages.length} messages`)} · <span class="num">${size(p.request_bytes)}</span>${withheld}</span>
    <details class="payload" data-testid="payload"><summary title="sha256 ${p.request_sha256}">${T('查看将发送的完整内容', 'Show the exact request')}</summary>${p.messages.map(
      (m) => html`<div class="msg"><b translate="no">${m.role}</b><pre lang="en" data-raw>${m.content}</pre></div>`,
    )}</details>`;
}

/**
 * `models` names the provider by its registry name. `consent` replaces the grant/revoke line when the
 * caller needs another decision (a note's every-provider grant).
 */
export function callStrip(p: CallPreview, busy: boolean, models: ModelList | null, testid = 'call-strip', consent?: Html): Html {
  const name = callTarget(models, p);
  const where = p.external
    ? html`<span class="ext">${ic('ext', 'xs')}${T('外部', 'External')}</span>`
    : html`<span>${ic('local', 'xs')}${T('本地', 'Local')}</span>`;
  const verified = p.verified
    ? html`<span class="nw" style="color:var(--ok)">${T('已验证', 'verified')}</span>`
    : html`<span class="nw held">${T('未验证', 'not verified')}</span>`;
  const cred = p.credential.present
    ? html`<span class="nw" data-credential="present">${ic('key', 'xs')}${T('有密钥', 'key present')}</span>`
    : html`<span class="nw held" data-credential="missing">${ic('key', 'xs')}${T('无密钥', 'no key')}</span>`;
  return html`<div class="callstrip" role="group" aria-label="${T('这次调用', 'This call')}" data-testid="${testid}" data-external="${p.external}" data-sha="${p.request_sha256}" data-bytes="${p.request_bytes}">
    <span class="lk">${T('调用', 'Call')}</span><span>${where} · <span translate="no">${name}</span> · <code class="nw" translate="no">${p.model}</code> · ${verified} · ${cred}</span>
    <span class="lk">${T('发送', 'Sends')}</span>${payload(p)}
    <span class="lk" data-must-see>${T('授权', 'Consent')}</span><span data-must-see>${consent ?? consentLine(p.consent, name, p.provider, busy)}</span>
    <span class="lk" data-must-see>${T('预算', 'Budget')}</span><span data-must-see><b class="num" style="font-weight:600">${p.budget.calls} / ${p.budget.limit}</b> <span class="muted">${T('今日 · ', 'today · ')}<span translate="no">${p.budget.label || name}</span></span></span>
  </div>`;
}
