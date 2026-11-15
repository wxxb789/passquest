// Passage paper (prototype passageHTML + markedText + setEv + selection marks).
//
// The core stores passages as plain-text paragraphs and locates evidence as code-point spans. The
// window splits each paragraph into sentence spans (`.s`, Intl.Segmenter) and additionally cuts at
// every evidence boundary, so an evidence range is always whole `.s` spans the CSS can light. Each
// `.s` carries its UTF-16 range in the paragraph (`data-a`/`data-b`) so a highlight or a mark can be
// mapped back to paragraph offsets.
import type { Passage, SourceLabel } from '../api/types';
import { html, type Html } from '../core/html';
import { sourceKindName, T } from '../core/i18n';
import type { Mark } from '../core/state';
import { toast } from '../core/toast';
import { ic } from '../icons';

export type PassageMode = 'practice' | 'feedback' | 'assess';

/** A half-open range of code points inside one paragraph (the core's `EvidenceSpan` offsets). */
export interface TextSpan {
  paragraph: number;
  start: number;
  end: number;
}

export interface PassageOptions {
  mode: PassageMode;
  /** Spans to cut sentences at (feedback: every option's evidence). */
  cuts?: readonly TextSpan[];
  /** The span lit now, and whether it is the verified key's evidence. */
  lit?: TextSpan | null;
  litKey?: boolean;
  marks?: readonly Mark[];
  foot: Html;
  /** A small disclosure beside the source label (e.g. the built-in sample is not exam material). */
  note?: Html | '';
}

/** Code-point offset → UTF-16 offset in `text`. */
export function cpToU16(text: string, cp: number): number {
  let u = 0;
  let i = 0;
  for (const ch of text) {
    if (i >= cp) break;
    u += ch.length;
    i++;
  }
  return u;
}

const segmenter = typeof Intl.Segmenter === 'function' ? new Intl.Segmenter('en', { granularity: 'sentence' }) : null;

function sentenceStarts(text: string): number[] {
  if (!segmenter) return [0];
  return Array.from(segmenter.segment(text), (s) => s.index);
}

function u16Span(paragraphs: readonly string[], span: TextSpan): { p: number; a: number; b: number } | null {
  const text = paragraphs[span.paragraph];
  if (text === undefined || span.end <= span.start) return null;
  return { p: span.paragraph, a: cpToU16(text, span.start), b: cpToU16(text, span.end) };
}

function markedText(text: string, offset: number, marks: readonly Mark[]): Html[] {
  // text = paragraph slice starting at `offset`; marks are paragraph offsets (prototype markedText)
  const out: Html[] = [];
  let at = 0;
  [...marks]
    .sort((x, y) => x.a - y.a)
    .forEach((m) => {
      const a = Math.max(0, m.a - offset);
      const b = Math.min(text.length, m.b - offset);
      if (b <= a || a < at) return;
      out.push(html`${text.slice(at, a)}<mark class="lm">${text.slice(a, b)}</mark>`);
      at = b;
    });
  out.push(html`${text.slice(at)}`);
  return out;
}

function paragraphHTML(text: string, pi: number, o: PassageOptions, cutsU16: { p: number; a: number; b: number }[], lit: { p: number; a: number; b: number } | null): Html {
  const points = new Set<number>([0, text.length, ...sentenceStarts(text)]);
  for (const c of cutsU16) if (c.p === pi) points.add(c.a).add(c.b);
  const sorted = [...points].filter((n) => n >= 0 && n <= text.length).sort((x, y) => x - y);
  const marks = (o.marks ?? []).filter((m) => m.p === pi);
  const spans: Html[] = [];
  for (let i = 0; i + 1 < sorted.length; i++) {
    const a = sorted[i]!;
    const b = sorted[i + 1]!;
    if (b <= a) continue;
    const isLit = !!lit && lit.p === pi && a >= lit.a && b <= lit.b;
    const cls = 's' + (isLit ? ' ev' + (o.litKey ? ' key' : '') : '');
    spans.push(html`<span class="${cls}" data-s="${pi}-${a}" data-a="${a}" data-b="${b}">${markedText(text.slice(a, b), a, marks)}</span>`);
  }
  return html`<p data-n="${pi + 1}">${spans}</p>`;
}

export function sourceLabel(s: SourceLabel): string {
  const parts: string[] = [sourceKindName(s.kind)];
  if (s.source_id) parts.push(s.source_id);
  if (s.page) parts.push(T(`第 ${s.page} 页`, `p.${s.page}`));
  parts.push(s.reviewed ? T('已审核', 'reviewed') : T('未审核', 'not reviewed'));
  return parts.join(' · ');
}

export function passageHTML(p: Passage, o: PassageOptions): Html {
  const cutsU16 = [...(o.cuts ?? []), ...(o.lit ? [o.lit] : [])]
    .map((s) => u16Span(p.paragraphs, s))
    .filter((s): s is NonNullable<typeof s> => s !== null);
  const lit = o.lit ? u16Span(p.paragraphs, o.lit) : null;
  const body = p.paragraphs.map((text, pi) => paragraphHTML(text, pi, o, cutsU16, lit));
  return html`<section class="paper pas ${o.mode === 'feedback' ? 'receded' : ''} ${o.mode === 'practice' ? 'rise' : ''}" style="--i:0" aria-label="${T('文章', 'Passage')}">
    <div class="pas-head">
      <div class="pas-meta">
        <span class="src" data-testid="source">${ic('seal', 's')}${sourceLabel(p.label)}</span>
        ${p.label.reviewed ? '' : html`<span class="ph">${ic('info', 'xs')}${T('来源未审核', 'Source not reviewed')}</span>`}
        ${o.note ?? ''}
      </div>
      <h2 class="pas-title exam" lang="en">${p.title}</h2>
    </div>
    <div class="pas-body exam" lang="en" id="passage" data-mode="${o.mode}">${body}</div>
    <div class="pas-foot">${o.foot}</div>
  </section>`;
}

