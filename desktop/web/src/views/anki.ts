// Anki (prototype vAnki + editSheetHTML + the sync/conflict sheets). The card preview is exactly the
// backend's documents (PreviewNote: question/answer × light/night, the same templates, stylesheet and
// body classes Anki uses), placed in `<iframe sandbox="allow-same-origin" srcdoc>`: no script runs in
// the frame, the card markup never enters the app document, and the window reads the frame's height
// so the card is never clipped and never scrolls. Pass Quest proposes; the learner edits, accepts or
// skips; the outbox pushes. "Refine with AI" previews the call, sends once on the learner's word and
// shows an advisory suggestion that only fills the editor when used.
import type { ModelList, Note, NoteDraft, NoteFields, NoteList, NotePreview, NotePush, NoteRefinePreview, NoteRefinement, StudioError } from '../api/types';
import { html, type Html } from '../core/html';
import { ankiStateOf, errorTitle, T } from '../core/i18n';
import type { Loadable, Theme } from '../core/state';
import { callStrip, providerName } from '../components/callstrip';
import { failureNote } from '../components/failure';
import { sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { whoMeta } from './coach';
import { errorCard, loadingCard, retryButton } from './states';

export type CardSide = 'both' | 'front' | 'back';

export interface AnkiUi {
  card: Theme | null;
  width: 'desk' | 'phone';
  side: CardSide;
  editing: boolean;
  refine: boolean;
  dirty: boolean;
  busy: boolean;
  theme: Theme;
  /** The provider registry, for provider names. */
  models: ModelList | null;
}

/** The refinement strip's data (data.anki.refine). */
export interface AnkiRefine {
  preview: Loadable<NoteRefinePreview>;
  suggestion: NoteRefinement | null;
  failure: StudioError | null;
  sending: boolean;
}

export function ankiStatus(d: NoteDraft | null, dirty: boolean): Html {
  if (!d) return html`<span class="chip sm">${T('没有草稿', 'No draft')}</span>`;
  return html`<span class="chip cool sm" data-testid="draft-status" data-draft="${d.id}">${T('草稿 · 等你决定', 'Draft · your call')}${dirty ? T(' · 已修改', ' · edited') : ''}</span>`;
}

/**
 * Sets a card frame's height to its document's height: nothing is clipped and nothing scrolls. The
 * root element's box is the content's height whatever the frame's current viewport (the model CSS
 * sizes nothing by the viewport), so no script has to run inside the frame.
 */
export function fitCardFrame(frame: HTMLIFrameElement): void {
  const doc = frame.contentDocument;
  const fit = () => {
    const root = frame.contentDocument?.documentElement;
    if (root) frame.style.height = `${Math.ceil(root.getBoundingClientRect().height)}px`;
  };
  fit();
  // Once more after the frame has laid out at its new height and its fonts are in.
  requestAnimationFrame(fit);
  void doc?.fonts?.ready.then(fit);
}

const SIDES: Record<CardSide, readonly ('question' | 'answer')[]> = {
  both: ['question', 'answer'],
  front: ['question'],
  back: ['answer'],
};

export function previewHTML(p: Loadable<NotePreview>, u: AnkiUi, d: NoteDraft | null): Html {
  if (p.status === 'error') {
    const source = d?.target.source_id;
    const grant =
      p.error.code === 'consent_required' && source
        ? // Notes accept only the global (every-provider) decision, so the scope is named and confirmed first.
          html`<div class="acts-row" style="margin-top:10px"><button class="btn ghost sm" data-act="consent-all-ask" data-v="${source}" ${u.busy ? 'disabled' : ''}>${ic('shield', 'xs')}${T(
            '授权这份资料的原文…',
            "Allow this source's text…",
          )}</button><span class="muted">${T('笔记只接受对所有提供方生效的授权；下一步会列出范围', 'Notes accept only a grant for every provider; the next step shows that scope')}</span></div>`
        : '';
    return html`<div class="pv-row">${failureNote(p.error, { testid: 'preview-error', extra: grant })}</div>`;
  }
  if (p.status !== 'ready') return html`<div class="pv-row"><p class="fine">${T('正在渲染预览……', 'Rendering the preview…')}</p></div>`;
  const v = p.value;
  const night = (u.card ?? u.theme) === 'dark';
  const theme = night ? 'night' : 'light';
  const width = u.width === 'phone' ? T('手机宽度 · 300 px', 'Phone · 300 px') : T('桌面宽度 · 500 px', 'Desktop · 500 px');
  return html`<div class="pv-row" data-testid="card-preview" data-sha="${v.fields_sha256}" data-css-sha="${v.css_sha256}" data-model-version="${v.model_version}" data-edited="${v.edited}" data-side="${u.side}" data-theme="${theme}">${SIDES[u.side].map(
    (side) => html`<div class="pv-side"><div class="pv-lab"><b>${side === 'question' ? T('正面', 'Front') : T('背面', 'Back')}</b><span>${width} · ${T('牌组', 'deck')} <span translate="no">${v.deck}</span></span></div><iframe class="ac-frame" sandbox="allow-same-origin" scrolling="no" referrerpolicy="no-referrer" title="${
      side === 'question' ? T('卡片正面预览', 'Card front preview') : T('卡片背面预览', 'Card back preview')
    }" data-testid="card-frame" data-side="${side}" data-theme="${theme}" srcdoc="${v[side][theme]}"></iframe></div>`,
  )}</div>`;
}

/** The card-design outcome of the last push, in the window's words; nothing when the design was current. */
export function modelLine(p: NotePush | null): Html {
  if (p?.model_customized)
    return html`<span class="chip warm sm" data-testid="model-state" data-state="customized" title="${T(
      '笔记照常推送；Pass Quest 不会覆盖你的改动',
      'The notes were pushed; Pass Quest does not overwrite your change',
    )}">${T('Anki 中的卡片样式是你改过的 · 保持原样', 'Card style changed in Anki · left as is')}</span>`;
  if (p?.model_updated)
    return html`<span class="chip ok sm" data-testid="model-state" data-state="updated">${ic('check', 'xs')}${T('Anki 中的卡片样式已换成新设计', 'Card style in Anki updated to the new design')}</span>`;
  return html``;
}

