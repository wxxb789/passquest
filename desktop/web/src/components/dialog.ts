// Centered dialogs (prototype .dlg): the shortcut sheet, the hint gate, leave-assessment, and the
// small forms the port adds (learner settings, credential entry, start a run). The safer choice
// carries `autofocus`.
import type { AssessmentPool, LearnerSettings } from '../api/types';
import { html, type Html, type Interp } from '../core/html';
import { T } from '../core/i18n';
import { ic, type IconName } from '../icons';

function dlg(tone: 'warm' | 'neutral' | 'adv', ico: IconName, id: string, title: string, body: Interp, acts: Interp): Html {
  return html`<div class="scrim" data-act="close"></div><div class="dlg ${tone}" role="dialog" aria-modal="true" aria-labelledby="${id}">
      <span class="ico">${ic(ico, 'l')}</span>
      <h2 class="h-disp" id="${id}">${title}</h2>
      ${body}
      <div class="acts">${acts}</div></div>`;
}

export function keysDialog(): Html {
  const g = (h: string, rows: [string, string[]][]) =>
    html`<div><h3>${h}</h3>${rows.map(([a, k]) => html`<div class="row"><span>${a}</span><span>${k.map((x) => html`<kbd>${x}</kbd>`)}</span></div>`)}</div>`;
  return html`<div class="scrim" data-act="close"></div><div class="dlg keys" role="dialog" aria-modal="true" aria-labelledby="dk">
      <div class="hd"><span class="ico" style="width:40px;height:40px;border-radius:50%;display:grid;place-items:center;background:var(--well);box-shadow:inset 0 0 0 1px var(--line-2)">${ic('help')}</span><h2 class="h-disp" id="dk" style="margin:0">${T('快捷键', 'Keyboard shortcuts')}</h2><button class="icon-btn" style="margin-left:auto" data-act="close" aria-label="${T('关闭', 'Close')}">${ic('x')}</button></div>
      <div class="grid">
        ${g(T('全局', 'Anywhere'), [
          [T('快捷键面板', 'This sheet'), ['?']],
          [T('切换主题', 'Theme'), ['Shift', 'T']],
          [T('切换语言', 'Language'), ['Shift', 'L']],
          [T('今日', 'Today'), ['G', 'T']],
          [T('练习', 'Practice'), ['G', 'P']],
          [T('测评', 'Assess'), ['G', 'A']],
          [T('报告', 'Report'), ['G', 'R']],
          [T('Anki 笔记', 'Anki'), ['G', 'K']],
          [T('设置', 'Settings'), ['G', 'S']],
          [T('关闭', 'Close'), ['Esc']],
        ])}
        ${g(T('练习', 'Practice'), [
          [T('选择选项', 'Choose'), ['A', '–', 'D']],
          [T('或用数字', 'or digits'), ['1', '–', '4']],
          [T('提交', 'Submit'), ['Enter']],
          [T('提示（转为有辅助）', 'Hint (assisted)'), ['H']],
          [T('跳过', 'Skip'), ['S']],
          [T('标记选中文字', 'Mark selection'), ['M']],
          [T('今日：开始练习', 'Today: start'), ['Enter']],
        ])}
        ${g(T('反馈 · 测评 · 笔记', 'Feedback · Assess · Anki'), [
          [T('导师模式', 'Coach modes'), ['1', '2', '3']],
          [T('看选项依据', 'Option evidence'), ['↑', '↓']],
          [T('生成笔记', 'Propose note'), ['K']],
          [T('下一题', 'Next item'), ['N']],
          [T('测评：上 / 下一题', 'Assess: prev / next'), ['←', '→']],
          [T('测评：确认', 'Assess: confirm'), ['Enter']],
          [T('笔记：翻面', 'Anki: flip'), ['F']],
          [T('笔记：编辑字段', 'Anki: edit fields'), ['E']],
        ])}
      </div></div>`;
}

/** The hint gate: opening a hint turns the attempt assisted, and that cannot be undone. */
export function hintGate(canCountNow: boolean): Html {
  return dlg(
    'warm',
    'eye',
    'dh',
    T('打开提示？', 'Open the hint?'),
    html`<p>${T('提示可见后，本次作答记为「有辅助」，', 'Once the hint is visible this attempt is recorded as ')}<b>${T('不计入', 'assisted')}</b>${T(
      '独立迁移。这一步不能撤销。',
      ' and will not count as an independent transfer. This cannot be undone.',
    )}</p>
      <div class="cmpare"><div><b>${T('现在', 'Now')}</b>${canCountNow ? T('独立 · 可计入', 'Independent · can count') : T('不计入', 'Does not count')}</div><span class="muted">${ic('arrow', 's')}</span><div class="on"><b>${T('打开后', 'After')}</b>${T('有辅助 · 不计入', 'Assisted · never counts')}</div></div>`,
    html`<button class="btn quiet" data-act="hint-yes">${T('打开提示', 'Open hint')}</button><button class="btn primary" data-act="close" autofocus>${T('继续独立作答', 'Stay independent')}<kbd>Esc</kbd></button>`,
  );
}

