// The call ledger shown before any provider call (prototype .callstrip): local vs external, provider
// and model with its verification, the exact request (size, hash, every message as text), the
// consent state of the source with grant/revoke in place, the credential presence and the budget slot.
import type { CoachConsent, CoachPreview } from '../api/types';
import { html, type Html } from '../core/html';
import { T } from '../core/i18n';
import { ic } from '../icons';

function consentLine(c: CoachConsent, provider: string, busy: boolean): Html {
  if (!c.private_source) return html`<span data-consent="first_party">${T('第一方内容 · 无需授权', 'First-party content · no consent needed')}</span>`;
  const src = c.source_id ?? '—';
  const v = `${c.source_id ?? ''}|${provider}`;
  const d = busy ? 'disabled' : '';
  if (c.granted)
    return html`<span data-consent="granted">${T(`${src} 已授权给 ${provider}`, `${src} granted to ${provider}`)} <span class="muted">· ${T('可撤回', 'revocable')}</span>
      <button class="btn quiet sm" data-act="consent-revoke" data-v="${v}" ${d}>${T('撤回', 'Revoke')}</button></span>`;
  return html`<span data-consent="withheld"><span class="held">${T(`${src} 未授权给 ${provider} · 原文已扣留`, `${src} not granted to ${provider} · text withheld`)}</span>
      <button class="btn ghost sm" data-act="consent-grant" data-v="${v}" ${d}>${T('授权发送原文', 'Grant consent')}</button></span>`;
}

function payload(p: CoachPreview): Html {
  const withheld = p.consent.withheld.length
    ? html` <span class="held">· ${T('扣留', 'withheld')}: ${p.consent.withheld.join(', ')}</span>`
    : '';
  return html`<span>${T(`${p.messages.length} 条消息`, `${p.messages.length} messages`)} · <span class="num">${p.request_bytes}</span> B · <code title="${p.request_sha256}">sha256 ${p.request_sha256.slice(0, 12)}</code>${withheld}</span>
    <details class="payload" data-testid="payload"><summary>${T('查看将发送的完整内容', 'Show the exact request')}</summary>${p.messages.map(
      (m) => html`<div class="msg"><b>${m.role}</b><pre lang="en">${m.content}</pre></div>`,
    )}</details>`;
}

export function callStrip(p: CoachPreview, busy: boolean): Html {
  const where = p.external
    ? html`<span class="ext">${ic('ext', 'xs')}${T('外部', 'External')}</span>`
    : html`<span>${ic('local', 'xs')}${T('本地', 'Local')}</span>`;
  const verified = p.verified
    ? html`<span class="nw" style="color:var(--ok)">${T('已验证', 'verified')}</span>`
    : html`<span class="nw held">${T('未验证', 'not verified')}</span>`;
  const cred = p.credential.present
    ? html`<span data-credential="present">${ic('key', 'xs')}${T('有密钥', 'key present')}</span>`
    : html`<span class="held" data-credential="missing">${ic('key', 'xs')}${T('无密钥', 'no key')}</span>`;
  return html`<div class="callstrip" role="group" aria-label="${T('这次调用', 'This call')}" data-testid="call-strip" data-external="${p.external}" data-sha="${p.request_sha256}" data-bytes="${p.request_bytes}">
    <span class="lk">${T('调用', 'Call')}</span><span>${where} · ${p.provider} · <code class="nw">${p.model}</code> · ${verified} · ${cred}</span>
    <span class="lk">${T('发送', 'Sends')}</span>${payload(p)}
    <span class="lk">${T('授权', 'Consent')}</span>${consentLine(p.consent, p.provider, busy)}
    <span class="lk">${T('预算', 'Budget')}</span><span><b class="num" style="font-weight:600">${p.budget.calls} / ${p.budget.limit}</b> <span class="muted">${T(`今日 · ${p.budget.label || p.provider}`, `today · ${p.budget.label || p.provider}`)}</span></span>
  </div>`;
}