function syncs(notes: Loadable<NoteList>): Html {
  if (notes.status !== 'ready') return html``;
  const c = notes.value.counts;
  return html`<div class="syncs" role="group" aria-label="${T('同步状态', 'Sync state')}" data-testid="sync-counts" data-pushed="${c.pushed}" data-queued="${c.queued}" data-conflict="${c.conflict}">
        <button class="sy" data-act="sheet" data-v="sync">${ic('check', 'xs')}<b class="num">${c.pushed}</b>${T('已推送', 'pushed')}</button>
        <button class="sy warm" data-act="sheet" data-v="sync">${ic('clock', 'xs')}<b class="num">${c.queued}</b>${T('排队', 'queued')}</button>
        ${
          c.conflict
            ? html`<button class="sy bad" data-act="sheet" data-v="conflict">${ic('conflict', 'xs')}<b class="num">${c.conflict}</b>${T('冲突', 'conflict')}</button>`
            : html`<button class="sy ok" data-act="sheet" data-v="sync">${ic('check', 'xs')}${T('无冲突', 'no conflict')}</button>`
        }
        <button class="sy" data-act="sheet" data-v="sync">${ic('present', 'xs')}<b class="num">${c.already_present}</b>${T('已存在', 'present')}</button>
      </div>`;
}

/** Where the draft came from, in words: the ids (source, question) stay in the draft's data. */
function sourceLine(d: NoteDraft): string {
  const t = d.target;
  const from = t.mark ? T('来自文章标记', 'From a passage mark') : t.attempt_id ? T(`来自第 ${t.attempt_id} 次作答`, `From attempt #${t.attempt_id}`) : T('来自题目', 'From a question');
  const page = t.page ? T(` · 第 ${t.page} 页`, ` · page ${t.page}`) : '';
  return `${from}${page} · ${T('由 Pass Quest 提议，你来决定', 'proposed by Pass Quest, your call')}`;
}

