// Assessment report (prototype vReport + annSheet + the report sheets): the observed score with
// `pass threshold: unknown`, the like-for-like comparison or its refusal, per-skill counts, and
// per-item annotations that were withheld during the run.
import type { AssessmentComparison, AssessmentReport, ReportItem, RunSummary } from '../api/types';
import { html, type Html } from '../core/html';
import { monthDay, RSTATE, sec, shortDate, skillName, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { sat } from '../components/satellite';
import { kv, sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { stateCard } from './states';

const outcomeCls = (o: ReportItem['outcome']) => RSTATE[o][3];

function ticks(n: number, on: number, cls: string, lab: string, sub: string, plusFrom?: number): Html {
  const cells = Array.from({ length: n }, (_, i) => html`<i class="${i < on ? (plusFrom !== undefined && i >= plusFrom ? 'plus' : 'on') : ''}"></i>`);
  return html`<div class="ticks ${cls}"><span class="lab">${lab}<small>${sub}</small></span><span class="bar" aria-hidden="true">${cells}</span><span class="v num">${on} / ${n}</span></div>`;
}

function checkName(name: string): string {
  switch (name) {
    case 'same_size':
      return T('同样题数', 'same size');
    case 'same_window':
      return T('同样时长', 'same time window');
    case 'same_pool_family':
      return T('同一题池族', 'same pool family');
    default:
      return name;
  }
}

function comparison(r: AssessmentReport, others: RunSummary[], compareWith: number | null, cmp: Loadable<AssessmentComparison>): Html {
  if (others.length === 0) return html`<p class="fine" style="margin:10px 0 0">${T('还没有可比较的其他测评。', 'No other run to compare with yet.')}</p>`;
  const seg = html`<div class="seg2" role="group" aria-label="${T('比较对象', 'Compare with')}">${others.map(
    (o) => html`<button data-act="cmp" data-v="${o.run_id}" aria-pressed="${compareWith === o.run_id}">${T(`对比 #${o.run_id}`, `vs #${o.run_id}`)}</button>`,
  )}</div>`;
  let bodyHtml: Html;
  if (cmp.status === 'error') bodyHtml = html`<div class="refuse">${ic('stop', 's')} ${cmp.error.message}</div>`;
  else if (cmp.status !== 'ready') bodyHtml = html`<p class="fine">${compareWith === null ? T('选择一轮进行比较。', 'Pick a run to compare with.') : T('正在比较……', 'Comparing…')}</p>`;
  else {
    const c = cmp.value;
    if (!c.like_for_like || c.refusal)
      bodyHtml = html`<div class="refuse">${ic('stop', 's')} <b>${T('不比较。', 'Comparison refused.')}</b> ${c.refusal?.message ?? ''}</div>`;
    else {
      const base = c.other;
      const baseCorrect = base.correct ?? 0;
      const now = r.counts.correct;
      const delta = c.correct_delta ?? now - baseCorrect;
      bodyHtml = html`${ticks(base.size, baseCorrect, '', `#${base.run_id}`, monthDay(base.started_at))}${ticks(
        r.run.size,
        now,
        'now',
        `#${r.run.run_id}`,
        monthDay(r.run.started_at),
        delta > 0 ? baseCorrect : undefined,
      )}
       <div class="delta"><span class="d1 num">${delta > 0 ? '+' : ''}${delta}</span><span>${T('题', 'items')}</span>${c.checks.map(
         (k) => html`<span class="chk">${ic(k.ok ? 'check' : 'x', 'xs')}${checkName(k.name)}</span>`,
       )}</div>`;
    }
  }
  return html`<div class="cmp">
        <div class="cmp-h"><span class="eyebrow">${T('同口径比较', 'Like-for-like')}</span>${seg}</div>
        ${bodyHtml}
      </div>`;
}

export function vReport(r: AssessmentReport | null, runs: RunSummary[], compareWith: number | null, cmp: Loadable<AssessmentComparison>): Html {
  if (!r)
    return stateCard({
      ico: 'report',
      eyebrow: T('报告', 'Report'),
      title: T('还没有完成的测评', 'No completed assessment yet'),
      body: T('完成一轮未见测评后，这里显示观察到的结果。及格线未知，不作预测。', 'After a completed unseen run this shows the observed result. The pass threshold is unknown; nothing is predicted.'),
      action: html`<a class="btn primary" href="#/assess">${T('去测评', 'Go to assessment')}${ic('arrow', 'xs')}</a>`,
    });
  const others = runs.filter((x) => x.run_id !== r.run.run_id && x.state === 'completed').slice(0, 3);
  const s = T('秒', 's');
  const annotated = r.items.filter((i) => !i.annotations.withheld).length;
  return html`<div class="scene orbit" style="--obj-w:640px">
    <div class="rail l">
      ${sat('time', 'clock', T('中位用时', 'Median time'), html`<span class="num">${sec(r.speed.median_ms)}</span><small>${s} · p90 <span class="num">${sec(r.speed.p90_ms)}</span> ${s}</small>`, '', 1)}
      ${sat('timeout', 'tout', T('超时', 'Timed out'), html`<span class="num">${r.counts.timed_out}</span><small>${T('题 · 不计分', 'items · not scored')}</small>`, '', 2)}
      ${sat('contam', 'shield', T('污染检查', 'Contamination'), html`<span class="num">${r.contamination.excluded_positions.length}</span><small>${T('题被排除', 'items excluded')}</small>`, '', 3)}
    </div>
    <article class="obj card rcard rise" style="--i:1" aria-labelledby="rep-t">
      <div class="obj-top"><span class="eyebrow" id="rep-t">${T(`测评第 ${r.run.run_id} 轮 · 已完成`, `Assessment run #${r.run.run_id} · complete`)}</span><span class="r"><span class="nw">${shortDate(
        r.run.completed_at ?? r.run.started_at,
      )}</span><span class="sep"></span>${T(`${r.run.size} 题 · ${r.run.minutes} 分钟`, `${r.run.size} items · ${r.run.minutes} min`)}</span></div>
      <div class="score">
        <div class="n num" aria-label="${T(`答对 ${r.counts.correct} / ${r.counts.scored}`, `${r.counts.correct} of ${r.counts.scored} correct`)}">${r.counts.correct}<small>/ ${r.counts.scored}</small></div>
        <div class="side"><span class="chip" id="pass-threshold">${T('及格线：未知', 'Pass threshold: unknown')}</span><span class="t">${T('只报告观察到的结果，', 'Observed result only —')}<br>${T('不给概率，也不下结论。', 'no probability, no verdict.')}</span></div>
      </div>
      ${comparison(r, others, compareWith, cmp)}
      <div class="skills">${r.skills.map(
        (k) =>
          html`<div class="sk"><span class="nm" title="${k.skill_id}">${skillName(k.skill_id)}</span><span class="v num">${k.correct}<small> / ${k.total}</small></span><span class="pips" aria-hidden="true">${Array.from(
            { length: k.total },
            (_, i) => html`<i class="${i < k.correct ? 'on' : ''}"></i>`,
          )}</span></div>`,
      )}</div>
    </article>
    <div class="rail r">
      ${sat('ann', 'eye', T('逐题注释 · 已解锁', 'Annotations · unlocked'), html`<span class="num">${annotated}</span><small>${T('题 · 证据与解析', 'items · evidence & reasoning')}</small>`, 'ok', 4)}
      ${
        r.pool_after
          ? sat(
              'pool2',
              'lock',
              T('测评池剩余', 'Pool after run'),
              html`<span class="num">${r.pool_after.items}</span><small>${T(
                `题 · ${r.pool_after.units} 单元 · 下限 ${r.pool_after.floor_items}`,
                `items · ${r.pool_after.units} units · floor ${r.pool_after.floor_items}`,
              )}</small>`,
              r.pool_after.items < r.pool_after.floor_items + r.run.size ? 'warm' : '',
              5,
            )
          : ''
      }
    </div>
  </div>`;
}

function annSheet(r: AssessmentReport, n: number | null): Html {
  const count = (o: 'c' | 'w' | 't') => r.items.filter((i) => outcomeCls(i.outcome) === o).length;
  const grid = html`<div class="rcells" role="group" aria-label="${T(`第 ${r.run.run_id} 轮逐题`, `Run #${r.run.run_id} items`)}">${r.items.map((it) => {
    const st = RSTATE[it.outcome];
    return html`<button class="rcell ${st[3]} ${n === it.position ? 'sel' : ''}" data-act="ann" data-v="${it.position}" aria-label="${T(`第 ${it.position} 题 · ${st[0]}`, `Item ${it.position} · ${st[1]}`)}" aria-pressed="${n === it.position}">${ic(st[2])}<span class="num">${it.position}</span></button>`;
  })}</div>
    <div class="rleg">${(['correct', 'incorrect', 'timed_out'] as const).map(
      (o) => html`<span><span class="rcell ${RSTATE[o][3]}">${ic(RSTATE[o][2])}</span>${T(RSTATE[o][0], RSTATE[o][1])} <b class="num">${count(RSTATE[o][3])}</b></span>`,
    )}</div>`;
  const it = n !== null ? r.items.find((x) => x.position === n) : undefined;
  let det = html`<p class="fine" style="margin-top:16px">${T('选一题查看证据与解析（测评中扣留，现在开放）。', 'Pick an item to see its evidence and reasoning (withheld during the run, open now).')}</p>`;
  if (it) {
    const st = RSTATE[it.outcome];
    const a = it.annotations;
    const optReason = (label: string) => a.option_reasoning.find((x) => x.label === label)?.text ?? null;
    det = html`<div class="blk" style="margin-top:16px">
      <h3>${T(`第 ${it.position} 题`, `Item ${it.position}`)}<span class="muted" style="font-weight:500">${it.skill_ids.map(skillName).join(' · ')}</span><span class="chip ${
        it.outcome === 'correct' ? 'ok' : it.outcome === 'incorrect' ? 'bad' : ''
      } sm" style="margin-left:auto">${T(st[0], st[1])}</span></h3>
      <p class="exam" lang="en" style="font-size:17px;line-height:1.5;color:var(--ink);margin:8px 0 10px">${it.stem}</p>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:6px 18px;font-size:var(--t-sm)">${it.options.map((o) => {
        const isKey = o.label === it.key_label;
        const why = a.withheld ? null : optReason(o.label);
        const missing = !a.withheld && a.option_reasoning_missing.includes(o.label);
        return html`<div style="display:flex;gap:8px;align-items:baseline;flex-wrap:wrap;${isKey ? 'color:var(--ok);font-weight:600' : 'color:var(--ink-2)'}"><b>${o.label}</b><span class="exam" lang="en">${o.text}</span>${
          o.label === it.selected_label && !isKey ? html`<span class="chip bad sm">${T('你的选择', 'yours')}</span>` : ''
        }${why ? html`<span class="muted" style="font-weight:400;flex-basis:100%">${why}</span>` : ''}${
          missing ? html`<span class="muted" style="font-weight:400;flex-basis:100%">${T('原书未提供该选项的解析。', 'No explanation provided for this option.')}</span>` : ''
        }</div>`;
      })}</div>
      ${
        a.withheld
          ? html`<p class="fine">${T('这一题的注释仍被扣留。', 'Annotations for this item are still withheld.')}</p>`
          : html`${a.evidence ? html`<div class="tactic" style="margin:12px 0 0;background:var(--glass-3)"><span class="ic">${ic('seal', 's')}</span><div class="k">${T('证据', 'Evidence')}</div><p class="exam" lang="en" style="font-size:15px;color:var(--ink)">“${a.evidence}”</p></div>` : ''}${
              a.reasoning ? html`<p class="fine">${a.reasoning}</p>` : ''
            }`
      }
    </div>`;
  }
  return sheetWrap('eye', T(`逐题注释 · 第 ${r.run.run_id} 轮`, `Annotations · run #${r.run.run_id}`), T('证据与解析在测评中被扣留，现在可逐题查看', 'Evidence and reasoning were withheld during the run'), html`${grid}${det}`, true);
}

