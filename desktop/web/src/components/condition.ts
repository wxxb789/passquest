// Condition capsule shown before answering (prototype .qsat .cond) and the feedback chip.
import type { Condition } from '../api/types';
import { html, type Html } from '../core/html';
import { CONDITION, T, TP } from '../core/i18n';

export function conditionCapsule(condition: Condition, canCount: boolean, hintOpen: boolean): Html {
  const c = CONDITION[condition];
  const room = condition === 'independent' && !canCount ? 'warm' : c.room;
  const title = hintOpen ? T(`${TP(c.title)} · 已看提示`, `${TP(c.title)} · hint visible`) : TP(c.title);
  return html`<div class="cond ${room}"><span class="glow"></span><div><b>${title}</b><span>${canCount ? TP(c.can) : TP(c.cannot)}</span></div></div>`;
}

export function assessmentCapsule(runId: number, size: number, minutes: number): Html {
  return html`<div class="cond still glass"><span class="glow"></span><div><b>${T(`测评 · 第 ${runId} 轮 · 限时、无辅助`, `Assessment · run #${runId} · timed, no help`)}</b><span>${T(
    `${size} 题 / ${minutes} 分钟 · 作答后不显示对错`,
    `${size} items / ${minutes} min · no feedback after answering`,
  )}</span></div></div>`;
}

export function conditionChip(condition: Condition, counts: boolean): Html {
  const c = CONDITION[condition];
  const tone = counts ? 'cool' : 'warm';
  const text = counts ? T(`${TP(c.title)} · 计入`, `${TP(c.title)} · counts`) : T(`${TP(c.title)} · 不计入`, `${TP(c.title)} · does not count`);
  return html`<span class="chip ${tone} sm"><span class="state-dot ${tone}"></span>${text}</span>`;
}