/** A refinement failure: the localized failure line and Retry (which only re-previews). */
function refineFailure(e: StudioError, busy: boolean): Html {
  return failureNote(e, { testid: 'refine-failure', retry: html`<button class="btn quiet sm" data-act="refine-retry" ${busy ? 'disabled' : ''}>${T('重试', 'Retry')}</button>` });
}

function suggestionHTML(s: NoteRefinement, busy: boolean, models: ModelList | null): Html {
  return html`<div class="reply rf-sug" data-testid="refine-suggestion" data-refinement="${s.refinement_id}" data-sha="${s.request_sha256}">
      <div class="field"><span class="fl">${T('建议 · 正面', 'Suggested · Front')}</span><p class="serif" lang="en" data-testid="suggested-front">${s.suggestion.front}</p></div>
      <div class="field"><span class="fl">${T('建议 · 背面', 'Suggested · Back')}</span><p lang="en" data-testid="suggested-back">${s.suggestion.back}</p></div>
      <div class="rmeta">${ic('adv', 'xs')}${T('建议性 · 不会自动采用', 'Advisory · never applied by itself')}<span class="sep"></span>${whoMeta(s.provider, s.model, models)}<span class="sep"></span><span class="num nw">${(s.latency_ms / 1000).toFixed(1)} s</span></div>
    </div>
    <div class="acts-row"><button class="btn adv sm" data-act="refine-use" ${busy ? 'disabled' : ''}>${T('使用建议', 'Use suggestion')}</button><button class="btn quiet sm" data-act="refine-retry" ${busy ? 'disabled' : ''}>${T('再预览一次', 'Preview again')}</button></div>
    <p class="fine">${T('使用建议只会填入左侧字段，接受仍由你来点。', 'Using it only fills the fields on the left; accepting stays your step.')}</p>`;
}

function refinePreview(d: NoteDraft, r: AnkiRefine, busy: boolean, models: ModelList | null): Html {
  const p = r.preview;
  if (p.status === 'error') return refineFailure(p.error, busy);
  if (p.status !== 'ready') return html`<div class="pending">${T('正在准备发送预览……', 'Preparing the send preview…')}</div>`;
  const v = p.value;
  if (!v.provider) return refineFailure({ code: 'not_configured', message: '', action: null }, busy);
  const source = d.target.source_id;
  // Notes accept only the every-provider decision, so a private source is granted through its dialog.
  const consent =
    v.consent.private_source && !v.consent.granted && source
      ? html`<span data-consent="withheld"><span class="held">${T('这份资料的原文已扣留', "This source's text is withheld")}</span>
      <button class="btn ghost sm" data-act="consent-all-ask" data-v="${source}" ${busy ? 'disabled' : ''}>${T('授权原文…', 'Allow the text…')}</button></span>`
      : undefined;
  const where = v.external ? T('外部调用', 'external call') : T('本地调用', 'local call');
  const canSend = v.consent.granted && !r.sending && !busy;
  return html`${callStrip(v, busy, models, 'refine-strip', consent)}
    <div class="pending">${T(`发送将进行一次${where}，消耗 1 次预算`, `Sending makes one ${where} and spends 1 budget slot`)} <span class="num nw">(${v.budget.calls} → ${v.budget.calls + 1} / ${v.budget.limit})</span>${T('。', '.')}${
      v.unanswered ? html`<div class="muted">${T('这道题还没作答：建议不会写出答案。', 'The question is unanswered: the suggestion may not reveal its answer.')}</div>` : ''
    }${!v.verified && v.action ? html`<div class="muted">${T('下一步：', 'Next: ')}${v.action}</div>` : ''}
      <div><button class="btn adv sm" data-act="refine-send" ${canSend ? '' : 'disabled'}>${ic(v.external ? 'ext' : 'local', 'xs')}${
        r.sending ? T('发送中……', 'Sending…') : html`${T('发送到 ', 'Send to ')}<span translate="no">${providerName(models, v.provider)}</span>`
      }</button></div></div>`;
}

