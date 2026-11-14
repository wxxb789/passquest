// Today (prototype vToday + the today sheets): the one next action with its reason and evidence,
// and quiet satellites for the other skills, the last run, the pool, Anki and the study brief.
// Every number is the core's; what the core does not report is shown as unknown.
import type { AssessmentRun, BriefSummary, Recommendation, Services, SkillEvidence, Target, Today } from '../api/types';
import { html, type Html } from '../core/html';
import {
  actName,
  briefStateName,
  causalDetail,
  causalText,
  monthDay,
  policyReason,
  policyStateName,
  shortDate,
  skillKindName,
  skillName,
  T,
} from '../core/i18n';
import type { Loadable } from '../core/state';
import { cstIco } from '../components/constellation';
import { dots, evDots } from '../components/evdots';
import { sat, type Flag } from '../components/satellite';
import { kv, sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { stateCard } from './states';

const miss = (s: SkillEvidence) => s.evidence.independent_incorrect;
const sname = (s: SkillEvidence) => skillName(s.skill.id, s.skill.title);

function facts(t: Today): Html {
  const p = t.prep;
  const date = p.exam_date || T('未设置', 'not set');
  const daily = p.daily_minutes > 0 ? T(`${p.daily_minutes} 分钟`, `${p.daily_minutes} min`) : T('未设置', 'not set');
  const f = t.find;
  const find = f.configured
    ? T(`已配置 · 今日 ${f.queries_used} / ${f.queries_limit} 次查询`, `configured · ${f.queries_used} / ${f.queries_limit} queries today`)
    : T(`未配置搜索 · ${f.queries_used} 次网络调用`, `no search provider · ${f.queries_used} network calls`);
  return html`<div class="facts settle" style="--i:0" data-testid="facts">
      <span class="chip" data-testid="exam-date">${ic('clock', 'xs')}<span class="k">${T('考试日期', 'Exam date')}</span>${date}<button class="lnk" data-act="learner">${T('设置', 'Set')}</button></span>
      <span class="chip" data-testid="daily-minutes"><span class="k">${T('每日学习时长', 'Daily study time')}</span>${daily}</span>
      <span class="chip" data-testid="today-pass-threshold"><span class="k">${T('及格线', 'Pass threshold')}</span>${T('未知', 'unknown')}</span>
      <span class="chip">${ic('search', 'xs')}<span class="k">${T('找练习', 'Find practice')}</span>${find}</span>
    </div>`;
}

function skillSat(s: SkillEvidence, i: number): Html {
  const e = s.evidence;
  return sat(
    `skill:${s.skill.id}`,
    cstIco(e.independent_correct, miss(s), e.assisted_attempts),
    sname(s),
    html`<span class="num">${e.independent_correct} / ${e.independent_correct + e.independent_incorrect}</span><small>→ ${actName(s.inference.action)}</small>`,
    '',
    i,
  );
}

function briefLine(b: BriefSummary | null): string {
  if (!b) return T('没有进行中的简报', 'none in progress');
  return `${briefStateName(b.state)} · ${b.step} / ${b.max_steps}`;
}

/**
 * The practice target Start opens (`next`): the recommendation's target, or for a lesson its
 * follow-up, exactly as the core's selectNextPractice swaps it. Null when nothing can be opened.
 */
export function openedTarget(r: Recommendation): Target | null {
  const t = r.target.kind === 'lesson' ? r.follow_up : r.target;
  return t && (t.kind === 'tactic' || t.kind === 'skill') ? t : null;
}
export const opensItem = (r: Recommendation): boolean => openedTarget(r) !== null;

/**
 * What the item Start opens will record, read from that target (not from the policy action): a
 * guided tactic shows the walkthrough (assisted), a pinned seen question is a revisit, a skill
 * target on fresh material can count. The item restates its condition before any answer.
 */
function promise(r: Recommendation): Html {
  const t = openedTarget(r);
  const box = (canCount: 'true' | 'false' | 'none', head: string, sub: string) =>
    html`<div class="promise ${canCount === 'true' ? '' : 'warm'}" data-testid="promise" data-can-count="${canCount}" data-target="${t?.kind ?? 'none'}"><span class="glow"></span><span><b>${head}</b>${sub}</span></div>`;
  if (!t)
    return box(
      'none',
      T('只有讲解 · 没有可打开的题', 'A lesson only · no item to open'),
      T(`讲解在命令行：passquest skill show ${r.skill.id}；也可以选其他练习`, `The lesson is in the CLI: passquest skill show ${r.skill.id}; or choose other practice`),
    );
  // The lesson itself is not shown in this window; say so instead of promising an explanation.
  const skipped = r.target.kind === 'lesson' ? T(`讲解不在这里显示（passquest skill show ${r.skill.id}）· `, `The lesson is not shown here (passquest skill show ${r.skill.id}) · `) : '';
  if (t.kind === 'tactic') {
    if (t.stage === 'guided')
      return box('false', T('引导讲解 · 记为有辅助', 'Guided walkthrough · recorded as assisted'), T('讲解可见的那一题不计入', 'The item shown with the walkthrough will not count'));
    if (t.stage === 'independent')
      return box('true', T('独立战术练习 · 可以计入', 'Independent tactic practice · can count'), T('答题前会再次显示本题的条件', 'The item shows its condition again before you answer'));
    return box('false', T('条件未知', 'Unknown condition'), T('答题前会显示本题的条件', 'The item shows its condition before you answer'));
  }
  if (t.question_id)
    return box('false', T('见过的题 · 不计入', 'A seen question · will not count'), skipped + T('重做不算独立迁移', 'A repeat is not independent transfer'));
  return box('true', T('新文章 · 可以计入', 'Fresh passage · can count'), skipped + T('答题前会再次显示本题的条件', 'The item shows its condition again before you answer'));
}

function hero(t: Today, busy: boolean): Html {
  const r = t.recommendation;
  if (!r)
    return html`<article class="obj hero card rise" style="--i:1" aria-labelledby="hero-t" data-testid="hero">
      <div class="obj-top"><span class="eyebrow">${T('下一步', 'Next')}</span><span class="r">${ic('local', 'xs')}${T('透明策略 · 本地计算', 'Transparent policy · local')}</span></div>
      <h1 class="hero-title h-disp" id="hero-t">${T('还没有可推荐的练习', 'No practice to recommend yet')}</h1>
      <p class="reason">${T('策略需要可练习的内容。可以先做内置示例，或导入并审核一份本地资料。', 'The policy needs practice content. Start with the built-in sample, or import and review a local source.')}</p>
      <div class="obj-foot"><span></span><span class="acts-row"><button class="btn ghost" data-act="sheet" data-v="pick">${T('选择练习…', 'Choose practice…')}</button><button class="btn primary lg" data-act="start" data-v="sample" id="start" ${busy ? 'disabled' : ''}>${T('练习内置示例', 'Practice the sample')}<kbd>Enter</kbd></button></span></div>
    </article>`;
  const ev = t.skills.find((s) => s.skill.id === r.skill.id) ?? null;
  const evid = ev
    ? html`<div class="evid" role="group" aria-label="${T('证据', 'Evidence')}">
        <div><div class="k"><b>${T('独立迁移', 'Independent')}</b><span class="num">${ev.evidence.independent_correct} / ${ev.evidence.independent_correct + ev.evidence.independent_incorrect}</span></div>
          <div class="dots">${dots('ok', ev.evidence.independent_correct)}${dots('miss', miss(ev))}</div>
          <div class="v cnt">${T('计入', 'Counts')}</div></div>
        <div><div class="k"><b>${T('有辅助', 'Assisted')}</b><span class="num">${ev.evidence.assisted_attempts}</span></div>
          <div class="dots">${dots('as', ev.evidence.assisted_attempts)}</div>
          <div class="v no">${T('不计入', 'Never counts')}</div></div>
        <div><div class="k"><b>${T('重复接触', 'Re-exposure')}</b><span class="num">${ev.evidence.passage_repeats}</span></div>
          <div class="dots">${dots('re', ev.evidence.passage_repeats)}</div>
          <div class="v no">${T('不计入', 'Never counts')}</div></div>
      </div>
      <div class="diag" data-causal="${ev.inference.causal_diagnosis}"><span class="q">${causalText(ev.inference.causal_diagnosis)}</span>${causalDetail(ev.inference.causal_diagnosis)}</div>`
    : '';
  return html`<article class="obj hero card rise" style="--i:1" aria-labelledby="hero-t" data-testid="hero" data-action="${r.action}" data-skill="${r.skill.id}">
      <div class="obj-top">
        <span class="eyebrow">${T('下一步', 'Next')}</span><span class="pol">${actName(r.action)}<span class="id">${policyStateName(r.state)}</span></span>
        <span class="r" title="${r.command}">${ic('local', 'xs')}${T('透明策略 · 本地计算', 'Transparent policy · local')}</span>
      </div>
      <h1 class="hero-title h-disp" id="hero-t">${skillName(r.skill.id, r.skill.title)}</h1>
      <div class="hero-sub">${skillKindName(r.skill.kind)}<span class="sep"></span><span class="id">${r.skill.id}</span></div>
      <p class="reason" data-testid="reason">${policyReason(r.reason, ev)}</p>
      ${evid}
      <div class="obj-foot">
        ${promise(r)}
        <span class="acts-row"><button class="btn ghost" data-act="sheet" data-v="pick">${T('其他练习…', 'Other practice…')}</button><button class="btn primary lg" data-act="start" data-v="recommended" id="start" ${busy || !opensItem(r) ? 'disabled' : ''}>${T('开始练习', 'Start practice')}<kbd>Enter</kbd></button></span>
      </div>
    </article>`;
}

function runLine(r: AssessmentRun): Html {
  const st = r.status === 'completed' ? T('已完成', 'complete') : r.status === 'abandoned' ? T('已放弃', 'abandoned') : T('进行中', 'active');
  return html`#${r.id} <small>${st} · ${T(`${r.actual_size} 题`, `${r.actual_size} items`)}</small>`;
}

export function vToday(t: Today, busy = false): Html {
  if (t.active_run)
    return stateCard({
      ico: 'assess',
      eyebrow: T('测评进行中', 'Assessment in progress'),
      title: T(`第 ${t.active_run.id} 轮测评尚未结束`, `Run #${t.active_run.id} is not finished`),
      body: T('测评期间不能练习、看导师或改模型。计时一直在走。', 'Practice, coach and model changes wait until the run ends. Its clock keeps running.'),
      code: 'assessment_active',
      action: html`<a class="btn primary lg" href="#/assess" data-testid="back-to-run">${T('回到测评', 'Back to the run')}${ic('arrow', 'xs')}</a>`,
    });
  const r = t.recommendation;
  const others = t.skills.filter((s) => s.skill.id !== r?.skill.id && s.inference.state !== 'no_material').slice(0, 2);
  const last = t.last_run;
  const pool = t.pool;
  const a = t.anki;
  const ankiFlag: Flag = a.conflict > 0 ? 'bad' : a.queued > 0 ? 'warm' : '';
  const briefFlag: Flag = t.brief?.state === 'paused' || t.brief?.state === 'needs_action' ? 'warm' : t.brief?.state === 'failed' ? 'bad' : '';
  return html`<div class="scene orbit" data-testid="today">
    ${facts(t)}
    <div class="rail l">
      ${others.map((s, i) => skillSat(s, i + 1))}
      ${sat(
        'run',
        'report',
        last ? T(`上次测评 · ${monthDay(last.started_at)}`, `Last run · ${monthDay(last.started_at)}`) : T('上次测评', 'Last run'),
        last ? runLine(last) : html`<small>${T('还没有测评', 'none yet')}</small>`,
        '',
        3,
      )}
    </div>
    ${hero(t, busy)}
    <div class="rail r">
      ${sat(
        'pool',
        'lock',
        T('未见测评池', 'Unseen pool'),
        pool.source_count > 0
          ? html`<span class="num">${pool.qualifying_items}</span><small>${T(`题 · ${pool.qualifying_units} 单元`, `items · ${pool.qualifying_units} units`)}</small>`
          : html`<small>${T('没有已审核的来源', 'no reviewed source')}</small>`,
        pool.source_count > 0 && !pool.floor_met ? 'warm' : '',
        4,
      )}
      ${sat(
        'anki',
        'anki',
        'Anki',
        html`<span class="num">${a.total}</span><small>${T(`条笔记 · ${a.queued} 排队 · ${a.conflict} 冲突`, `notes · ${a.queued} queued · ${a.conflict} conflict`)}</small>`,
        ankiFlag,
        5,
      )}
      ${sat('brief', t.brief?.state === 'running' ? 'pulse' : 'pause', T('学习简报', 'Study brief'), briefLine(t.brief), briefFlag, 6)}
    </div>
  </div>`;
}

// ---------------------------------------------------------------- sheets

function skillSheet(s: SkillEvidence, t: Today): Html {
  const recommended = t.recommendation?.skill.id === s.skill.id;
  const e = s.evidence;
  const answered = e.independent_correct + e.independent_incorrect;
  return sheetWrap(
    'pulse',
    sname(s),
    html`<span class="id">${s.skill.id}</span><span class="sep"></span>${skillKindName(s.skill.kind)}`,
    html`<div class="cols2">
      <div class="blk"><h3>${T('证据', 'Evidence')}</h3><div style="display:flex;gap:6px;margin:10px 0 8px;flex-wrap:wrap">${evDots(answered, e.independent_correct, e.assisted_attempts, e.passage_repeats)}</div>
        <p>${T(
          `独立迁移 ${answered} 次，答对 ${e.independent_correct} 次 · 有辅助 ${e.assisted_attempts} 次 · 重复接触 ${e.passage_repeats} 次 · 重做 ${e.revisits} 次（后三者不计入）`,
          `${answered} independent transfers, ${e.independent_correct} correct · assisted ${e.assisted_attempts} · re-exposure ${e.passage_repeats} · revisits ${e.revisits} (none of the last three counts)`,
        )}</p></div>
      <div class="blk"><h3>${T('策略建议', 'Policy action')}</h3><p style="font-size:var(--t-sm);color:var(--ink)">${actName(s.inference.action)} <span class="id">${policyStateName(s.inference.state)}</span></p>
        <p>${causalText(s.inference.causal_diagnosis)}${causalDetail(s.inference.causal_diagnosis) ? html` · ${causalDetail(s.inference.causal_diagnosis)}` : ''}</p></div>
    </div>
    <p class="fine">${policyReason(s.inference.reason, s)}</p>
    ${
      recommended || s.inference.state === 'no_material'
        ? ''
        : html`<div class="sh-acts"><button class="btn ghost sm" data-act="start-skill" data-v="${s.skill.id}">${T('练习这个技能', 'Practice this skill')}${ic('arrow', 'xs')}</button></div>`
    }`,
  );
}

function briefSheet(b: BriefSummary | null, services: Loadable<Services>): Html {
  if (!b) return sheetWrap('pause', T('学习简报', 'Study brief'), '', html`<p class="fine">${T('没有未完成的简报任务。', 'No unfinished brief run.')}</p>`);
  const stepBar = Array.from({ length: b.max_steps }, (_, i) => html`<i class="${i < b.step ? '' : 'off'}"></i>`);
  const node = services.status === 'ready' ? services.value.node_available : null;
  const canResume = b.state === 'paused' || b.state === 'needs_action';
  const acts = canResume
    ? html`<button class="btn primary sm" data-act="brief-resume" ${node === false ? 'disabled' : ''}>${ic('play', 'xs')}${T('继续', 'Resume')}</button><button class="btn quiet sm" data-act="brief-cancel">${T('取消任务', 'Cancel run')}</button>`
    : b.state === 'running'
      ? html`<button class="btn quiet sm" data-act="brief-cancel">${T('取消任务', 'Cancel run')}</button>`
      : '';
  const nodeLine =
    node === null
      ? html` · <span class="muted">${T('Node 未检查', 'Node not checked')}</span>`
      : node
        ? ''
        : html` · <span class="held">${T('Node 不可用', 'Node unavailable')}</span>`;
  return sheetWrap(
    b.state === 'running' ? 'pulse' : 'pause',
    T('学习简报', 'Study brief'),
    html`<span class="id">${b.run_id}</span><span class="sep"></span>${T('耐久任务 · 可恢复', 'durable run · resumable')}`,
    html`${kv(T('状态', 'State'), `${briefStateName(b.state)} · ${T(`第 ${b.step} / ${b.max_steps} 步`, `step ${b.step} / ${b.max_steps}`)}`)}
      <div class="seg-bar" style="margin-bottom:14px">${stepBar}</div>
      <div class="ledger">
        <div><span class="lk">${T('执行', 'Runs on')}</span><span class="lv">${ic('local', 'xs')} ${T('本地 Agent 宿主（Node）', 'Local agent host (Node)')}${nodeLine}</span></div>
        <div><span class="lk">${T('调用', 'Calls')}</span><span class="lv">${
          b.provider ? html`<span class="ext">${ic('ext', 'xs')}${T('外部', 'External')}</span> · ${b.provider} · <code>${b.model}</code>` : html`<span class="muted">${T('未记录', 'not recorded')}</span>`
        }</span></div>
        <div><span class="lk">${T('开始', 'Started')}</span><span class="lv">${shortDate(b.created_at)}</span></div>
      </div>
      <div class="sh-acts">${acts}</div>`,
  );
}

/** Practice picker: the recommendation, a skill, a tactic (guided or independent) or the sample. */
function pickSheet(t: Today): Html {
  const rows = t.skills
    .filter((s) => s.inference.state !== 'no_material')
    .map(
      (s) => html`<div class="svc" data-skill="${s.skill.id}"><span class="state-dot ${s.skill.kind === 'scoring' ? 'ok' : 'off'}"></span><span>${sname(s)} <span class="muted">${skillKindName(s.skill.kind)}</span></span><span class="st acts-row">
        <button class="btn ghost sm" data-act="start-skill" data-v="${s.skill.id}">${T('技能练习', 'Skill practice')}</button>${
          s.skill.kind === 'scoring'
            ? html`<button class="btn ghost sm" data-act="start-tactic" data-v="${s.skill.id}|guided">${T('战术 · 引导', 'Tactic · guided')}</button><button class="btn ghost sm" data-act="start-tactic" data-v="${s.skill.id}|independent">${T('战术 · 独立', 'Tactic · independent')}</button>`
            : ''
        }</span></div>`,
    );
  return sheetWrap(
    'practice',
    T('选择练习', 'Choose practice'),
    T('打开一道题就会记录它的展示时间', 'Opening an item records when it was shown'),
    html`${t.recommendation && opensItem(t.recommendation) ? html`<div class="svc"><span class="state-dot ok"></span><span>${T('策略推荐', 'The recommendation')} · ${skillName(t.recommendation.skill.id, t.recommendation.skill.title)}</span><span class="st"><button class="btn primary sm" data-act="start" data-v="recommended">${T('开始', 'Start')}</button></span></div>` : ''}
      ${rows}
      <div class="svc"><span class="state-dot off"></span><span>${T('内置示例', 'Built-in sample')}</span><span class="st"><button class="btn ghost sm" data-act="start" data-v="sample">${T('开始', 'Start')}</button></span></div>
      <p class="fine">${T('引导阶段会显示战术讲解，那一题记为有辅助。', 'The guided stage shows the tactic walkthrough; that item is recorded as assisted.')}</p>`,
  );
}

export function todaySheet(key: string, t: Today, services: Loadable<Services>): Html | null {
  if (key.startsWith('skill:')) {
    const s = t.skills.find((x) => x.skill.id === key.slice(6));
    return s ? skillSheet(s, t) : null;
  }
  switch (key) {
    case 'pick':
      return pickSheet(t);
    case 'run': {
      const r = t.last_run;
      if (!r) return sheetWrap('report', T('上次测评', 'Last assessment'), '', html`<p class="fine">${T('还没有结束的测评。', 'No finished assessment yet.')}</p>`);
      return sheetWrap(
        'report',
        T(`上次测评 · 第 ${r.id} 轮`, `Last assessment · run #${r.id}`),
        shortDate(r.started_at),
        html`${kv(T('状态', 'State'), r.status === 'completed' ? T('已完成', 'complete') : T('已放弃', 'abandoned'))}
        ${kv(T('规模', 'Size'), T(`${r.actual_size} 题 · ${r.window_minutes} 分钟`, `${r.actual_size} items · ${r.window_minutes} min`))}
        ${kv(T('及格线', 'Pass threshold'), T('未知 · 不作预测', 'unknown · no prediction'))}
        <div class="sh-acts"><a class="btn ghost sm" href="#/report">${T('查看报告', 'See the report')}${ic('arrow', 'xs')}</a></div>`,
      );
    }
    case 'pool': {
      const p = t.pool;
      if (p.source_count === 0)
        return sheetWrap(
          'lock',
          T('未见测评池', 'Unseen assessment pool'),
          T('只有没见过的题才能用于测评', 'Only unseen items can be used to assess'),
          html`<p class="fine">${T('还没有已审核的导入来源，因此没有测评池。', 'No reviewed imported source exists, so there is no pool yet.')}</p>`,
        );
      const total = p.qualifying_units + p.spent_units;
      return sheetWrap(
        'lock',
        T('未见测评池', 'Unseen assessment pool'),
        T('只有没见过的题才能用于测评', 'Only unseen items can be used to assess'),
        html`<div class="kv"><span>${T('剩余', 'Remaining')}</span><b class="big num">${p.qualifying_items} <small style="font-size:14px;color:var(--ink-3)">${T(`题 / ${p.qualifying_units} 单元`, `items / ${p.qualifying_units} units`)}</small></b></div>
        <div class="seg-bar" aria-hidden="true">${Array.from({ length: Math.min(total, 120) }, (_, i) => html`<i class="${i >= p.qualifying_units ? 'off' : ''}"></i>`)}</div>
        <div class="kv" style="margin-top:8px"><span>${T('已用', 'Spent')}</span><b class="num">${T(`${p.spent_units} 单元`, `${p.spent_units} units`)}</b></div>
        ${kv(T('下限', 'Floor'), T(`${p.floor_items} 题`, `${p.floor_items} items`))}
        ${p.floor_met ? '' : kv(T('还差', 'Short by'), T(`${p.shortfall_items} 题`, `${p.shortfall_items} items`))}
        <p class="fine">${T('练习过的文章永远不会回到测评池：独立 ≠ 重复接触。', 'A passage you practised never returns to the pool: independent ≠ re-exposure.')}</p>
        <div class="sh-acts"><a class="btn ghost sm" href="#/assess">${T('去测评', 'Go to assessment')}${ic('arrow', 'xs')}</a></div>`,
      );
    }
    case 'anki': {
      const a = t.anki;
      return sheetWrap(
        'anki',
        'Anki',
        T('Pass Quest 只提议，你决定', 'Pass Quest proposes; you decide'),
        html`${kv(T('本地笔记', 'Local notes'), a.total)}
        ${kv(T('已推送', 'Pushed'), a.pushed)}
        ${kv(T('已存在', 'Already present'), a.already_present)}
        <div class="kv"><span>${T('排队 · 可连接时推送一次', 'Queued · pushes once when reachable')}</span><b class="num" style="color:var(--warm)">${a.queued}</b></div>
        <div class="kv"><span>${T('冲突 · 你在 Anki 里改过，不会被静默覆盖', 'Conflict · edited in Anki, never overwritten silently')}</span><b class="num" style="color:var(--bad)">${a.conflict}</b></div>
        ${a.failed + a.deck_mismatch ? kv(T('失败 / 牌组不符', 'Failed / wrong deck'), `${a.failed} / ${a.deck_mismatch}`) : ''}
        <div class="sh-acts"><a class="btn ghost sm" href="#/anki">${T('打开 Anki 笔记', 'Open Anki notes')}${ic('arrow', 'xs')}</a></div>`,
      );
    }
    case 'brief':
      return briefSheet(t.brief, services);
  }
  return null;
}

/** Lines for the coach popover on Today: local policy text, not model text. */
export function todayPopLines(t: Today): string[] {
  const lines: string[] = [];
  const r = t.recommendation;
  if (r) lines.push(policyReason(r.reason, t.skills.find((s) => s.skill.id === r.skill.id) ?? null));
  lines.push(T('作答时我不说话；需要时按 H 看提示，但那一题就不计入。', 'I stay quiet while you answer; H opens a hint, but then that item will not count.'));
  return lines;
}
