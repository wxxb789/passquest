// Feedback (prototype vFeedback): the graded card overlapping the receded passage. The deterministic
// grade and the verified key lead; every option shows its authored explanation or the explicit
// "not provided" state; hovering an option lights its evidence in the passage.
import type { Feedback, OptionFeedback, TextSpan } from '../api/types';
import { html, mmss, type Html } from '../core/html';
import { skillName, T } from '../core/i18n';
import { conditionChip } from '../components/condition';
import { feedbackFoot, passageHTML } from '../components/passage';
import { ic } from '../icons';

/** The span an option lights: its own evidence, or the verified evidence for the key. */
export function optionSpan(fb: Feedback, label: string | null): TextSpan | null {
  if (!label) return null;
  const o = fb.options.find((x) => x.label === label);
  if (!o) return null;
  return o.evidence ?? (o.label === fb.key.label ? fb.evidence.span : null);
}

function optionRow(fb: Feedback, o: OptionFeedback, ev: string): Html {
  const isKey = o.label === fb.key.label;
  const chosen = o.label === fb.selected_label;
  const mine = chosen && !isKey;
  const tag = isKey
    ? html`<span class="chip ok sm">${ic('seal', 'xs')}${chosen ? T('你的选择 · 答案', 'Yours · key') : T('答案 · 已核验', 'Key · verified')}</span>`
    : mine
      ? html`<span class="chip bad sm">${T('你的选择', 'Your choice')}</span>`
      : '';
  const ex =
    o.explanation_missing || o.explanation === null
      ? html`<span class="ex muted" data-missing="true">${T('原书未提供该选项的解析。', 'The source provides no explanation for this option.')}</span>`
      : html`<span class="ex">${o.explanation}</span>`;
  return html`<button class="fo ${isKey ? 'is-key' : ''} ${mine ? 'is-mine' : ''} ${ev === o.label ? 'on' : ''}" data-ev="${o.label}">
            <span class="key" aria-hidden="true">${o.label}</span>
            <span><span class="ot exam" lang="en"><span class="sr">${o.label}. </span>${o.text}</span>${ex}</span>
            <span class="tag">${tag}</span>
          </button>`;
}

export function vFeedback(fb: Feedback, ev: string | null): Html {
  const lit = ev ?? fb.key.label;
  const right = fb.verdict === 'correct';
  const verdict = fb.verdict === 'skipped' ? T('已跳过', 'Skipped') : right ? T('正确', 'Correct') : T('不正确', 'Incorrect');
  const cuts = fb.options.map((o) => o.evidence).filter((s): s is TextSpan => s !== null);
  return html`<div class="scene fb">
    ${passageHTML(fb.passage, { mode: 'feedback', cuts, lit: optionSpan(fb, lit), litKey: lit === fb.key.label, foot: feedbackFoot() })}
    <article class="card fcard" aria-label="${T('评分结果', 'Graded result')}" data-attempt="${fb.attempt_id}">
      <div class="f-scroll">
        <div class="verdict ${right ? 'yes' : 'no'}">
          <span class="mk">${ic(right ? 'check' : 'x', 'l')}</span>
          <div><div class="w">${verdict}</div>
          <div class="m">${fb.selected_label ? html`${T('你选了', 'You chose')} <b>${fb.selected_label}</b><span class="sep"></span>` : ''}${T('已核验答案', 'Verified key')} <b id="verified-key">${fb.key.label}</b></div></div>
          <div class="q-r">${T(`第 ${fb.question.number} 题`, `Question ${fb.question.number}`)}<br>${fb.question.skill_ids.map(skillName).join(' · ')}</div>
        </div>
        <div class="f-chips">${conditionChip(fb.condition, fb.counts)}<span class="chip sm">${ic('clock', 'xs')}<span class="num">${mmss(fb.elapsed_ms / 1000)}</span></span><span class="chip ok sm">${ic('seal', 'xs')}${T('确定性评分', 'Deterministic grade')}</span></div>
        <p class="f-stem exam" lang="en">${fb.question.stem}</p>
        <div class="fopts">${fb.options.map((o) => optionRow(fb, o, lit))}</div>
        ${
          fb.evidence.span === null
            ? html`<div class="tactic" id="evidence"><span class="ic">${ic('seal', 's')}</span><div class="k">${T('证据', 'Evidence')}</div><p class="exam" lang="en">“${fb.evidence.text}”</p></div>`
            : ''
        }
        ${
          fb.tactic
            ? html`<div class="tactic"><span class="ic">${ic('book', 's')}</span>
          <div class="k">${T('战术', 'Tactic')} · ${fb.tactic.name}</div>
          <p>${fb.tactic.summary}</p>
        </div>`
            : ''
        }
      </div>
      <div class="f-acts">
        <button class="btn ghost" data-act="propose-note">${ic('anki', 's')}${T('生成 Anki 笔记', 'Propose an Anki note')}<kbd>K</kbd></button>
        <span class="sp"></span>
        <button class="btn primary" data-act="next">${T('下一题', 'Next')}<kbd>N</kbd></button>
      </div>
    </article>
  </div>`;
}