export function refinePanelHTML(d: NoteDraft, r: AnkiRefine, busy: boolean, models: ModelList | null): Html {
  const body = r.suggestion ? suggestionHTML(r.suggestion, busy, models) : r.failure ? refineFailure(r.failure, busy) : refinePreview(d, r, busy, models);
  return html`<div class="es-refine" data-testid="refine" role="region" aria-label="${T('AI 润色', 'Refine with AI')}">
      <div class="rf-h"><span class="chip adv sm">${ic('adv', 'xs')}${T('AI 润色 · 建议性', 'Refine with AI · advisory')}</span><button class="btn quiet sm" data-act="refine-close">${T('收起', 'Close')}</button></div>
      ${body}
    </div>`;
}

function editSheetHTML(d: NoteDraft, f: NoteFields, u: AnkiUi, r: AnkiRefine): Html {
  const fl = (zh: string, en: string) => html`<span class="fl">${T(zh, en)}</span>`;
  return html`<section class="esheet" role="region" aria-label="${T('编辑笔记字段', 'Edit note fields')}">
    <div class="grab"></div>
    <div class="es-b ${u.refine ? 'refining' : ''}">
      <div>
        <label class="field">${fl('正面', 'Front')}<textarea class="serif one" data-field="front" lang="en" rows="2">${f.front}</textarea></label>
        <label class="field">${fl('战术', 'Tactic')}<textarea class="one" data-field="tactic" lang="en" rows="1">${f.tactic}</textarea></label>
        <div class="field">${fl('来源', 'Source')}<div class="ro"><span data-raw translate="no">${d.source || '—'}</span><span>${T('卡片上只写编号，不写来源标题', 'the card carries the id only, never the title')}</span></div></div>
      </div>
      <div>
        <label class="field">${fl('背面', 'Back')}<textarea data-field="back" lang="en" rows="5">${f.back}</textarea></label>
      </div>
      ${u.refine ? refinePanelHTML(d, r, u.busy, u.models) : ''}
    </div>
    <div class="es-f">
      <span id="estatus">${ankiStatus(d, u.dirty)}</span><span class="muted" style="font-size:12px">${T('改动实时显示在上方卡片；清空的字段保留草稿原值', 'Changes show live on the card above; an emptied field keeps the draft value')}</span>
      <span class="sp"></span>
      ${u.refine ? '' : html`<button class="btn adv sm" data-act="refine">${ic('adv', 'xs')}${T('AI 润色', 'Refine with AI')}</button>`}
      <button class="btn quiet sm" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <button class="btn primary sm" data-act="accept" id="acceptbtn2" data-primary ${u.busy ? 'disabled' : ''}>${ic('check', 'xs')}${T('接受', 'Accept')}</button>
      <button class="btn ghost sm" data-act="edit">${T('完成', 'Done')}<kbd>Esc</kbd></button>
    </div>
  </section>`;
}

function restrictedLine(d: NoteDraft): Html {
  if (!d.restricted) return html``;
  return d.consent_granted
    ? html`<span class="chip sm">${ic('shield', 'xs')}${T('私有来源 · 已授权', 'Private source · consent recorded')}</span>`
    : html`<span class="chip warm sm">${ic('shield', 'xs')}${T('私有来源 · 原文未写入卡片', 'Private source · its text is not on the card')}</span>`;
}

