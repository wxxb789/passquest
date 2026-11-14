// The call ledger shown before any provider call (prototype .callstrip): local vs external, provider
// and model, what is sent, the consent state and the budget slot.
import type { CoachPreview, ConsentState } from '../api/types';
import { html, type Html } from '../core/html';
import { monthDay, T } from '../core/i18n';
import { ic } from '../icons';

export function consentLine(c: ConsentState, providerName: string): Html {
  if (!c.required) return html`${T('第一方内容 · 无需授权', 'First-party content · no consent needed')}`;
  const src = c.source_id ?? '—';
  if (c.granted)
    return html`${T(`${src} 已授权 ${providerName}`, `${src} granted to ${providerName}`)} <span class="muted">· ${T('于', 'on')} <span class="nw">${monthDay(c.changed_at)}</span> · ${T('可撤回', 'revocable')}</span>`;
  return html`<span class="held">${T(`${src} 未授权 ${providerName}`, `${src} not granted to ${providerName}`)}</span>${
    c.changed_at ? html` <span class="muted">· ${T('撤回记录', 'revocation logged')} <span class="nw">${monthDay(c.changed_at)}</span></span>` : ''
  }`;
}

export function payloadLine(p: CoachPreview): Html {
  const withheld = p.consent.required && !p.consent.granted;
  const src = p.consent.source_id;
  if (withheld)
    return html`<span class="held">${T(`${src ?? ''} 原文扣留：未授权`, `${src ?? ''} text withheld — no consent`)}</span>${
      p.consent.withheld.length ? html` <span class="muted">· ${p.consent.withheld.join(', ')}</span>` : ''
    }`;
  return html`${p.payload_summary.join(', ')}${src ? html` <span class="muted">— ${T('来源', 'source')} ${src}</span>` : ''}`;
}

export function callStrip(p: CoachPreview): Html {
  const where = p.external
    ? html`<span class="ext">${ic('ext', 'xs')}${T('外部', 'External')}</span>`
    : html`<span>${ic('local', 'xs')}${T('本地', 'Local')}</span>`;
  const verified = p.verified
    ? html`<span class="nw" style="color:var(--ok)">${T('已验证', 'verified')}</span>`
    : html`<span class="nw held">${T('未验证', 'not verified')}</span>`;
  return html`<div class="callstrip" role="group" aria-label="${T('这次调用', 'This call')}">
    <span class="lk">${T('调用', 'Call')}</span><span>${where} · ${p.provider_name} · <code class="nw">${p.model}</code> · ${verified}</span>
    <span class="lk">${T('发送', 'Sends')}</span><span>${payloadLine(p)} <span class="muted nw">· ${p.request_bytes} B</span></span>
    <span class="lk">${T('授权', 'Consent')}</span><span>${consentLine(p.consent, p.provider_name)}</span>
    <span class="lk">${T('预算', 'Budget')}</span><span><b class="num" style="font-weight:600">${p.budget.used} / ${p.budget.limit}</b> <span class="muted">${T(`今日 · ${p.provider_name}`, `today · ${p.provider_name}`)}</span></span>
  </div>`;
}
