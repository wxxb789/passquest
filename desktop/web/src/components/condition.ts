// Condition capsule shown before answering (prototype .qsat .cond) and the feedback chip. The
// condition and whether it can count are the core's (`PracticeItem.condition`, `can_count`).
import { html, type Html } from '../core/html';
import { conditionOf, T, TP } from '../core/i18n';

export function conditionCapsule(condition: string, canCount: boolean, hintOpen: boolean, guided: boolean): Html {
  const c = conditionOf(condition);
  const room = canCount ? 'cool' : c.room;
  const title = hintOpen
    ? T(`${TP(c.title)} · 已看提示`, `${TP(c.title)} · hint visible`)
    : guided
      ? T(`${TP(c.title)} · 引导讲解`, `${TP(c.title)} · guided walkthrough`)
      : TP(c.title);
  return html`<div class="cond ${room}" data-testid="condition" data-condition="${condition}" data-can-count="${canCount}"><span class="glow"></span><div><b>${title}</b><span>${canCount ? TP(c.can) : TP(c.cannot)}</span></div></div>`;
}

export function assessmentCapsule(runId: number, size: number, minutes: number): Html {
  return html`<div class="cond still glass"><span class="glow"></span><div><b>${T(`测评 · 第 ${runId} 轮 · 限时、无辅助`, `Assessment · run #${runId} · timed, no help`)}</b><span>${T(
    `${size} 题 / ${minutes} 分钟 · 作答后不显示对错`,
    `${size} items / ${minutes} min · no feedback after answering`,
  )}</span></div></div>`;
}

export function conditionChip(condition: string, counted: boolean): Html {
  const c = conditionOf(condition);
  const tone = counted ? 'cool' : 'warm';
  const text = counted ? T(`${TP(c.title)} · 计入`, `${TP(c.title)} · counts`) : T(`${TP(c.title)} · 不计入`, `${TP(c.title)} · does not count`);
  return html`<span class="chip ${tone} sm" data-testid="feedback-condition" data-condition="${condition}" data-counted="${counted}"><span class="state-dot ${tone}"></span>${text}</span>`;
}
