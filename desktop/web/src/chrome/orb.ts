// The coach orb (prototype orbHTML). It leaves the room during an assessment; on Feedback the coach
// sheet grows from it; elsewhere it opens a small local-policy popover.
import type { ModelStatus } from '../api/types';
import { html, type Html } from '../core/html';
import { T } from '../core/i18n';
import type { Route } from '../core/state';

export interface OrbInput {
  route: Route;
  coachOpen: boolean;
  popOpen: boolean;
  model: ModelStatus | null;
  coachSheet: Html | null;
}

/** The active provider's calls / limit today, from the core's budget. */
function budgetOf(m: ModelStatus): string {
  const b = m.budget.providers.find((p) => p.provider === m.provider);
  return b ? `${b.calls} / ${b.limit}` : '—';
}

export function orbHTML(i: OrbInput): Html {
  const r = i.route;
  if (r === 'assess') return html``;
  const sheet = r === 'feedback' && i.coachOpen && i.coachSheet !== null;
  const st = r === 'today' || r === 'report' ? '' : sheet ? 'open' : 'quiet';
  const say =
    r === 'today'
      ? i.model?.configured && i.model.model
        ? html`<span class="orb-say" data-testid="orb-model"><span class="state-dot ${i.model.text_chat_state === 'verified' ? 'ok' : 'warm'}"></span>${i.model.model}<span class="sep"></span>${
            i.model.text_chat_state === 'verified' ? T('已验证', 'verified') : T('未验证', 'not verified')
          }<span class="sep"></span>${T('今日', 'today')} <b class="num">${budgetOf(i.model)}</b></span>`
        : html`<span class="orb-say"><span class="state-dot off"></span>${T('未选择模型', 'No model chosen')}</span>`
      : r === 'report'
        ? html`<span class="orb-say">${T('报告说明', 'About this report')}</span>`
        : '';
  const label = sheet ? T('收起导师', 'Collapse coach') : T('导师', 'Coach');
  return html`${sheet ? i.coachSheet : ''}<div class="orb-wrap">${say}<button class="orb ${st}" data-act="coach" aria-label="${label}" aria-expanded="${sheet || i.popOpen}"><span class="core"></span></button></div>`;
}

/** The small non-modal coach popover (prototype layer 'pop'): local policy lines, never model text. */
export function popHTML(lines: readonly string[], foot: Html | ''): Html {
  return html`<div class="csheet small" role="dialog" aria-labelledby="pop-t">
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:12px"><span class="co-head" style="padding:0"><span class="o"></span></span><b id="pop-t">${T('导师', 'Coach')}</b><span class="chip sm" style="margin-left:auto">${T(
        '本地策略 · 非模型生成',
        'Local policy · not model text',
      )}</span></div>
      ${lines.map((l) => html`<p style="margin:0 0 8px;font-size:var(--t-b);line-height:1.65">${l}</p>`)}
      ${foot}
      <div style="display:flex;justify-content:flex-end;margin-top:6px"><button class="btn quiet sm" data-act="close">${T('好', 'OK')}<kbd>Esc</kbd></button></div></div>`;
}
