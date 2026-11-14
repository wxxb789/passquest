// Assessment report (prototype vReport + annSheet + the report sheets): the observed score with
// `pass threshold: unknown`, the like-for-like comparison or its refusal, per-skill counts, timing,
// contamination, and per-item annotations that were withheld during the run.
import type { AssessmentComparison, AssessmentReport, AssessmentReportItem, AssessmentRun, StudioError } from '../api/types';
import { html, type Html } from '../core/html';
import { contaminationLabel, errorLabel, errorTitle, monthDay, RSTATE, sec, shortDate, skillName, T, type ItemOutcome } from '../core/i18n';
import { ui, type Loadable } from '../core/state';
import { sat } from '../components/satellite';
import { kv, sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { stateCard } from './states';

/** How an item ended, from the report's own fields; correctness only appears after the run. */
export function outcomeOf(it: AssessmentReportItem): ItemOutcome {
  if (it.state === 'unavailable') return 'unavailable';
  if (it.timed_out) return 'timed_out';
  if (it.state !== 'answered') return 'pending';
  if (it.contamination_reasons?.length) return 'excluded';
  if (it.status === 'correct' || it.status === 'incorrect' || it.status === 'skipped') return it.status;
  return 'pending';
}

function ticks(n: number, on: number, cls: string, lab: string, sub: string): Html {
  const cells = Array.from({ length: Math.min(n, 60) }, (_, i) => html`<i class="${i < on ? 'on' : ''}"></i>`);
  return html`<div class="ticks ${cls}"><span class="lab">${lab}<small>${sub}</small></span><span class="bar" aria-hidden="true">${cells}</span><span class="v num">${on} / ${n}</span></div>`;
}

const DIMENSION: Record<string, readonly [string, string]> = {
  status: ['完成状态', 'completion'],
  item_count: ['题数', 'item count'],
  window_minutes: ['时长', 'time window'],
  pool_family: ['题池族', 'pool family'],
};
const dimName = (d: string) => (DIMENSION[d] ? T(DIMENSION[d][0], DIMENSION[d][1]) : T('其他条件', 'another condition'));

/** The first unlike dimension of two runs, in the core's check order (to word the core's refusal). */
function unlike(a: AssessmentRun, b: AssessmentRun): string | null {
  if (a.status !== 'completed' || b.status !== 'completed') return 'status';
  if (a.actual_size !== b.actual_size) return 'item_count';
  if (a.window_minutes !== b.window_minutes) return 'window_minutes';
  if (a.pool_family !== b.pool_family) return 'pool_family';
  return null;
}

function refusal(e: StudioError, base: AssessmentRun, other: AssessmentRun | undefined): Html {
  const d = other ? unlike(other, base) : null;
  const why = d ? T(`${dimName(d)}不同，不做比较。`, `The ${dimName(d)} differs, so the runs are not compared.`) : '';
  // The localized sentence states the core's refusal; without one, the core's own message is shown.
  const detail = why && ui.lang === 'zh' ? '' : html`<span class="muted" lang="en"> ${e.message}</span>`;
  return html`<div class="refuse" data-testid="compare-refused" data-code="${e.code}" title="${e.message}">${ic('stop', 's')} <b>${errorLabel(e.code)}</b> ${why}${detail}</div>`;
}

function comparison(r: AssessmentReport, others: AssessmentRun[], compareWith: number | null, cmp: Loadable<AssessmentComparison>): Html {
  if (others.length === 0) return html`<p class="fine" style="margin:10px 0 0">${T('还没有其他已完成的测评可比较。', 'No other completed run to compare with yet.')}</p>`;
  const seg = html`<div class="seg2" role="group" aria-label="${T('比较对象', 'Compare with')}">${others.map(
    (o) => html`<button data-act="cmp" data-v="${o.id}" aria-pressed="${compareWith === o.id}">${T(`对比 #${o.id}`, `vs #${o.id}`)}</button>`,
  )}</div>`;
  let bodyHtml: Html;
  if (cmp.status === 'error') bodyHtml = refusal(cmp.error, r.run, others.find((o) => o.id === compareWith));
  else if (cmp.status !== 'ready') bodyHtml = html`<p class="fine">${compareWith === null ? T('选择一轮进行比较。', 'Pick a run to compare with.') : T('正在比较……', 'Comparing…')}</p>`;
  else {
    const c = cmp.value;
    const [a, b] = c.runs;
    if (!a || !b) bodyHtml = html``;
    else {
      const pts = c.accuracy_change_points;
      bodyHtml = html`${ticks(a.accuracy.total, a.accuracy.correct, '', `#${a.run.id}`, monthDay(a.run.started_at))}${ticks(b.accuracy.total, b.accuracy.correct, 'now', `#${b.run.id}`, monthDay(b.run.started_at))}
       <div class="delta" data-testid="compare"><span class="d1 num">${pts > 0 ? '+' : ''}${pts.toFixed(1)}</span><span>${T('个百分点', 'points')}</span>
         <span class="chk">${ic('clock', 'xs')}${T('中位用时', 'median')} ${c.median_change_ms > 0 ? '+' : ''}${sec(c.median_change_ms)} ${T('秒', 's')}</span>
         <span class="chk">${ic('check', 'xs')}${T(`同样 ${c.dimensions['item_count'] ?? '—'} 题`, `same ${c.dimensions['item_count'] ?? '—'} items`)}</span>
         <span class="chk">${ic('check', 'xs')}${T(`同样 ${c.dimensions['window_minutes'] ?? '—'} 分钟`, `same ${c.dimensions['window_minutes'] ?? '—'} min`)}</span>
         <span class="chk">${ic('check', 'xs')}${T('同一题池族', 'same pool family')}</span></div>
       <p class="fine">${T('两次本地样本的同口径比较，不说明任何变化是由 AI 或辅导造成的。', 'A like-for-like comparison of two local samples; it does not show that AI or coaching caused any change.')}</p>`;
    }
  }
  return html`<div class="cmp">
        <div class="cmp-h"><span class="eyebrow">${T('同口径比较', 'Like-for-like')}</span>${seg}</div>
        ${bodyHtml}
      </div>`;
}

export function vReport(r: AssessmentReport | null, runs: AssessmentRun[], compareWith: number | null, cmp: Loadable<AssessmentComparison>): Html {
  if (!r)
    return stateCard({
      ico: 'report',
      eyebrow: T('报告', 'Report'),
      title: T('还没有结束的测评', 'No finished assessment yet'),
      body: T('完成一轮未见测评后，这里显示观察到的结果。及格线未知，不作预测。', 'After a finished unseen run this shows the observed result. The pass threshold is unknown; nothing is predicted.'),
      action: html`<a class="btn primary" href="#/assess">${T('去测评', 'Go to assessment')}${ic('arrow', 'xs')}</a>`,
    });
  const others = runs.filter((x) => x.id !== r.run.id && x.status === 'completed').slice(0, 3);
  const s = T('秒', 's');
  const annotated = r.items.filter((i) => i.annotations.available && !i.annotations.withheld).length;
  const status = r.run.status === 'completed' ? T('已完成', 'complete') : r.run.status === 'abandoned' ? T('已放弃', 'abandoned') : T('进行中', 'active');
  const runPick =
    runs.filter((x) => x.status !== 'active').length > 1
      ? html`<div class="seg2" style="width:max-content;margin-top:8px" role="group" aria-label="${T('选择测评', 'Pick a run')}">${runs
          .filter((x) => x.status !== 'active')
          .slice(0, 5)
          .map((x) => html`<button data-act="report-run" data-v="${x.id}" aria-pressed="${x.id === r.run.id}">#${x.id}</button>`)}</div>`
      : '';
  return html`<div class="scene orbit" style="--obj-w:640px" data-testid="report" data-run="${r.run.id}">
    <div class="rail l">
      ${sat('time', 'clock', T('中位用时', 'Median time'), html`<span class="num">${sec(r.speed.basis_items ? r.speed.median_ms : null)}</span><small>${s} · p90 <span class="num">${sec(r.speed.basis_items ? r.speed.p90_ms : null)}</span> ${s}</small>`, '', 1)}
      ${sat('timeout', 'tout', T('超时', 'Timed out'), html`<span class="num">${r.counts.timed_out}</span><small>${T('题 · 不计分', 'items · not scored')}</small>`, '', 2)}
      ${sat('contam', 'shield', T('污染检查', 'Contamination'), html`<span class="num">${r.counts.contaminated}</span><small>${T('题被排除', 'items excluded')}</small>`, '', 3)}
    </div>
    <article class="obj card rcard rise" style="--i:1" aria-labelledby="rep-t">
      <div class="obj-top"><span class="eyebrow" id="rep-t">${T(`测评第 ${r.run.id} 轮 · ${status}`, `Assessment run #${r.run.id} · ${status}`)}</span><span class="r"><span class="nw">${shortDate(
        r.run.completed_at ?? r.run.abandoned_at ?? r.run.started_at,
      )}</span><span class="sep"></span>${T(`${r.run.actual_size} 题 · ${r.run.window_minutes} 分钟`, `${r.run.actual_size} items · ${r.run.window_minutes} min`)}</span></div>
      ${runPick}
      <div class="score">
        <div class="n num" data-testid="score" aria-label="${T(`答对 ${r.counts.correct} / ${r.counts.scored}`, `${r.counts.correct} of ${r.counts.scored} correct`)}">${r.counts.correct}<small>/ ${r.counts.scored}</small></div>
        <div class="side"><span class="chip" id="pass-threshold" data-testid="pass-threshold" data-value="${r.pass_threshold}">${T('及格线：未知', 'Pass threshold: unknown')}</span><span class="t">${T('只报告观察到的结果，', 'Observed result only —')}<br>${T(
          `计分 ${r.counts.scored} 题 · 跳过 ${r.counts.skipped} · 超时 ${r.counts.timed_out}`,
          `${r.counts.scored} scored · ${r.counts.skipped} skipped · ${r.counts.timed_out} timed out`,
        )}</span></div>
      </div>
      ${comparison(r, others, compareWith, cmp)}
      <div class="skills">${r.skills.map(
        (k) =>
          html`<div class="sk"><span class="nm" title="${k.skill_id}">${skillName(k.skill_id)}</span><span class="v num">${k.correct}<small> / ${k.total}</small></span><span class="pips" aria-hidden="true">${Array.from(
            { length: Math.min(k.total, 40) },
            (_, i) => html`<i class="${i < k.correct ? 'on' : ''}"></i>`,
          )}</span></div>`,
      )}</div>
    </article>
    <div class="rail r">
      ${sat('ann', 'eye', T('逐题注释 · 已解锁', 'Annotations · unlocked'), html`<span class="num">${annotated}</span><small>${T('题 · 证据与解析', 'items · evidence & reasoning')}</small>`, 'ok', 4)}
      ${sat('resume', 'door', T('恢复', 'Resumes'), html`<span class="num">${r.run.resume_count}</span><small>${T(`次 · 恢复后作答 ${r.counts.answered_after_resume} 题`, `· ${r.counts.answered_after_resume} answered after`)}</small>`, '', 5)}
    </div>
  </div>`;
}

function annSheet(r: AssessmentReport, n: number | null): Html {
  const counts = new Map<ItemOutcome, number>();
  for (const it of r.items) counts.set(outcomeOf(it), (counts.get(outcomeOf(it)) ?? 0) + 1);
  const grid = html`<div class="rcells" role="group" aria-label="${T(`第 ${r.run.id} 轮逐题`, `Run #${r.run.id} items`)}">${r.items.map((it) => {
    const st = RSTATE[outcomeOf(it)];
    return html`<button class="rcell ${st[3]} ${n === it.position ? 'sel' : ''}" data-act="ann" data-v="${it.position}" data-outcome="${outcomeOf(it)}" aria-label="${T(`第 ${it.position + 1} 题 · ${st[0]}`, `Item ${it.position + 1} · ${st[1]}`)}" aria-pressed="${n === it.position}">${ic(st[2])}<span class="num">${it.position + 1}</span></button>`;
  })}</div>
    <div class="rleg">${(['correct', 'incorrect', 'skipped', 'timed_out', 'excluded', 'unavailable', 'pending'] as const)
      .filter((o) => o === 'correct' || o === 'incorrect' || counts.get(o))
      .map((o) => html`<span><span class="rcell ${RSTATE[o][3]}">${ic(RSTATE[o][2])}</span>${T(RSTATE[o][0], RSTATE[o][1])} <b class="num">${counts.get(o) ?? 0}</b></span>`)}</div>`;
  const it = n !== null ? r.items.find((x) => x.position === n) : undefined;
  let det = html`<p class="fine" style="margin-top:16px">${T('选一题查看证据与解析（测评中扣留，现在开放）。', 'Pick an item to see its evidence and reasoning (withheld during the run, open now).')}</p>`;
  if (it) {
    const out = outcomeOf(it);
    const st = RSTATE[out];
    const a = it.annotations;
    const reasons = Object.entries(a.option_reasoning ?? {}).sort(([x], [y]) => x.localeCompare(y));
    const missing = a.option_reasoning_missing ?? [];
    det = html`<div class="blk" style="margin-top:16px" data-testid="annotation" data-position="${it.position}">
      <h3>${T(`第 ${it.position + 1} 题`, `Item ${it.position + 1}`)}<span class="id">${it.question_id}</span><span class="chip ${out === 'correct' ? 'ok' : out === 'incorrect' ? 'bad' : ''} sm" style="margin-left:auto">${T(
        st[0],
        st[1],
      )}</span></h3>
      <p class="fine">${T(`用时 ${sec(it.elapsed_ms)} 秒`, `${sec(it.elapsed_ms)} s`)}${it.after_resume ? T(' · 恢复后作答', ' · answered after a resume') : ''}${
        it.contamination_reasons?.length
          ? html`${T(' · 已排除：', ' · excluded: ')}${it.contamination_reasons.map((c, i) => html`${i ? T('、', ', ') : ''}${contaminationLabel(c)}`)}`
          : ''
      }</p>
      ${
        a.withheld || !a.available
          ? html`<p class="fine">${a.withheld ? T('这一题的注释仍被扣留。', 'Annotations for this item are still withheld.') : T('这一题没有已编写的注释。', 'No annotations are authored for this item.')}</p>`
          : html`${a.evidence ? html`<div class="tactic" style="margin:12px 0 0;background:var(--glass-3)"><span class="ic">${ic('seal', 's')}</span><div class="k">${T('证据', 'Evidence')}</div><p class="exam" lang="en" style="font-size:15px;color:var(--ink)">“${a.evidence}”</p></div>` : ''}${
              a.reasoning ? html`<p class="fine" lang="en">${a.reasoning}</p>` : ''
            }
            <div style="display:grid;grid-template-columns:1fr;gap:6px;font-size:var(--t-sm)">${[
              ...reasons.map(([label, text]) => ({ label, row: html`<div><b>${label}</b> <span lang="en" class="muted">${text}</span></div>` })),
              ...missing.map((label) => ({ label, row: html`<div><b>${label}</b> <span class="muted">${T('原书未提供该选项的解析。', 'No explanation provided for this option.')}</span></div>` })),
            ]
              .sort((x, y) => x.label.localeCompare(y.label))
              .map((x) => x.row)}</div>
            ${a.glossary?.length ? html`<p class="fine">${T('词表', 'Glossary')}：${a.glossary.map((g, i) => html`${i ? ' · ' : ''}<b lang="en">${g.term}</b> ${g.meaning}`)}</p>` : ''}`
      }
    </div>`;
  }
  return sheetWrap('eye', T(`逐题注释 · 第 ${r.run.id} 轮`, `Annotations · run #${r.run.id}`), T('证据与解析在测评中被扣留，现在可逐题查看', 'Evidence and reasoning were withheld during the run'), html`${grid}${det}`, true);
}

function speedBand(b: string): string {
  switch (b) {
    case 'comfortable':
      return T('宽松（中位用时不到每题上限的一半）', 'comfortable (median under half the per-item limit)');
    case 'steady':
      return T('平稳（中位用时在上限的一半到四分之三）', 'steady (median between half and three quarters of the limit)');
    case 'pressed':
      return T('吃紧（中位用时超过上限的四分之三）', 'pressed (median above three quarters of the limit)');
    default:
      return T('无法判断', 'not determined');
  }
}

export function reportSheet(key: string, r: AssessmentReport, annItem: number | null): Html | null {
  const s = T('秒', 's');
  const timedOut = r.items.filter((i) => i.timed_out).map((i) => i.position + 1);
  switch (key) {
    case 'ann':
      return annSheet(r, annItem);
    case 'time':
      return sheetWrap(
        'clock',
        T('时间', 'Time'),
        T(`第 ${r.run.id} 轮 · 每题上限 ${sec(r.run.per_item_timeout_ms)} 秒`, `Run #${r.run.id} · ${sec(r.run.per_item_timeout_ms)} s per item`),
        html`<div class="kv"><span>${T('中位用时', 'Median')}</span><b class="big num">${sec(r.speed.basis_items ? r.speed.median_ms : null)} <small style="font-size:14px">${s}</small></b></div>
        <div class="kv"><span>p90</span><b class="big num">${sec(r.speed.basis_items ? r.speed.p90_ms : null)} <small style="font-size:14px">${s}</small></b></div>
        ${kv(T('依据', 'Basis'), T(`${r.speed.basis_items} 题`, `${r.speed.basis_items} items`))}
        ${kv(T('节奏', 'Pace'), speedBand(r.speed.band))}
        ${kv(T('超时', 'Timed out'), timedOut.length ? T(`${timedOut.length} 题 · 第 ${timedOut.join('、')} 题`, `${timedOut.length} items · ${timedOut.join(', ')}`) : '0')}`,
      );
    case 'timeout':
      return sheetWrap(
        'tout',
        T('超时', 'Timed out'),
        T('超时的题不计分', 'Timed-out items are not scored'),
        html`${kv(T('超时', 'Timed out'), r.counts.timed_out, true)}${kv(T('跳过', 'Skipped'), r.counts.skipped)}${kv(T('不可用', 'Unavailable'), r.counts.unavailable)}${
          timedOut.length ? html`<p class="fine">${T(`第 ${timedOut.join('、')} 题`, `Items ${timedOut.join(', ')}`)}</p>` : ''
        }`,
      );
    case 'contam': {
      const c = r.contamination;
      return sheetWrap(
        'shield',
        T('污染检查', 'Contamination check'),
        T('运行前见过的题会被列出并排除计分', 'Items seen before the run are listed and excluded'),
        html`${kv(T('本轮被排除', 'Excluded this run'), r.counts.contaminated, true)}
        ${kv(T('运行前见过', 'Seen before the run'), c.prior_exposure)}
        ${kv(T('重复变体', 'Repeated variants'), c.repeated_variant)}
        ${kv(T('运行中看过提示', 'Hints during the run'), c.hints_during_run)}
        ${kv(T('超过时限后恢复', 'Resumed after the window'), c.resumed_after_window)}
        ${c.excluded_items.length ? html`<p class="fine">${T('排除：', 'Excluded: ')}${c.excluded_items.map((i) => `#${i.position + 1}`).join(' ')}</p>` : ''}`,
      );
    }
    case 'resume':
      return sheetWrap(
        'door',
        T('恢复', 'Resumes'),
        T('离开后计时不暂停', 'The clock never pauses while away'),
        html`${kv(T('恢复次数', 'Resumes'), r.run.resume_count, true)}${kv(T('恢复后作答', 'Answered after a resume'), r.counts.answered_after_resume)}${
          r.run.last_resumed_at ? kv(T('最近一次恢复', 'Last resumed'), shortDate(r.run.last_resumed_at)) : ''
        }`,
      );
  }
  return null;
}

export function reportPopLines(r: AssessmentReport, cmp: Loadable<AssessmentComparison>): string[] {
  const lines: string[] = [];
  if (cmp.status === 'ready') {
    const [a, b] = cmp.value.runs;
    if (a && b)
      lines.push(
        T(
          `和第 ${a.run.id} 轮同口径：正确率变化 ${cmp.value.accuracy_change_points.toFixed(1)} 个百分点。`,
          `Like-for-like with run #${a.run.id}: accuracy changed ${cmp.value.accuracy_change_points.toFixed(1)} points.`,
        ),
      );
  } else if (cmp.status === 'error') lines.push(errorTitle(cmp.error.code));
  lines.push(T(`本轮计分 ${r.counts.scored} 题，答对 ${r.counts.correct} 题。`, `${r.counts.correct} of ${r.counts.scored} scored items correct.`));
  lines.push(T('及格线未知：这里只有观察到的结果。', 'The pass threshold is unknown: this is observation only.'));
  return lines;
}