export function vAnki(
  d: NoteDraft | null,
  f: NoteFields | null,
  preview: Loadable<NotePreview>,
  notes: Loadable<NoteList>,
  u: AnkiUi,
  refine: AnkiRefine,
  lastPush: NotePush | null,
): Html {
  if (notes.status === 'error' && !d) return errorCard(notes.error, retryButton('reload'));
  if (notes.status !== 'ready' && notes.status !== 'error' && !d) return loadingCard();
  const tgl = <K extends string>(act: string, label: string, cur: K, items: readonly (readonly [K, string, string, Html | ''])[]) =>
    html`<div class="tgl" role="group" aria-label="${label}">${items.map(
      ([k, z, e, i]) => html`<button data-act="${act}" data-v="${k}" aria-pressed="${cur === k}">${i}${T(z, e)}</button>`,
    )}</div>`;
  // Push only does something for notes not yet in Anki (local, queued, failed or conflicting); with
  // nothing waiting it is not offered.
  const waiting = notes.status === 'ready' ? pushable(notes.value) : 0;
  const pushBtn = html`<button class="btn ghost" data-act="push" ${u.busy ? 'disabled' : ''}>${ic('arrow', 'xs')}${T('推送到 Anki', 'Push to Anki')}</button>`;
  const list = notes.status === 'ready' ? notes.value.notes : [];
  if (!d)
    return html`<div class="scene anki" id="ankiscene" data-testid="anki">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记', 'Notes')}</span><span id="astatus">${ankiStatus(null, false)}</span>${modelLine(lastPush)}<span class="muted">${T(
        '在反馈里提议一条笔记，或在练习文章中标记文字后生成。',
        'Propose a note from feedback, or mark passage text during practice.',
      )}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table rise" style="--i:1" id="preview"><div class="pv-row notes-list">${
      list.length ? list.slice(0, 8).map(noteRow) : html`<p class="fine">${T('还没有接受的笔记。', 'No accepted notes yet.')}</p>`
    }</div></div>
    <div class="an-tools"></div>
    <div class="an-acts settle" style="--i:3">${
      waiting
        ? pushBtn
        : html`<span class="cap">${
            list.length ? T('所有笔记都已在 Anki 中 · 新接受的笔记从这里推送', 'Every note is in Anki · newly accepted notes push from here') : T('接受一条笔记后，从这里推送到 Anki', 'Accept a note first; it pushes to Anki from here')
          }</span>`
    }</div>
  </div>`;
  const fields = f ?? d.fields;
  return html`<div class="scene anki ${u.editing ? 'editing' : ''}" id="ankiscene" data-testid="anki" data-draft="${d.id}">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记草稿', 'Note draft')}</span><span id="astatus">${ankiStatus(d, u.dirty)}</span>${restrictedLine(d)}${modelLine(lastPush)}<span class="muted">${sourceLine(d)}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table ${u.width === 'phone' ? 'phone' : ''} rise" style="--i:1" id="preview">${previewHTML(preview, u, d)}</div>
    <div class="an-tools settle" style="--i:2">
      ${tgl('side', T('面', 'Side'), u.side, [
        ['both', '两面', 'Both', ''],
        ['front', '正面', 'Front', ''],
        ['back', '背面', 'Back', ''],
      ] as const)}
      ${tgl('ctheme', T('卡片主题', 'Card theme'), u.card ?? u.theme, [
        ['light', '浅色', 'Light', ic('sun', 'xs')],
        ['dark', '深色', 'Dark', ic('moon', 'xs')],
      ] as const)}
      ${tgl('cwidth', T('宽度', 'Width'), u.width, [
        ['desk', '桌面', 'Desktop', ic('local', 'xs')],
        ['phone', '手机', 'Phone', ic('phone', 'xs')],
      ] as const)}
    </div>
    <div class="an-acts settle" style="--i:3">
      <button class="btn primary" data-act="accept" id="acceptbtn" ${u.editing ? '' : 'data-primary'} ${u.busy ? 'disabled' : ''}>${ic('check', 'xs')}${T('接受', 'Accept')}</button>
      <button class="btn ghost" data-act="edit">${ic('edit', 'xs')}${T('编辑字段', 'Edit fields')}<kbd>E</kbd></button>
      <button class="btn quiet" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <span class="div"></span>
      <button class="btn adv sm" data-act="refine">${ic('adv', 'xs')}${T('AI 润色', 'Refine with AI')}</button><span class="cap">${T('建议性 · 先确认', 'Advisory · asks first')}</span>
      ${waiting ? html`<span class="div"></span>${pushBtn}` : ''}
    </div>
    ${u.editing ? editSheetHTML(d, fields, u, refine) : ''}
  </div>`;
}

// ---------------------------------------------------------------- sheets

/** Notes a push would act on: accepted but not in Anki yet, queued, failed or in conflict. */
const pushable = (l: NoteList): number => l.counts.local + l.counts.queued + l.counts.failed + l.counts.conflict;

