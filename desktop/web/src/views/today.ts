// Today (prototype vToday + the today sheets): the one next action with its reason and evidence,
// and quiet satellites for the other skills, the last run, the pool, Anki and the study brief.
import type { BriefSummary, SkillEvidence, Today } from '../api/types';
import { html, type Html } from '../core/html';
import { actName, BRIEF_STATE, monthDay, shortDate, skillKindName, skillName, T, TP } from '../core/i18n';
import { cstIco } from '../components/constellation';
import { dots, evDots } from '../components/evdots';
import { sat, type Flag } from '../components/satellite';
import { kv, sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { stateCard } from './states';

const miss = (s: SkillEvidence) => Math.max(0, s.independent - s.independent_correct);

function facts(t: Today): Html {
  const date = t.learner.exam_date ?? T('未设置', 'not set');
  const daily = t.learner.daily_minutes !== null ? T(`${t.learner.daily_minutes} 分钟`, `${t.learner.daily_minutes} min`) : T('未设置', 'not set');
  const find = t.find.configured
    ? T(`已配置 · 今日 ${t.find.calls_today} 次网络调用`, `configured · ${t.find.calls_today} network calls today`)
    : T(`未配置搜索 · ${t.find.calls_today} 次网络调用`, `no search provider · ${t.find.calls_today} network calls`);
  return html`<div class="facts settle" style="--i:0">
      <span class="chip">${ic('clock', 'xs')}<span class="k">${T('考试日期', 'Exam date')}</span>${date}<button class="lnk" data-act="learner">${T('设置', 'Set')}</button></span>
      <span class="chip"><span class="k">${T('每日学习时长', 'Daily study time')}</span>${daily}</span>
      <span class="chip"><span class="k">${T('及格线', 'Pass threshold')}</span>${T('未知', 'unknown')}</span>
      <span class="chip">${ic('search', 'xs')}<span class="k">${T('找练习', 'Find practice')}</span>${find}</span>
    </div>`;
}

function skillSat(s: SkillEvidence, i: number): Html {
  return sat(
    `skill:${s.skill_id}`,
    cstIco(s.independent_correct, miss(s), s.assisted),
    skillName(s.skill_id),
    html`<span class="num">${s.independent_correct} / ${s.independent}</span><small>→ ${actName(s.action)}</small>`,
    '',
    i,
  );
}

function briefLine(b: BriefSummary | null): string {
  if (!b) return T('没有进行中的简报', 'none in progress');
  return `${TP(BRIEF_STATE[b.state])} · ${b.step} / ${b.max_steps}`;
}

function hero(t: Today): Html {
  const r = t.recommendation;
  if (!r)
    return html`<article class="obj hero card rise" style="--i:1" aria-labelledby="hero-t">
      <div class="obj-top"><span class="eyebrow">${T('下一步', 'Next')}</span><span class="r">${ic('local', 'xs')}${T('透明策略 · 本地计算', 'Transparent policy · local')}</span></div>
      <h1 class="hero-title h-disp" id="hero-t">${T('还没有可推荐的练习', 'No practice to recommend yet')}</h1>
      <p class="reason">${T('策略需要可练习的内容。可以先做内置示例，或导入并审核一份本地资料。', 'The policy needs practice content. Start with the built-in sample, or import and review a local source.')}</p>
      <div class="obj-foot"><span></span><button class="btn primary lg" data-act="start" data-v="sample" id="start">${T('练习内置示例', 'Practice the sample')}<kbd>Enter</kbd></button></div>
    </article>`;
  const ev = t.skills.find((s) => s.skill_id === r.skill_id) ?? null;
  const evid = ev
    ? html`<div class="evid" role="group" aria-label="${T('证据', 'Evidence')}">
        <div><div class="k"><b>${T('独立迁移', 'Independent')}</b><span class="num">${ev.independent_correct} / ${ev.independent}</span></div>
          <div class="dots">${dots('ok', ev.independent_correct)}${dots('miss', miss(ev))}</div>
          <div class="v cnt">${T('计入', 'Counts')}</div></div>
        <div><div class="k"><b>${T('有辅助', 'Assisted')}</b><span class="num">${ev.assisted}</span></div>
          <div class="dots">${dots('as', ev.assisted)}</div>
          <div class="v no">${T('不计入', 'Never counts')}</div></div>
        <div><div class="k"><b>${T('重复接触', 'Re-exposure')}</b><span class="num">${ev.re_exposure}</span></div>
          <div class="dots">${dots('re', ev.re_exposure)}</div>
          <div class="v no">${T('不计入', 'Never counts')}</div></div>
      </div>
      <div class="diag"><span class="q" title="${ev.causal_diagnosis}">${
        ev.causal_diagnosis === 'unresolved' ? T('错因 · 未定', 'Cause · unresolved') : ev.causal_diagnosis
      }</span>${ev.causal_diagnosis === 'unresolved' ? T('证据还不足以判断错因。', 'The evidence is not enough to tell why.') : ''}</div>`
    : '';
  const promise = r.can_count
    ? html`<div class="promise"><span class="glow"></span><span><b>${T('新文章 · 本次可计入', 'Fresh passage · this can count')}</b>${T('作为一次独立迁移记录', 'Recorded as an independent transfer')}</span></div>`
    : html`<div class="promise"><span class="glow" style="background:var(--warm);box-shadow:0 0 0 4px var(--warm-soft)"></span><span><b style="color:var(--warm)">${T('本次不计入', 'This will not count')}</b>${T('材料已见过，记为重复接触', 'Material already seen — recorded as re-exposure')}</span></div>`;
  return html`<article class="obj hero card rise" style="--i:1" aria-labelledby="hero-t">
      <div class="obj-top">
        <span class="eyebrow">${T('下一步', 'Next')}</span><span class="pol" title="${r.action}">${actName(r.action)}<span class="id">${r.action}</span></span>
        <span class="r" title="${r.command}">${ic('local', 'xs')}${T('透明策略 · 本地计算', 'Transparent policy · local')}</span>
      </div>
      <h1 class="hero-title h-disp" id="hero-t">${r.skill_id ? skillName(r.skill_id) : T('练习', 'Practice')}</h1>
      ${r.skill_id ? html`<div class="hero-sub">${r.skill_kind ? skillKindName(r.skill_kind) : ''}<span class="sep"></span><span class="id">${r.skill_id}</span></div>` : ''}
      <p class="reason">${r.reason}</p>
      ${evid}
      <div class="obj-foot">
        ${promise}
        <button class="btn primary lg" data-act="start" data-v="recommended" id="start">${T('开始练习', 'Start practice')}<kbd>Enter</kbd></button>
      </div>
    </article>`;
}

export function vToday(t: Today): Html {
  if (t.active_run)
    return stateCard({
      ico: 'assess',
      eyebrow: T('测评进行中', 'Assessment in progress'),
      title: T(`第 ${t.active_run.run_id} 轮测评尚未结束`, `Run #${t.active_run.run_id} is not finished`),
      body: T('测评期间不能练习、看导师或改模型。计时一直在走。', 'Practice, coach and model changes wait until the run ends. Its clock keeps running.'),
      code: 'assessment_active',
      action: html`<a class="btn primary lg" href="#/assess">${T('回到测评', 'Back to the run')}${ic('arrow', 'xs')}</a>`,
    });
  const r = t.recommendation;
  const others = t.skills.filter((s) => s.skill_id !== r?.skill_id).slice(0, 2);
  const last = t.last_run;
  const pool = t.pool;
  const a = t.anki;
  const ankiFlag: Flag = a.conflict > 0 ? 'bad' : a.queued > 0 ? 'warm' : '';
  const briefFlag: Flag = t.brief?.state === 'paused' || t.brief?.state === 'needs_action' ? 'warm' : t.brief?.state === 'failed' ? 'bad' : '';
  return html`<div class="scene orbit">
    ${facts(t)}
    <div class="rail l">
      ${others.map((s, i) => skillSat(s, i + 1))}
      ${sat(
        'run',
        'report',
        last ? T(`上次测评 · ${monthDay(last.started_at)}`, `Last run · ${monthDay(last.started_at)}`) : T('上次测评', 'Last run'),
        last ? html`#${last.run_id} <span class="num" style="margin-left:6px">${last.correct ?? '—'} / ${last.size}</span>` : html`<small>${T('还没有测评', 'none yet')}</small>`,
        '',
        3,
      )}
    </div>
    ${hero(t)}
    <div class="rail r">
      ${sat(
        'pool',
        'lock',
        T('未见测评池', 'Unseen pool'),
        pool ? html`<span class="num">${pool.items}</span><small>${T(`题 · ${pool.units} 单元`, `items · ${pool.units} units`)}</small>` : html`<small>${T('没有已审核的来源', 'no reviewed source')}</small>`,
        pool && !pool.can_start ? 'warm' : '',
        4,
      )}
      ${sat(
        'anki',
        'anki',
        'Anki',
        html`<span class="num">${a.accepted}</span><small>${T(`已接受 · ${a.queued} 排队 · ${a.conflict} 冲突`, `accepted · ${a.queued} queued · ${a.conflict} conflict`)}</small>`,
        ankiFlag,
        5,
      )}
      ${sat('brief', t.brief?.state === 'running' ? 'pulse' : 'pause', T('学习简报', 'Study brief'), briefLine(t.brief), briefFlag, 6)}
    </div>
  </div>`;
}

// ---------------------------------------------------------------- sheets

function skillSheet(s: SkillEvidence, t: Today): Html {
  const recommended = t.recommendation?.skill_id === s.skill_id;
  return sheetWrap(
    'pulse',
    skillName(s.skill_id),
    html`<span class="id">${s.skill_id}</span><span class="sep"></span>${skillKindName(s.kind)}`,
    html`<div class="cols2">
      <div class="blk"><h3>${T('证据', 'Evidence')}</h3><div style="display:flex;gap:6px;margin:10px 0 8px">${evDots(s.independent, s.independent_correct, s.assisted, s.re_exposure)}</div>
        <p>${T(
          `独立迁移 ${s.independent} 次，答对 ${s.independent_correct} 次 · 有辅助 ${s.assisted} 次 · 重复接触 ${s.re_exposure} 次（后两者不计入）`,
          `${s.independent} independent transfers, ${s.independent_correct} correct · assisted ${s.assisted} · re-exposure ${s.re_exposure} (neither counts)`,
        )}</p></div>
      <div class="blk"><h3>${T('策略建议', 'Policy action')}</h3><p style="font-size:var(--t-sm);color:var(--ink)">${actName(s.action)} <span class="id">${s.action}</span></p>
        <p>${s.causal_diagnosis === 'unresolved' ? T('错因诊断：未定。证据不足时不猜原因。', 'Causal diagnosis: unresolved. No cause is guessed without evidence.') : s.causal_diagnosis}</p></div>
    </div>
    ${s.reason ? html`<p class="fine">${s.reason}</p>` : ''}
    ${recommended ? '' : html`<div class="sh-acts"><button class="btn ghost sm" data-act="start-skill" data-v="${s.skill_id}">${T('练习这个技能', 'Practice this skill')}${ic('arrow', 'xs')}</button></div>`}`,
  );
}

function briefSheet(b: BriefSummary | null): Html {
  if (!b)
    return sheetWrap('pause', T('学习简报', 'Study brief'), '', html`<p class="fine">${T('没有未完成的简报任务。', 'No unfinished brief run.')}</p>`);
  const stepBar = Array.from({ length: b.max_steps }, (_, i) => html`<i class="${i < b.step ? '' : 'off'}"></i>`);
  const canResume = b.state === 'paused' || b.state === 'needs_action';
  const acts = canResume
    ? html`<button class="btn primary sm" data-act="brief-resume" ${b.node_available ? '' : 'disabled'}>${ic('play', 'xs')}${T('继续', 'Resume')}</button><button class="btn quiet sm" data-act="brief-cancel">${T('取消任务', 'Cancel run')}</button>`
    : b.state === 'running'
      ? html`<button class="btn quiet sm" data-act="brief-cancel">${T('取消任务', 'Cancel run')}</button>`
      : '';
  return sheetWrap(
    b.state === 'running' ? 'pulse' : 'pause',
    T('学习简报', 'Study brief'),
    html`<span class="id">${b.run_id}</span><span class="sep"></span>${T('耐久任务 · 可恢复', 'durable run · resumable')}`,
    html`${kv(T('状态', 'State'), `${TP(BRIEF_STATE[b.state])} · ${T(`第 ${b.step} / ${b.max_steps} 步`, `step ${b.step} / ${b.max_steps}`)}`)}
      <div class="seg-bar" style="margin-bottom:14px">${stepBar}</div>
      <div class="ledger">
        <div><span class="lk">${T('执行', 'Runs on')}</span><span class="lv">${ic('local', 'xs')} ${T('本地 Agent 宿主（Node）', 'Local agent host (Node)')}${
          b.node_available ? '' : html` · <span class="held">${T('Node 不可用', 'Node unavailable')}</span>`
        }</span></div>
        <div><span class="lk">${T('调用', 'Calls')}</span><span class="lv">${
          b.provider ? html`<span class="ext">${ic('ext', 'xs')}${T('外部', 'External')}</span> · ${b.provider} · <code>${b.model ?? ''}</code>` : html`<span class="muted">${T('未记录', 'not recorded')}</span>`
        }</span></div>
        <div><span class="lk">${T('发送内容', 'Payload')}</span><span class="lv">${T('技能统计与战术说明（第一方）', 'Skill counts and tactic text (first-party)')}</span></div>
      </div>
      <div class="sh-acts">${acts}</div>`,
  );
}

export function todaySheet(key: string, t: Today): Html | null {
  if (key.startsWith('skill:')) {
    const s = t.skills.find((x) => x.skill_id === key.slice(6));
    return s ? skillSheet(s, t) : null;
  }
  switch (key) {
    case 'run': {
      const r = t.last_run;
      if (!r) return sheetWrap('report', T('上次测评', 'Last assessment'), '', html`<p class="fine">${T('还没有完成的测评。', 'No completed assessment yet.')}</p>`);
      return sheetWrap(
        'report',
        T(`上次测评 · 第 ${r.run_id} 轮`, `Last assessment · run #${r.run_id}`),
        shortDate(r.started_at),
        html`${kv(T('答对', 'Correct'), `${r.correct ?? '—'} / ${r.size}`, true)}
        ${kv(T('规模', 'Size'), T(`${r.size} 题 · ${r.minutes} 分钟`, `${r.size} items · ${r.minutes} min`))}
        ${kv(T('及格线', 'Pass threshold'), T('未知 · 不作预测', 'unknown · no prediction'))}
        <div class="sh-acts"><a class="btn ghost sm" href="#/report">${T('查看报告', 'See the report')}${ic('arrow', 'xs')}</a></div>`,
      );
    }
    case 'pool': {
      const p = t.pool;
      if (!p)
        return sheetWrap(
          'lock',
          T('未见测评池', 'Unseen assessment pool'),
          T('只有没见过的题才能用于测评', 'Only unseen items can be used to assess'),
          html`<p class="fine">${T('还没有已审核的导入来源，因此没有测评池。', 'No reviewed imported source exists, so there is no pool yet.')}</p>`,
        );
      const total = p.units + p.spent_units;
      return sheetWrap(
        'lock',
        T('未见测评池', 'Unseen assessment pool'),
        T('只有没见过的题才能用于测评', 'Only unseen items can be used to assess'),
        html`<div class="kv"><span>${T('剩余', 'Remaining')}</span><b class="big num">${p.items} <small style="font-size:14px;color:var(--ink-3)">${T(`题 / ${p.units} 单元`, `items / ${p.units} units`)}</small></b></div>
        <div class="seg-bar" aria-hidden="true">${Array.from({ length: total }, (_, i) => html`<i class="${i >= p.units ? 'off' : ''}"></i>`)}</div>
        <div class="kv" style="margin-top:8px"><span>${T('已用', 'Spent')}</span><b class="num">${T(`${p.spent_units} 单元`, `${p.spent_units} units`)}</b></div>
        ${kv(T('下限', 'Floor'), T(`${p.floor_items} 题`, `${p.floor_items} items`))}
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
        html`${kv(T('已接受', 'Accepted'), a.accepted)}
        ${kv(T('已推送', 'Pushed'), a.pushed)}
        <div class="kv"><span>${T('排队 · 可连接时推送一次', 'Queued · pushes once when reachable')}</span><b class="num" style="color:var(--warm)">${a.queued}</b></div>
        <div class="kv"><span>${T('冲突 · 你在 Anki 里改过，不会被静默覆盖', 'Conflict · edited in Anki, never overwritten silently')}</span><b class="num" style="color:var(--bad)">${a.conflict}</b></div>
        <div class="sh-acts"><a class="btn ghost sm" href="#/anki">${T('打开 Anki 笔记', 'Open Anki notes')}${ic('arrow', 'xs')}</a></div>`,
      );
    }
    case 'brief':
      return briefSheet(t.brief);
  }
  return null;
}

/** Lines for the coach popover on Today: local policy text, not model text. */
export function todayPopLines(t: Today): string[] {
  const lines: string[] = [];
  if (t.recommendation) lines.push(t.recommendation.reason);
  lines.push(T('作答时我不说话；需要时按 H 看提示，但那一题就不计入。', 'I stay quiet while you answer; H opens a hint, but then that item will not count.'));
  if (t.providers_unusable) lines.push(t.providers_unusable.message);
  return lines;
}
