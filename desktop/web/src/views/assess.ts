// Assessment, the still room (prototype vAssess + navigatorHTML). Clocks are display-only and read
// the server's presented_at and deadlines; the core decides timeouts. The navigator never shows
// correctness.
import type { ActiveRun, AssessmentItem, AssessmentPool } from '../api/types';
import { html, mmss, type Html } from '../core/html';
import { T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { RING_CIRCUMFERENCE } from '../core/clock';
import { assessmentCapsule } from '../components/condition';
import { optsHTML } from '../components/options';
import { assessFoot, passageHTML } from '../components/passage';
import { ic } from '../icons';
import { errorCard, loadingCard, retryButton, stateCard } from './states';

export function navigatorHTML(run: ActiveRun): Html {
  const n = { answered: 0, timed_out: 0, pending: 0, unavailable: 0 };
  const cells = run.cells.map((c) => {
    n[c.state]++;
    const cur = c.position === run.current_position;
    const cls = cur ? 'cur' : c.state === 'timed_out' ? 'to' : c.state === 'answered' ? 'ans' : '';
    const st = cur
      ? T('当前', 'current')
      : c.state === 'timed_out'
        ? T('超时', 'timed out')
        : c.state === 'answered'
          ? T('已作答', 'answered')
          : c.state === 'unavailable'
            ? T('不可用', 'unavailable')
            : T('未作答', 'pending');
    const closed = c.state === 'timed_out' || c.state === 'unavailable';
    return html`<button class="cell ${cls}" data-act="goto" data-v="${c.position}" aria-label="${T(`第 ${c.position} 题`, `Item ${c.position}`)} · ${st}" title="${c.position} · ${st}" ${closed ? 'disabled' : ''} ${cur ? html`aria-current="step"` : ''}></button>`;
  });
  return html`<div class="nav40" role="navigation" aria-label="${T('题目导航', 'Item navigator')}">
    <div class="legend">
      <span><i class="cell ans"></i>${T('已答', 'Answered')} <b class="num">${n.answered}</b></span>
      <span><i class="cell to"></i>${T('超时', 'Timed out')} <b class="num">${n.timed_out}</b></span>
      <span><i class="cell"></i>${T('未答', 'Pending')} <b class="num">${n.pending}</b></span>
    </div>
    <div class="cells">${cells}</div>
    <div class="leave"><span class="n">${T('离开后可恢复，', 'You can resume later;')}<br>${T('计时不暂停', 'the clock keeps running')}</span><button class="btn ghost sm" data-act="leave">${ic('door', 'xs')}${T('离开', 'Leave')}</button></div>
  </div>`;
}

/** Top-bar run clock (remaining time until the run deadline). */
export function runClockHTML(run: ActiveRun): Html {
  return html`<div class="runclock" role="timer" aria-label="${T('测评剩余时间', 'Run time left')}">
      <span class="l">${T('剩余', 'Left')}</span><span class="t" id="runclock" data-clock="until" data-t="${run.deadline_at}" data-notify="run:${run.run_id}">${mmss(run.minutes * 60)}</span><span class="l">/ ${mmss(run.minutes * 60)}</span></div>`;
}

function itemHTML(run: ActiveRun, it: AssessmentItem, sel: string | null, busy: boolean): Html {
  const C = RING_CIRCUMFERENCE;
  return html`<div class="scene read assess">
    ${passageHTML(it.passage, { mode: 'assess', foot: assessFoot(it.passage_positions) })}
    <div class="qcol">
      <div class="qsat">${assessmentCapsule(run.run_id, run.size, run.minutes)}</div>
      <article class="qcard card" aria-label="${T('题目', 'Question')}" data-position="${it.position}">
        <div class="as-top">
          <div class="ring" role="timer" aria-label="${T('本题用时', 'Item clock')}"><svg viewBox="0 0 56 56" aria-hidden="true"><circle class="bg" cx="28" cy="28" r="25" fill="none" stroke-width="3"/><circle class="fg" id="ringfg" cx="28" cy="28" r="25" fill="none" stroke-width="3" stroke-linecap="round" stroke-dasharray="${C.toFixed(1)}" stroke-dashoffset="${C.toFixed(1)}" data-clock="ring" data-from="${it.presented_at}" data-t="${it.item_deadline_at}"/></svg><span class="ring-t" id="itemt" data-clock="since" data-t="${it.presented_at}">0:00</span></div>
          <div><div class="h-disp" style="font-size:20px">${T('第 ', 'Item ')}<span class="num">${it.position}</span>${T(` / ${it.size} 题`, ` / ${it.size}`)}</div>
          <div class="meta">${T('本题', 'This item')} <span class="num" id="itemt2" data-clock="since" data-t="${it.presented_at}">0:00</span> / <span class="num">${mmss(it.per_item_ms / 1000)}</span><span class="sep"></span>${T(
            `每题 = ${run.minutes} 分钟 ÷ ${run.size}`,
            `per item = ${run.minutes} min ÷ ${run.size}`,
          )}<span hidden data-clock="until" data-t="${it.item_deadline_at}" data-notify="item:${it.run_id}:${it.position}"></span></div></div>
        </div>
        <p class="q-stem exam" lang="en">${it.stem}</p>
        ${optsHTML(it.options, sel, 'apick')}
        <div class="q-foot">
          <span class="keyhint"><kbd>←</kbd><kbd>→</kbd> ${T('换题', 'move')}</span>
          <span class="sp"></span>
          <button class="btn primary" data-act="anext" id="anext" ${sel && !busy ? '' : 'disabled'}>${T('确认并下一题', 'Confirm & next')}<kbd>Enter</kbd></button>
        </div>
      </article>
    </div>
  </div>`;
}

function noRun(pool: Loadable<AssessmentPool>): Html {
  if (pool.status === 'error') return errorCard(pool.error, retryButton('reload'));
  if (pool.status !== 'ready') return loadingCard();
  const p = pool.value;
  if (!p.can_start)
    return stateCard({
      ico: 'lock',
      eyebrow: T('测评', 'Assessment'),
      title: T('未见测评池不足，暂不能测评', 'Not enough unseen items to assess'),
      body: html`${p.shortfall?.message ?? T(`剩余 ${p.items} 题，下限 ${p.floor_items} 题。`, `${p.items} items left, floor ${p.floor_items}.`)}${
        p.shortfall?.action ? html`<br><span class="muted">${T('下一步：', 'Next: ')}<code>${p.shortfall.action}</code></span>` : ''
      }`,
      code: p.shortfall?.code ?? 'pool_floor',
    });
  return stateCard({
    ico: 'assess',
    eyebrow: T('测评', 'Assessment'),
    title: T('限时、无辅助的未见测评', 'A timed, unassisted, unseen run'),
    body: T(
      `未见测评池剩余 ${p.items} 题 / ${p.units} 单元（下限 ${p.floor_items} 题）。一轮 ${p.default_size} 题 / ${p.default_minutes} 分钟，作答后不显示对错。`,
      `${p.items} unseen items / ${p.units} units remain (floor ${p.floor_items}). One run is ${p.default_size} items / ${p.default_minutes} min, with no feedback after answering.`,
    ),
    action: html`<button class="btn primary lg" data-act="run-ask">${T('开始测评', 'Start a run')}${ic('arrow', 'xs')}</button>`,
  });
}

export interface AssessUi {
  sel: string | null;
  busy: boolean;
}

export function vAssess(run: ActiveRun | null, item: Loadable<AssessmentItem>, pool: Loadable<AssessmentPool>, u: AssessUi): Html {
  if (!run) return noRun(pool);
  if (item.status === 'error') return errorCard(item.error, retryButton('reload'));
  if (item.status !== 'ready') return loadingCard();
  return itemHTML(run, item.value, u.sel ?? item.value.selected_label, u.busy);
}