export function leaveDialog(position: number | null): Html {
  return dlg(
    'neutral',
    'door',
    'dl',
    T('离开测评？', 'Leave the assessment?'),
    html`<p>${position !== null ? T(`离开后可从第 ${position} 题恢复，`, `You can resume at item ${position} later. `) : T('离开后可恢复，', 'You can resume later. ')}<b>${T(
      '计时不暂停',
      'The clock keeps running',
    )}</b>${T('：剩余时间会继续减少。', ' while you are away.')}</p>`,
    html`<button class="btn quiet" data-act="leave-yes">${T('离开', 'Leave')}</button><button class="btn primary" data-act="close" autofocus>${T('继续测评', 'Keep going')}</button>`,
  );
}

/** Optional, display-only learner settings (Q-0011-06). */
export function learnerDialog(s: LearnerSettings): Html {
  return dlg(
    'neutral',
    'clock',
    'dlr',
    T('考试日期与每日时长', 'Exam date and daily time'),
    html`<p>${T('只用于显示，不影响选题、评分或测评。', 'Display only — never used for selection, scoring or assessment.')}</p>
      <form class="pq-form" id="learnerform" data-form="learner">
        <label class="field"><span class="fl">${T('考试日期', 'Exam date')}</span><input type="date" name="exam_date" value="${s.exam_date ?? ''}"></label>
        <label class="field"><span class="fl">${T('每日学习时长（分钟）', 'Daily study time (minutes)')}</span><input type="number" name="daily_minutes" min="1" max="1440" step="1" value="${s.daily_minutes ?? ''}"></label>
      </form>`,
    html`<button class="btn quiet" data-act="close" autofocus>${T('取消', 'Cancel')}</button><button class="btn primary" data-act="learner-save">${T('保存', 'Save')}</button>`,
  );
}

/** Credential entry. The value goes to the OS credential store and is cleared from the DOM at once. */
export function credentialDialog(providerName: string, storeAvailable: boolean, reason: string | null): Html {
  return dlg(
    'neutral',
    'key',
    'dcr',
    T(`${providerName} 的密钥`, `${providerName} key`),
    storeAvailable
      ? html`<p>${T('密钥写入系统密钥库。之后这里只显示有 / 无，从不显示密钥本身。', 'The key goes to the OS credential store. Afterwards it is shown only as present / missing, never displayed.')}</p>
      <form class="pq-form" id="credform" data-form="credential" autocomplete="off">
        <label class="field"><span class="fl">${T('API 密钥', 'API key')}</span><input type="password" name="key" autocomplete="off" spellcheck="false"></label>
      </form>`
      : html`<p>${T('本机没有可用的系统密钥库：', 'No OS credential store is available here: ')}${reason ?? ''}${T('。请改用环境变量。', '. Use the environment variable instead.')}</p>`,
    storeAvailable
      ? html`<button class="btn quiet" data-act="close" autofocus>${T('取消', 'Cancel')}</button><button class="btn primary" data-act="credential-save">${T('保存到密钥库', 'Save to the store')}</button>`
      : html`<button class="btn primary" data-act="close" autofocus>${T('好', 'OK')}</button>`,
  );
}

export function startRunDialog(pool: AssessmentPool): Html {
  return dlg(
    'neutral',
    'assess',
    'dsr',
    T('开始限时测评？', 'Start a timed assessment?'),
    html`<p>${T(
      `${pool.default_size} 题 / ${pool.default_minutes} 分钟，限时、无辅助，作答后不显示对错。离开后计时不暂停。展示过的题会从未见测评池中用掉。`,
      `${pool.default_size} items / ${pool.default_minutes} min, timed and unassisted, no feedback after answering. The clock keeps running if you leave. Presented items are spent from the unseen pool.`,
    )}</p>`,
    html`<button class="btn quiet" data-act="close" autofocus>${T('取消', 'Cancel')}</button><button class="btn primary" data-act="run-start">${T('开始', 'Start')}</button>`,
  );
}
