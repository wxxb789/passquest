// Feedback (prototype vFeedback): the graded card overlapping the receded passage. The deterministic
// grade and the verified key lead; every option shows its authored explanation or the explicit
// "not provided" state; the evidence sentence the core located is lit in the passage.
import type { Feedback, OptionExplanation } from '../api/types';
import { html, mmss, type Html } from '../core/html';
import { T } from '../core/i18n';
import { conditionChip } from '../components/condition';
import { feedbackFoot, passageHTML, type TextSpan } from '../components/passage';
import { ic } from '../icons';

/** The located evidence span (rune offsets in one paragraph), or null when the core could not place it. */
export function evidenceSpan(fb: Feedback): TextSpan | null {
  return fb.evidence.located ? fb.evidence : null;
}

/** The span an option lights: only the verified key has located evidence. */
export function optionSpan(fb: Feedback, label: string | null): TextSpan | null {
  return label !== null && label === fb.key.label ? evidenceSpan(fb) : null;
}

function optionRow(o: OptionExplanation, ev: string): Html {
  const mine = o.selected && !o.correct;
  const tag = o.correct
    ? html`<span class="chip ok sm">${ic('seal', 'xs')}${o.selected ? T('你的选择 · 答案', 'Yours · key') : T('答案 · 已核验', 'Key · verified')}</span>`
    : mine
      ? html`<span class="chip bad sm">${T('你的选择', 'Your choice')}</span>`
      : '';
  const ex = o.missing
    ? html`<span class="ex muted" data-missing="true">${T('原书未提供该选项的解析。', 'The source provides no explanation for this option.')}</span>`
    : html`<span class="ex" lang="en" data-missing="false">${o.explanation}</span>`;
  return html`<button class="fo ${o.correct ? 'is-key' : ''} ${mine ? 'is-mine' : ''} ${ev === o.label ? 'on' : ''}" data-ev="${o.label}" data-testid="option-explanation">
            <span class="key" aria-hidden="true">${o.label}</span>
            <span><span class="ot exam" lang="en"><span class="sr">${o.label}. </span>${o.text}</span>${ex}</span>
            <span class="tag">${tag}</span>
          </button>`;
}

function verdictText(v: string): string {
  switch (v) {
    case 'correct':
      return T('正确', 'Correct');
    case 'incorrect':
      return T('不正确', 'Incorrect');
    case 'skipped':
      return T('已跳过', 'Skipped');
    default:
      return T('评分未知', 'Grade unknown');
  }
}

export function vFeedback(fb: Feedback, ev: string | null): Html {
  const lit = ev ?? fb.key.label;
  const right = fb.verdict === 'correct';
  const span = evidenceSpan(fb);
  const evidence = span
    ? ''
    : html`<div class="tactic" id="evidence" data-testid="evidence" data-located="false"><span class="ic">${ic('seal', 's')}</span><div class="k">${T('证据（未能在文章中定位）', 'Evidence (not located in the passage)')}</div><p class="exam" lang="en">“${fb.evidence.text}”</p></div>`;
  return html`<div class="scene fb" data-testid="feedback-scene">
    ${passageHTML(fb.passage, { mode: 'feedback', cuts: span ? [span] : [], lit: optionSpan(fb, lit), litKey: lit === fb.key.label, foot: feedbackFoot() })}
    <article class="card fcard" aria-label="${T('评分结果', 'Graded result')}" data-testid="feedback" data-attempt="${fb.attempt_id}" data-verdict="${fb.verdict}" data-elapsed-ms="${fb.elapsed_ms}">
      <div class="f-scroll">
        <div class="verdict ${right ? 'yes' : 'no'}">
          <span class="mk">${ic(right ? 'check' : 'x', 'l')}</span>
          <div><div class="w">${verdictText(fb.verdict)}</div>
          <div class="m">${fb.selected ? html`${T('你选了', 'You chose')} <b>${fb.selected.label}</b><span class="sep"></span>` : ''}${T('已核验答案', 'Verified key')} <b id="verified-key" data-testid="verified-key">${fb.key.label}</b></div></div>
          <div class="q-r">${T(`第 ${fb.attempt_number} 次作答`, `Attempt ${fb.attempt_number}`)}<br><span class="id">${fb.question_id}</span></div>
        </div>
        <div class="f-chips">${conditionChip(fb.condition, fb.counted)}<span class="chip sm" data-testid="elapsed">${ic('clock', 'xs')}<span class="num">${mmss(fb.elapsed_ms / 1000)}</span></span><span class="chip ok sm">${ic('seal', 'xs')}${T('确定性评分', 'Deterministic grade')}</span>${
          fb.already_answered ? html`<span class="chip sm">${T('已记录过 · 未重复写入', 'Already recorded · not written again')}</span>` : ''
        }</div>
        <p class="f-stem exam" lang="en">${fb.stem}</p>
        <div class="fopts">${fb.options.map((o) => optionRow(o, lit))}</div>
        ${evidence}
        ${fb.reasoning ? html`<div class="tactic"><span class="ic">${ic('seal', 's')}</span><div class="k">${T('已审核解析', 'Reviewed reasoning')}</div><p lang="en">${fb.reasoning}</p></div>` : ''}
        ${
          fb.tactic
            ? html`<div class="tactic" data-testid="tactic-link" data-tactic="${fb.tactic.id}"><span class="ic">${ic('book', 's')}</span>
          <div class="k">${T('战术', 'Tactic')} · <span lang="en">${fb.tactic.name}</span></div>
          <p lang="en">${fb.tactic.summary}</p>
          <button class="btn quiet sm" data-act="start-tactic" data-v="${fb.tactic.id}|guided">${T('练习这个战术', 'Practise this tactic')}${ic('arrow', 'xs')}</button>
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

