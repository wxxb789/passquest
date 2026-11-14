// Assessment, the still room (prototype vAssess + navigatorHTML). Clocks are display-only and read
// the server's absolute instants (item presented_at/deadline_at, run deadline_at); when a deadline
// passes the window asks the core, which records the timeout. The navigator never shows
// correctness. Positions are 0-based in the core and shown 1-based.
import type { AssessmentItem, AssessmentPool, AssessmentRun, AssessmentState, NavigatorItem } from '../api/types';
import { html, mmss, type Html } from '../core/html';
import { T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { RING_CIRCUMFERENCE } from '../core/clock';
import { assessmentCapsule } from '../components/condition';
import { optsHTML } from '../components/options';
import { assessFoot, passageHTML } from '../components/passage';
import { ic } from '../icons';
import { errorCard, loadingCard, retryButton, stateCard } from './states';

function cellState(c: NavigatorItem): readonly [string, string] {
  if (c.current || c.state === 'presented') return ['cur', T('当前', 'current')];
  switch (c.state) {
    case 'answered':
      return ['ans', T('已作答', 'answered')];
    case 'timed_out':
      return ['to', T('超时', 'timed out')];
    case 'unavailable':
      return ['to', T('不可用', 'unavailable')];
    default:
      return ['', T('未作答', 'pending')];
  }
}

export function navigatorHTML(nav: readonly NavigatorItem[], leave: boolean): Html {
  const n = { answered: 0, timed_out: 0, pending: 0 };
  for (const c of nav) {
    if (c.state === 'answered') n.answered++;
    else if (c.state === 'timed_out') n.timed_out++;
    else if (c.state === 'pending' || c.state === 'presented') n.pending++;
  }
  const cells = nav.map((c) => {
    const [cls, st] = cellState(c);
    return html`<span class="cell ${cls}" role="img" data-state="${c.state}" data-position="${c.position}" aria-label="${T(`第 ${c.position + 1} 题`, `Item ${c.position + 1}`)} · ${st}" title="${c.position + 1} · ${st}" ${
      cls === 'cur' ? html`aria-current="step"` : ''
    }></span>`;
  });
  return html`<div class="nav40" role="navigation" aria-label="${T('题目导航', 'Item navigator')}" data-testid="navigator">
    <div class="legend">
      <span><i class="cell ans"></i>${T('已答', 'Answered')} <b class="num">${n.answered}</b></span>
      <span><i class="cell to"></i>${T('超时', 'Timed out')} <b class="num">${n.timed_out}</b></span>
      <span><i class="cell"></i>${T('未答', 'Pending')} <b class="num">${n.pending}</b></span>
    </div>
    <div class="cells">${cells}</div>
    ${
      leave
        ? html`<div class="leave"><span class="n">${T('离开后可恢复，', 'You can resume later;')}<br>${T('计时不暂停', 'the clock keeps running')}</span><button class="btn ghost sm" data-act="leave">${ic('door', 'xs')}${T('离开', 'Leave')}</button></div>`
        : ''
    }
  </div>`;
}

/** Top-bar run clock (remaining time until the run deadline). */
export function runClockHTML(run: AssessmentRun): Html {
  return html`<div class="runclock" role="timer" aria-label="${T('测评剩余时间', 'Run time left')}">
      <span class="l">${T('剩余', 'Left')}</span><span class="t" id="runclock" data-clock="until" data-t="${run.deadline_at}" data-notify="run:${run.id}">${mmss(run.window_minutes * 60)}</span><span class="l">/ ${mmss(run.window_minutes * 60)}</span></div>`;
}

function itemHTML(run: AssessmentRun, it: AssessmentItem, sel: string | null, busy: boolean): Html {
  const C = RING_CIRCUMFERENCE;
  const per = run.per_item_timeout_ms;
  return html`<div class="scene read assess" data-testid="assess">
    ${passageHTML(it.passage, { mode: 'assess', foot: assessFoot(it.position, it.count) })}
    <div class="qcol">
      <div class="qsat">${assessmentCapsule(run.id, run.actual_size, run.window_minutes)}</div>
      <article class="qcard card" aria-label="${T('题目', 'Question')}" data-testid="assess-item" data-run="${run.id}" data-position="${it.position}" data-presented-at="${it.presented_at}" data-deadline-at="${it.deadline_at}">
        <div class="as-top">
          <div class="ring" role="timer" aria-label="${T('本题用时', 'Item clock')}"><svg viewBox="0 0 56 56" aria-hidden="true"><circle class="bg" cx="28" cy="28" r="25" fill="none" stroke-width="3"/><circle class="fg" id="ringfg" cx="28" cy="28" r="25" fill="none" stroke-width="3" stroke-linecap="round" stroke-dasharray="${C.toFixed(1)}" stroke-dashoffset="${C.toFixed(1)}" data-clock="ring" data-from="${it.presented_at}" data-t="${it.deadline_at}"/></svg><span class="ring-t" id="itemt" data-clock="since" data-t="${it.presented_at}">0:00</span></div>
          <div><div class="h-disp" style="font-size:20px">${T('第 ', 'Item ')}<span class="num">${it.position + 1}</span>${T(` / ${it.count} 题`, ` / ${it.count}`)}</div>
          <div class="meta">${T('本题', 'This item')} <span class="num" id="itemt2" data-clock="since" data-t="${it.presented_at}">0:00</span> / <span class="num">${mmss(per / 1000)}</span><span class="sep"></span>${T(
            `每题 = ${run.window_minutes} 分钟 ÷ ${run.actual_size}`,
            `per item = ${run.window_minutes} min ÷ ${run.actual_size}`,
          )}<span hidden data-clock="until" data-t="${it.deadline_at}" data-notify="item:${it.run_id}:${it.position}"></span></div></div>
        </div>
        <p class="q-stem exam" lang="en">${it.stem}</p>
        ${optsHTML(it.options, sel, 'apick')}
        <div class="q-foot">
          <button class="btn quiet" data-act="askip-item" ${busy ? 'disabled' : ''}>${T('跳过', 'Skip')}<kbd>S</kbd></button>
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
  if (p.source_count === 0)
    return stateCard({
      ico: 'lock',
      eyebrow: T('测评', 'Assessment'),
      title: T('还没有未见测评池', 'There is no unseen pool yet'),
      body: T('测评只用已审核导入来源里没见过的题。先导入并审核一份来源。', 'An assessment only uses unseen items from a reviewed imported source. Import and review one first.'),
      code: 'pool_floor',
    });
  if (!p.floor_met)
    return stateCard({
      ico: 'lock',
      eyebrow: T('测评', 'Assessment'),
      title: T('未见测评池不足，暂不能测评', 'Not enough unseen items to assess'),
      body: T(
        `剩余 ${p.qualifying_items} 题 / ${p.qualifying_units} 单元，下限 ${p.floor_items} 题，还差 ${p.shortfall_items} 题。练习过的文章不会回到测评池。`,
        `${p.qualifying_items} items / ${p.qualifying_units} units remain; the floor is ${p.floor_items}, ${p.shortfall_items} short. A practised passage never returns to the pool.`,
      ),
      code: 'pool_floor',
    });
  return stateCard({
    ico: 'assess',
    eyebrow: T('测评', 'Assessment'),
    title: T('限时、无辅助的未见测评', 'A timed, unassisted, unseen run'),
    body: T(
      `未见测评池剩余 ${p.qualifying_items} 题 / ${p.qualifying_units} 单元（下限 ${p.floor_items} 题）。一轮 ${p.default_size} 题 / ${p.default_minutes} 分钟，作答后不显示对错。`,
      `${p.qualifying_items} unseen items / ${p.qualifying_units} units remain (floor ${p.floor_items}). One run is ${p.default_size} items / ${p.default_minutes} min, with no feedback after answering.`,
    ),
    action: html`<button class="btn primary lg" data-act="run-ask">${T('开始测评', 'Start a run')}${ic('arrow', 'xs')}</button>`,
  });
}

/** An active run this window is not driving yet: resuming counts one resume; the clock never stopped. */
function resumeCard(s: AssessmentState, run: AssessmentRun): Html {
  const answered = s.navigator.filter((c) => c.state === 'answered').length;
  return stateCard({
    ico: 'assess',
    eyebrow: T('测评进行中', 'Assessment in progress'),
    title: T(`第 ${run.id} 轮 · 已答 ${answered} / ${run.actual_size}`, `Run #${run.id} · ${answered} of ${run.actual_size} answered`),
    body: html`${T('计时一直在走，剩余 ', 'The clock kept running: ')}<b class="num" data-clock="until" data-t="${run.deadline_at}" data-notify="run:${run.id}">—</b>${T(
      '。继续会记一次恢复，恢复后作答的题会在报告里标出。',
      ' left. Resuming records one resume; items answered after it are marked in the report.',
    )}`,
    code: 'assessment_active',
    action: html`<button class="btn quiet" data-act="run-abandon-ask">${T('放弃本轮…', 'Abandon the run…')}</button><button class="btn primary lg" data-act="run-resume" data-testid="resume-run">${T('继续测评', 'Resume the run')}${ic('arrow', 'xs')}</button>`,
  });
}

export interface AssessUi {
  sel: string | null;
  busy: boolean;
  attached: boolean;
}

export function vAssess(s: AssessmentState, pool: Loadable<AssessmentPool>, u: AssessUi): Html {
  const run = s.run;
  if (!run || s.state === 'none' || s.state === 'completed' || s.state === 'abandoned') return noRun(pool);
  if (s.item && u.attached) return itemHTML(run, s.item, u.sel, u.busy);
  return resumeCard(s, run);
}