export function practiceFoot(marks: number): Html {
  return html`${ic('mark', 's')}<span id="markcount">${markCountText(marks)}</span><span style="margin-left:auto" class="acts-row">${
    marks ? html`<button class="btn quiet sm" data-act="mark-note" data-testid="mark-note">${ic('anki', 'xs')}${T('用标记生成笔记', 'Note from the mark')}</button>` : ''
  }<span class="keyhint"><kbd>M</kbd> ${T('标记', 'mark')}</span></span>`;
}

export function markCountText(n: number): string {
  return n
    ? T(`已标记 ${n} 处 · 稍后可生成 Anki 笔记`, `${n} marked · available for an Anki note later`)
    : T('选中文字即可标记，留作 Anki 笔记', 'Select text to mark it for a later Anki note');
}

export function feedbackFoot(): Html {
  return html`${ic('eye', 's')}<span>${T('高亮的是已核验答案的依据句', 'The lit sentence is the evidence for the verified key')}</span>`;
}

export function assessFoot(position: number, count: number): Html {
  return html`${ic('book', 's')}<span>${T(`第 ${position + 1} / ${count} 题 · 不显示提示、词表和对错`, `Item ${position + 1} of ${count} · no hints, glossary or feedback`)}</span>`;
}

// ---------------------------------------------------------------- live DOM behaviour

/** Lights the `.s` spans inside `span` (feedback hover), scrolling it to 35% of the box. */
export function lightSpan(paragraphs: readonly string[], span: TextSpan | null, isKey: boolean, smooth: boolean): void {
  document.querySelectorAll('.pas-body .s').forEach((s) => s.classList.remove('ev', 'key'));
  const r = span ? u16Span(paragraphs, span) : null;
  if (!r) return;
  let first: HTMLElement | null = null;
  document.querySelectorAll<HTMLElement>(`.pas-body p[data-n="${r.p + 1}"] .s`).forEach((s) => {
    const a = Number(s.dataset.a);
    const b = Number(s.dataset.b);
    if (a >= r.a && b <= r.b) {
      s.classList.add('ev');
      if (isKey) s.classList.add('key');
      first ??= s;
    }
  });
  const box = document.getElementById('passage');
  const el = first as HTMLElement | null;
  if (el && box) box.scrollTo({ top: Math.max(0, el.offsetTop - box.clientHeight * 0.35), behavior: smooth ? 'smooth' : 'auto' });
}

/** Feedback entry: put the lit evidence at 35% of the passage box (prototype centerEvidence). */
export function centerEvidence(): void {
  const el = document.querySelector<HTMLElement>('.pas-body .s.ev');
  const box = document.getElementById('passage');
  if (el && box) box.scrollTop = Math.max(0, el.offsetTop - box.clientHeight * 0.35);
}

// selection → mark (prototype lines 2087–2118)
let pendingRange: Range | null = null;

export function hideMarkPop(): void {
  document.querySelector('.markpop')?.remove();
  pendingRange = null;
}

export function hasPendingMark(): boolean {
  return pendingRange !== null;
}

export function checkSelection(): void {
  const sel = window.getSelection();
  const box = document.getElementById('passage');
  if (!box || box.dataset.mode !== 'practice' || !sel || sel.rangeCount === 0 || sel.isCollapsed || !box.contains(sel.anchorNode) || !box.contains(sel.focusNode)) {
    hideMarkPop();
    return;
  }
  pendingRange = sel.getRangeAt(0).cloneRange();
  const r = pendingRange.getBoundingClientRect();
  document.querySelector('.markpop')?.remove();
  const p = document.createElement('div');
  p.className = 'markpop';
  p.innerHTML = html`<span>${T('标记为笔记素材', 'Mark for a note')}</span><button data-mark>${ic('mark', 'xs')}${T('标记', 'Mark')}<kbd>M</kbd></button>`.value;
  p.style.left = Math.max(12, r.left + r.width / 2 - 110) + 'px';
  p.style.top = Math.max(12, r.top - 50) + 'px';
  document.body.appendChild(p);
  p.addEventListener('mousedown', (ev) => ev.preventDefault());
  p.querySelector('[data-mark]')?.addEventListener('click', () => markHandler());
}

let markHandler: () => void = () => {};
export function onMark(fn: () => void): void {
  markHandler = fn;
}

/** Turns the pending selection into a mark, or null (and a toast) when it spans sentences. */
export function takeMark(): Mark | null {
  const range = pendingRange;
  if (!range) return null;
  const sa = range.startContainer.parentElement?.closest<HTMLElement>('.s');
  const sb = range.endContainer.parentElement?.closest<HTMLElement>('.s');
  if (!sa || sa !== sb) {
    toast(T('请在一句之内选择', 'Select within one sentence'));
    hideMarkPop();
    return null;
  }
  const pre = document.createRange();
  pre.selectNodeContents(sa);
  pre.setEnd(range.startContainer, range.startOffset);
  const base = Number(sa.dataset.a);
  const a = base + pre.toString().length;
  const text = range.toString();
  const b = a + text.length;
  const p = Number(sa.closest('p')?.dataset.n ?? '1') - 1;
  window.getSelection()?.removeAllRanges();
  hideMarkPop();
  return { p, a, b, text };
}