export function reportSheet(key: string, r: AssessmentReport, annItem: number | null): Html | null {
  const s = T('秒', 's');
  const timedOut = r.items.filter((i) => i.outcome === 'timed_out').map((i) => i.position);
  switch (key) {
    case 'ann':
      return annSheet(r, annItem);
    case 'time':
      return sheetWrap(
        'clock',
        T('时间', 'Time'),
        T(`第 ${r.run.run_id} 轮 · 每题上限 ${sec(r.per_item_ms)} 秒`, `Run #${r.run.run_id} · ${sec(r.per_item_ms)} s per item`),
        html`<div class="kv"><span>${T('中位用时', 'Median')}</span><b class="big num">${sec(r.speed.median_ms)} <small style="font-size:14px">${s}</small></b></div>
        <div class="kv"><span>p90</span><b class="big num">${sec(r.speed.p90_ms)} <small style="font-size:14px">${s}</small></b></div>
        ${kv(T('超时', 'Timed out'), timedOut.length ? T(`${timedOut.length} 题 · 第 ${timedOut.join('、')} 题`, `${timedOut.length} items · ${timedOut.join(', ')}`) : '0')}`,
      );
    case 'timeout':
      return sheetWrap(
        'tout',
        T('超时', 'Timed out'),
        T('超时的题不计分', 'Timed-out items are not scored'),
        html`${kv(T('超时', 'Timed out'), r.counts.timed_out, true)}${kv(T('未作答', 'Unanswered'), r.counts.unanswered)}${
          timedOut.length ? html`<p class="fine">${T(`第 ${timedOut.join('、')} 题`, `Items ${timedOut.join(', ')}`)}</p>` : ''
        }`,
      );
    case 'contam': {
      const c = r.contamination;
      return sheetWrap(
        'shield',
        T('污染检查', 'Contamination check'),
        T('运行前见过的题会被列出并排除计分', 'Items seen before the run are listed and excluded'),
        html`${kv(T('本轮被排除', 'Excluded this run'), c.excluded_positions.length, true)}
        ${kv(T('运行前见过', 'Seen before the run'), c.prior_exposure)}
        ${kv(T('重复变体', 'Repeated variants'), c.repeated_variant)}
        ${kv(T('运行中看过提示', 'Hints during the run'), c.hints_during_run)}
        ${kv(T('超时后恢复', 'Resumed after the window'), c.resumed_after_window)}
        ${r.limitations.length ? html`<p class="fine">${r.limitations.join(' ')}</p>` : ''}`,
      );
    }
    case 'pool2': {
      const p = r.pool_after;
      if (!p) return null;
      const max = Math.max(p.items_before, p.items, p.floor_items, 1);
      return sheetWrap(
        'lock',
        T('测评池剩余', 'Pool after run'),
        T(`下限 ${p.floor_items} 题`, `Floor ${p.floor_items} items`),
        html`<div class="kv"><span>${T('剩余', 'Remaining')}</span><b class="big num">${p.items} <small style="font-size:14px;color:var(--ink-3)">${T(`题 / ${p.units} 单元`, `items / ${p.units} units`)}</small></b></div>
        <div class="floor" aria-hidden="true"><i style="width:${((p.items / max) * 100).toFixed(2)}%"></i><b style="left:${((p.floor_items / max) * 100).toFixed(2)}%"></b></div>
        <div class="floor-l"><span>0</span><span style="left:${((p.floor_items / max) * 100).toFixed(2)}%">${T(`下限 ${p.floor_items}`, `floor ${p.floor_items}`)}</span><span style="right:0">${T(`运行前 ${p.items_before}`, `${p.items_before} before`)}</span></div>`,
      );
    }
  }
  return null;
}

export function reportPopLines(r: AssessmentReport, cmp: Loadable<AssessmentComparison>): string[] {
  const lines: string[] = [];
  if (cmp.status === 'ready') {
    const c = cmp.value;
    if (c.like_for_like && !c.refusal && c.correct_delta !== null)
      lines.push(T(`这次和第 ${c.other.run_id} 轮同口径：${c.correct_delta > 0 ? '+' : ''}${c.correct_delta} 题。`, `Like-for-like with run #${c.other.run_id}: ${c.correct_delta > 0 ? '+' : ''}${c.correct_delta} items.`));
    else if (c.refusal) lines.push(c.refusal.message);
  }
  for (const u of r.unknowns) lines.push(u);
  lines.push(T('及格线未知：这里只有观察到的结果。', 'The pass threshold is unknown: this is observation only.'));
  return lines;
}
