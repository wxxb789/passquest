// Anki (prototype vAnki + editSheetHTML + the sync/conflict sheets). The card preview is the real
// `note.RenderCard` HTML with the real model CSS (PreviewNote), rendered inside
// `<iframe sandbox="" srcdoc>` so the card markup never enters the app document. Pass Quest
// proposes; the learner edits, accepts or skips; the outbox pushes.
import type { Note, NoteDraft, NoteFields, NoteList, NotePreview, NotePush } from '../api/types';
import { html, type Html } from '../core/html';
import { ankiStateOf, errorLabel, errorTitle, T } from '../core/i18n';
import type { Loadable, Theme } from '../core/state';
import { sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { errorCard, errorDetail, loadingCard, retryButton } from './states';

export interface AnkiUi {
  card: Theme | null;
  width: 'desk' | 'phone';
  editing: boolean;
  dirty: boolean;
  busy: boolean;
  theme: Theme;
}

export function ankiStatus(d: NoteDraft | null, dirty: boolean): Html {
  if (!d) return html`<span class="chip sm">${T('没有草稿', 'No draft')}</span>`;
  return html`<span class="chip cool sm" data-testid="draft-status" data-draft="${d.id}">${T('草稿 · 等你决定', 'Draft · your call')}${dirty ? T(' · 已修改', ' · edited') : ''}</span>`;
}

function srcdoc(body: string, css: string, dark: boolean): string {
  // The whole document is one attribute value; html`` escapes it and the browser decodes it back
  // into the sandboxed frame. No script runs there (sandbox="" without allow-scripts).
  // Anki's night mode marks an ancestor of `.card` (the model CSS uses `.nightMode .card`).
  return `<!doctype html><html${dark ? ' class="nightMode night_mode"' : ''}><head><meta charset="utf-8"><style>${css}</style></head><body class="card">${body}</body></html>`;
}

export function previewHTML(p: Loadable<NotePreview>, u: AnkiUi, d: NoteDraft | null): Html {
  if (p.status === 'error') {
    const source = d?.target.source_id;
    const grant =
      p.error.code === 'consent_required' && source
        ? // Notes accept only the global (every-provider) decision, so the scope is named and confirmed first.
          html`<div class="acts-row" style="margin-top:10px"><button class="btn ghost sm" data-act="consent-all-ask" data-v="${source}" ${u.busy ? 'disabled' : ''}>${ic('shield', 'xs')}${T(
            `授权 ${source} 的原文…`,
            `Allow the text of ${source}…`,
          )}</button><span class="muted">${T('笔记只接受对所有提供方生效的授权；下一步会列出范围', 'Notes accept only a grant for every provider; the next step shows that scope')}</span></div>`
        : '';
    return html`<div class="pv-row"><div class="fine" data-testid="preview-error" data-code="${p.error.code}"><b>${errorLabel(p.error.code)}</b> ${errorDetail(p.error)}${grant}</div></div>`;
  }
  if (p.status !== 'ready') return html`<div class="pv-row"><p class="fine">${T('正在渲染预览……', 'Rendering the preview…')}</p></div>`;
  const v = p.value;
  const dark = (u.card ?? u.theme) === 'dark';
  return html`<div class="pv-row"><div class="pv-side"><div class="pv-lab"><b>${T('卡片', 'Card')} · <span class="id">${v.model}</span></b><span>${
    u.width === 'phone' ? T('手机宽度 · 300 px', 'Phone · 300 px') : T('桌面宽度 · 500 px', 'Desktop · 500 px')
  } · ${T('牌组', 'deck')} ${v.deck}</span></div><iframe class="ac-frame" sandbox="" referrerpolicy="no-referrer" title="${T('卡片预览', 'Card preview')}" data-testid="card-preview" data-sha="${v.html_sha256}" data-edited="${v.edited}" srcdoc="${srcdoc(
    v.html,
    v.css,
    dark,
  )}"></iframe></div></div>`;
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

function sourceLine(d: NoteDraft): string {
  const t = d.target;
  const id = [t.source_id, t.page ? `p${String(t.page).padStart(3, '0')}` : null, t.question_id].filter(Boolean).join(' · ');
  const from = t.mark ? T('来自文章标记', 'From a passage mark') : t.attempt_id ? T(`来自作答 #${t.attempt_id}`, `From attempt #${t.attempt_id}`) : T('来自题目', 'From a question');
  return `${from}${id ? ' · ' + id : ''} · ${T('由 Pass Quest 提议，你来决定', 'proposed by Pass Quest, your call')}`;
}

function editSheetHTML(d: NoteDraft, f: NoteFields, u: AnkiUi): Html {
  const fl = (zh: string, en: string, id: string) => html`<span class="fl">${T(zh, en)}<span class="id">${id}</span></span>`;
  return html`<section class="esheet" role="region" aria-label="${T('编辑笔记字段', 'Edit note fields')}">
    <div class="grab"></div>
    <div class="es-b">
      <div>
        <label class="field">${fl('正面', 'Front', 'Front')}<textarea class="serif one" data-field="front" lang="en" rows="2">${f.front}</textarea></label>
        <label class="field">${fl('战术', 'Tactic', 'Tactic')}<textarea class="one" data-field="tactic" lang="en" rows="1">${f.tactic}</textarea></label>
        <div class="field">${fl('来源', 'Source', 'Source')}<div class="ro">${d.source || '—'}<span>${T('只存编号，不存来源标题', 'id only, never the title')}</span></div></div>
      </div>
      <div>
        <label class="field">${fl('背面', 'Back', 'Back')}<textarea data-field="back" lang="en" rows="5">${f.back}</textarea></label>
      </div>
    </div>
    <div class="es-f">
      <span id="estatus">${ankiStatus(d, u.dirty)}</span><span class="muted" style="font-size:12px">${T('改动实时显示在上方卡片；清空的字段保留草稿原值', 'Changes show live on the card above; an emptied field keeps the draft value')}</span>
      <span class="sp"></span>
      <button class="btn quiet sm" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <button class="btn primary sm" data-act="accept" id="acceptbtn2" ${u.busy ? 'disabled' : ''}>${ic('check', 'xs')}${T('接受', 'Accept')}</button>
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

export function vAnki(d: NoteDraft | null, f: NoteFields | null, preview: Loadable<NotePreview>, notes: Loadable<NoteList>, u: AnkiUi): Html {
  if (notes.status === 'error' && !d) return errorCard(notes.error, retryButton('reload'));
  if (notes.status !== 'ready' && notes.status !== 'error' && !d) return loadingCard();
  const tgl = <K extends string>(act: string, label: string, cur: K, items: readonly (readonly [K, string, string, Html | ''])[]) =>
    html`<div class="tgl" role="group" aria-label="${label}">${items.map(
      ([k, z, e, i]) => html`<button data-act="${act}" data-v="${k}" aria-pressed="${cur === k}">${i}${T(z, e)}</button>`,
    )}</div>`;
  const pushBtn = html`<button class="btn ghost" data-act="push" ${u.busy ? 'disabled' : ''}>${ic('arrow', 'xs')}${T('推送到 Anki', 'Push to Anki')}</button>`;
  const list = notes.status === 'ready' ? notes.value.notes : [];
  if (!d)
    return html`<div class="scene anki" id="ankiscene" data-testid="anki">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记', 'Notes')}</span><span id="astatus">${ankiStatus(null, false)}</span><span class="muted">${T(
        '在反馈里提议一条笔记，或在练习文章中标记文字后生成。',
        'Propose a note from feedback, or mark passage text during practice.',
      )}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table rise" style="--i:1" id="preview"><div class="pv-row notes-list">${
      list.length ? list.slice(0, 8).map(noteRow) : html`<p class="fine">${T('还没有接受的笔记。', 'No accepted notes yet.')}</p>`
    }</div></div>
    <div class="an-tools"></div>
    <div class="an-acts settle" style="--i:3">${pushBtn}</div>
  </div>`;
  const fields = f ?? d.fields;
  return html`<div class="scene anki ${u.editing ? 'editing' : ''}" id="ankiscene" data-testid="anki" data-draft="${d.id}">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记草稿', 'Note draft')}</span><span id="astatus">${ankiStatus(d, u.dirty)}</span>${restrictedLine(d)}<span class="muted">${sourceLine(d)}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table ${u.width === 'phone' ? 'phone' : ''} rise" style="--i:1" id="preview">${previewHTML(preview, u, d)}</div>
    <div class="an-tools settle" style="--i:2">
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
      <button class="btn primary" data-act="accept" id="acceptbtn" ${u.busy ? 'disabled' : ''}>${ic('check', 'xs')}${T('接受', 'Accept')}</button>
      <button class="btn ghost" data-act="edit">${ic('edit', 'xs')}${T('编辑字段', 'Edit fields')}<kbd>E</kbd></button>
      <button class="btn quiet" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <span class="div"></span>
      ${pushBtn}
    </div>
    ${u.editing ? editSheetHTML(d, fields, u) : ''}
  </div>`;
}

// ---------------------------------------------------------------- sheets

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
  if (p.error) return `${errorTitle(p.error.code)} · ${base}${c.conflicts ? T(` · 冲突 ${c.conflicts}`, ` · conflicts ${c.conflicts}`) : ''}`;
  return base;
}

export function ankiSheet(key: string, notes: Loadable<NoteList>, endpoint: string | null, busy: boolean): Html | null {
  if (notes.status !== 'ready') return null;
  const l = notes.value;
  if (key === 'sync')
    return sheetWrap(
      'anki',
      T('笔记与同步', 'Notes & sync'),
      html`${T('本地', 'local')} · <code>${endpoint ?? T('端点未检查', 'endpoint not checked')}</code> · <span class="muted">${T('推送时才连接', 'contacted only on push')}</span>`,
      html`${l.notes.length ? l.notes.map(noteRow) : html`<p class="fine">${T('还没有接受的笔记。', 'No accepted notes yet.')}</p>`}
      <div class="sh-acts"><button class="btn ghost sm" data-act="push" ${busy ? 'disabled' : ''}>${T('推送排队的笔记', 'Push queued notes')}</button></div>
      <p class="fine" style="display:flex;gap:8px;align-items:center;margin-top:14px">${ic('phone', 'xs')}${T('手机同步通过 AnkiWeb，由你自己操作。', 'Phone sync via AnkiWeb is your own action.')}</p>`,
    );
  if (key === 'conflict') {
    const conflicts = l.notes.filter((n) => n.anki_state === 'conflict');
    return sheetWrap(
      'conflict',
      T('冲突', 'Conflict'),
      T('你在 Anki 中改过这些笔记，不会被静默覆盖', 'These notes changed in Anki. Nothing is overwritten silently'),
      html`${conflicts.length ? conflicts.map((n) => html`<div class="blk"><h3 lang="en">${n.fields.front}</h3><p class="muted">${n.note_id} · ${T(`修订 ${n.revision}`, `rev ${n.revision}`)}</p></div>`) : html`<p class="fine">${T('没有冲突。', 'No conflicts.')}</p>`}
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