function noteRow(n: Note): Html {
  const st = ankiStateOf(n.anki_state);
  return html`<div class="note ${st[3]}" data-testid="note" data-note="${n.note_id}" data-state="${n.anki_state}">${ic(st[2], 's')}<span class="f" lang="en">${n.fields.front}</span><span class="st">${T(st[0], st[1])} · ${T(
    `修订 ${n.revision}`,
    `rev ${n.revision}`,
  )}${n.anki_state === 'conflict' ? html`<button class="btn ghost sm" data-act="sheet" data-v="conflict">${T('处理', 'Resolve')}</button>` : ''}</span></div>`;
}

/** A push result in the window's words; the per-note outcomes are in the list. */
export function pushSummary(p: NotePush): string {
  const c = p.counts;
  if (p.state === 'queued')
    return T(`Anki 无法连接，${c.queued} 条笔记已排队，下次可连接时推送一次。`, `Anki is unreachable; ${c.queued} notes are queued and will push once when reachable.`);
  const base = T(`已推送 ${c.pushed} · 已存在 ${c.already_present} · 排队 ${c.queued}`, `pushed ${c.pushed} · already present ${c.already_present} · queued ${c.queued}`);
  const model = p.model_customized
    ? T(' · Anki 中的卡片样式是你改过的，保持原样', ' · the card style was changed in Anki and is left as is')
    : p.model_updated
      ? T(' · Anki 中的卡片样式已换成新设计', ' · the card style in Anki is now the new design')
      : '';
  if (p.error) return `${errorTitle(p.error.code)} · ${base}${c.conflicts ? T(` · 冲突 ${c.conflicts}`, ` · conflicts ${c.conflicts}`) : ''}${model}`;
  return base + model;
}

export function ankiSheet(key: string, notes: Loadable<NoteList>, endpoint: string | null, busy: boolean): Html | null {
  if (notes.status !== 'ready') return null;
  const l = notes.value;
  if (key === 'sync')
    return sheetWrap(
      'anki',
      T('笔记与同步', 'Notes & sync'),
      html`${T('本地 AnkiConnect', 'Local AnkiConnect')} · <span class="muted">${endpoint ? T('推送时才连接', 'contacted only on push') : T('端点未检查', 'endpoint not checked')}</span>`,
      html`${l.notes.length ? l.notes.map(noteRow) : html`<p class="fine">${T('还没有接受的笔记。', 'No accepted notes yet.')}</p>`}
      ${pushable(l) ? html`<div class="sh-acts"><button class="btn ghost sm" data-act="push" ${busy ? 'disabled' : ''}>${T('推送排队的笔记', 'Push queued notes')}</button></div>` : ''}
      <p class="fine" style="display:flex;gap:8px;align-items:center;margin-top:14px">${ic('phone', 'xs')}${T('手机同步通过 AnkiWeb，由你自己操作。', 'Phone sync via AnkiWeb is your own action.')}</p>`,
    );
  if (key === 'conflict') {
    const conflicts = l.notes.filter((n) => n.anki_state === 'conflict');
    return sheetWrap(
      'conflict',
      T('冲突', 'Conflict'),
      T('你在 Anki 中改过这些笔记，不会被静默覆盖', 'These notes changed in Anki. Nothing is overwritten silently'),
      html`${conflicts.length ? conflicts.map((n) => html`<div class="blk"><h3 lang="en">${n.fields.front}</h3><p class="muted">${T(`修订 ${n.revision}`, `rev ${n.revision}`)}</p></div>`) : html`<p class="fine">${T('没有冲突。', 'No conflicts.')}</p>`}
      <p class="fine">${T('Anki 一侧的字段内容没有保存，因此这里不能并排显示或采用 Anki 的版本。', "Anki's field values are not stored, so its version cannot be shown or adopted here.")}</p>
      ${
        conflicts.length
          ? html`<div class="sh-acts"><button class="btn ghost sm" data-act="push-force" ${busy ? 'disabled' : ''}>${T('保留 Pass Quest 版本（覆盖 Anki）', 'Keep Pass Quest’s (overwrite Anki)')}</button><button class="btn primary sm" data-act="close">${T('保持原样', 'Leave as is')}</button></div>`
          : ''
      }`,
      true,
    );
  }
  return null;
}
