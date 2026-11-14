// Practice, before answering (prototype vPractice + qcolHTML): the passage paper and the question
// column with the condition capsule, the learner's own clock and the gated hint.
import type { PracticeItem } from '../api/types';
import { html, type Html } from '../core/html';
import { skillName, T } from '../core/i18n';
import type { Mark } from '../core/state';
import { conditionCapsule } from '../components/condition';
import { optsHTML } from '../components/options';
import { passageHTML, practiceFoot } from '../components/passage';
import { ic } from '../icons';
import { stateCard } from './states';

export interface PracticeUi {
  sel: string | null;
  marks: readonly Mark[];
  busy: boolean;
}

function hintFoot(item: PracticeItem): Html {
  if (item.hint) return html`<span class="chip warm sm">${ic('eye', 'xs')}${T('提示已打开', 'Hint open')}</span>`;
  if (item.hint_available)
    return html`<button class="hintbtn" data-act="hint" aria-describedby="hintnote">${ic('eye', 's')}${T('提示', 'Hint')}<kbd>H</kbd></button>`;
  return html`<button class="hintbtn" disabled title="${T('这道题没有已编写的提示', 'No authored hint for this item')}">${ic('eye', 's')}${T('本题无提示', 'No hint for this item')}</button>`;
}

export function qcolHTML(item: PracticeItem, u: PracticeUi): Html {
  const q = item.question;
  const hinted = item.hint !== null;
  const hint = item.hint
    ? html`<div class="hintbox"><div class="k">${ic('eye', 's')}${T('提示 · 已核验战术', 'Hint · verified tactic')} · ${item.hint.title}</div>${item.hint.steps.map(
        (s, i) => html`${i ? html`<br>` : ''}${s}`,
      )}</div>`
    : '';
  return html`<div class="qsat settle" style="--i:1">
      ${conditionCapsule(item.condition, item.can_count, hinted)}
      <div class="clock glass" role="timer" aria-label="${T('你的用时', 'Your time')}"><span class="t num" id="itemclock" data-clock="since" data-t="${item.presented_at}">0:00</span><span class="l">${T('你的用时 · 不限时', 'Your time · no limit')}</span></div>
    </div>
    <article class="qcard card rise${hinted ? ' hinted' : ''}" style="--i:2" aria-label="${T('题目', 'Question')}">
      <div class="q-head"><span class="eyebrow">${T(`第 ${q.number} 题 · 共 ${q.count} 题`, `Question ${q.number} of ${q.count}`)}</span><span class="r">${q.skill_ids.map(skillName).join(' · ')}</span></div>
      <p class="q-stem exam" lang="en">${q.stem}</p>
      ${hint}
      ${optsHTML(q.options, u.sel, 'pick')}
      <div class="q-foot">
        ${hintFoot(item)}
        <span class="sp"></span>
        <button class="btn quiet" data-act="skip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}<kbd>S</kbd></button>
        <button class="btn primary" data-act="submit" id="submit" ${u.sel && !u.busy ? '' : 'disabled'}>${T('提交', 'Submit')}<kbd>Enter</kbd></button>
      </div>
      ${
        hinted || !item.hint_available
          ? ''
          : html`<div class="q-note" id="hintnote">${ic('arrow', 'xs')}${T('打开提示后，本题记为有辅助，不计入', 'Opening the hint makes this attempt assisted — it will not count')}</div>`
      }
    </article>
    <div class="keyhint settle" style="--i:3;justify-content:center"><kbd>A</kbd>–<kbd>D</kbd> ${T('或', 'or')} <kbd>1</kbd>–<kbd>4</kbd> ${T('选择', 'choose')}<span class="sep"></span><kbd>?</kbd> ${T('全部快捷键', 'all shortcuts')}</div>`;
}

export function vPractice(item: PracticeItem | null, u: PracticeUi): Html {
  if (!item)
    return stateCard({
      ico: 'practice',
      eyebrow: T('练习', 'Practice'),
      title: T('没有打开的题目', 'No item is open'),
      body: T('从今日的下一步开始，或练习内置示例。展示一道题会记录展示时间，作答用时从那一刻算起。', "Start from today's next step, or practise the built-in sample. Showing an item records its presentation time; your time counts from then."),
      action: html`<button class="btn ghost" data-act="start" data-v="sample">${T('内置示例', 'Built-in sample')}</button><button class="btn primary lg" data-act="start" data-v="next" id="start">${T('下一步', 'Next step')}<kbd>Enter</kbd></button>`,
    });
  return html`<div class="scene read">${passageHTML(item.passage, { mode: 'practice', marks: u.marks, foot: practiceFoot(u.marks.length) })}<div class="qcol" id="qcol">${qcolHTML(item, u)}</div></div>`;
}
