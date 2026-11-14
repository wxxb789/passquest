// Anki (prototype vAnki + editSheetHTML + the sync/conflict sheets). The card preview is the real
// `note.RenderCard` HTML with the real model CSS, rendered inside `<iframe sandbox="" srcdoc>` so the
// card markup never enters the app document. Pass Quest proposes; the learner edits, accepts or skips.
import type { NoteDraft, NoteFields, NoteList, NotePreview, NoteSummary } from '../api/types';
import { html, type Html } from '../core/html';
import { ANKI_STATE, T } from '../core/i18n';
import type { Loadable, Theme } from '../core/state';
import { sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { errorCard, loadingCard, retryButton } from './states';

export interface AnkiUi {
  side: 'both' | 'front' | 'back';
  card: Theme | null;
  width: 'desk' | 'phone';
  editing: boolean;
  dirty: boolean;
  busy: boolean;
  theme: Theme;
}

export const nextRevision = (d: NoteDraft): number => (d.revision ?? 0) + 1;

export function ankiStatus(d: NoteDraft | null, dirty: boolean): Html {
  if (!d) return html`<span class="chip sm">${T('没有草稿', 'No draft')}</span>`;
  if (d.status === 'skipped') return html`<span class="chip sm">${T('已跳过 · 不生成笔记', 'Skipped · no note')}</span>`;
  if (d.status === 'accepted' && !dirty)
    return html`<span class="chip ok sm">${ic('check', 'xs')}${T(`已接受 · 修订 ${d.revision ?? 1}`, `Accepted · revision ${d.revision ?? 1}`)}</span>`;
  const rev = d.revision ?? 0;
  return html`<span class="chip cool sm">${rev ? T(`草稿 · 修订 ${rev}`, `Draft · revision ${rev}`) : T('草稿', 'Draft')}${dirty ? T(' · 已修改', ' · edited') : ''}</span>`;
}

function srcdoc(body: string, css: string, dark: boolean): string {
  // The whole document is one attribute value; html`` escapes it and the browser decodes it back
  // into the sandboxed frame. No script runs there (sandbox="" without allow-scripts).
  return `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style></head><body class="card${dark ? ' nightMode night_mode' : ''}">${body}</body></html>`;
}

export function previewHTML(p: Loadable<NotePreview>, u: AnkiUi): Html {
  if (p.status === 'error') return html`<div class="pv-row"><p class="fine">${p.error.message}</p></div>`;
  if (p.status !== 'ready') return html`<div class="pv-row"><p class="fine">${T('正在渲染预览……', 'Rendering the preview…')}</p></div>`;
  const v = p.value;
  const dark = (u.card ?? u.theme) === 'dark';
  const sides = u.side === 'both' ? (['front', 'back'] as const) : [u.side];
  return html`<div class="pv-row">${sides.map(
    (s) => html`<div class="pv-side"><div class="pv-lab"><b>${s === 'front' ? T('正面', 'Front') : T('背面', 'Back')}</b><span>${
      u.width === 'phone' ? T('手机宽度 · 300 px', 'Phone · 300 px') : T('桌面宽度 · 500 px', 'Desktop · 500 px')
    }</span></div><iframe class="ac-frame" sandbox="" referrerpolicy="no-referrer" title="${T('卡片预览', 'Card preview')}" data-sha="${v.html_sha256}" srcdoc="${srcdoc(
      s === 'front' ? v.front_html : v.back_html,
      v.css,
      dark,
    )}"></iframe></div>`,
  )}</div>`;
}

function syncs(notes: Loadable<NoteList>): Html {
  if (notes.status !== 'ready') return html``;
  const c = notes.value.counts;
  return html`<div class="syncs" role="group" aria-label="${T('同步状态', 'Sync state')}">
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
  const s = d.source;
  const id = s ? [s.source_id, s.page !== null ? `p${String(s.page).padStart(3, '0')}` : null, s.question_id].filter(Boolean).join(' · ') : '';
  const from = d.attempt_id !== null ? T(`来自作答 #${d.attempt_id}`, `From attempt #${d.attempt_id}`) : T('来自文章标记', 'From a passage mark');
  return `${from}${id ? ' · ' + id : ''} · ${T('由 Pass Quest 提议，你来决定', 'proposed by Pass Quest, your call')}`;
}

function editSheetHTML(d: NoteDraft, f: NoteFields, u: AnkiUi): Html {
  const canAccept = d.status !== 'accepted' || u.dirty;
  const fl = (zh: string, en: string, id: string) => html`<span class="fl">${T(zh, en)}<span class="id">${id}</span></span>`;
  const src = d.source ? [d.source.source_id, d.source.page !== null ? `p${String(d.source.page).padStart(3, '0')}` : null, d.source.question_id].filter(Boolean).join(' · ') : '—';
  return html`<section class="esheet" role="region" aria-label="${T('编辑笔记字段', 'Edit note fields')}">
    <div class="grab"></div>
    <div class="es-b">
      <div>
        <label class="field">${fl('正面', 'Front', 'Front')}<textarea class="serif one" data-field="front" lang="en" rows="1">${f.front}</textarea></label>
        <label class="field">${fl('战术', 'Tactic', 'Tactic')}<textarea class="one" data-field="tactic" lang="en" rows="1">${f.tactic}</textarea></label>
        <div class="field">${fl('来源', 'Source', 'Source')}<div class="ro">${src}<span>${T('只存编号，不存来源标题', 'id only, never the title')}</span></div></div>
      </div>
      <div>
        <label class="field">${fl('背面 · 解释', 'Back · explanation', 'Back ¶1')}<textarea data-field="back_explanation" lang="en" rows="3">${f.back_explanation}</textarea></label>
        <label class="field">${fl('背面 · 例句', 'Back · example', 'Back ¶2')}<textarea class="serif" data-field="back_example" lang="en" rows="1">${f.back_example}</textarea></label>
      </div>
    </div>
    <div class="es-f">
      <span id="estatus">${ankiStatus(d, u.dirty)}</span><span class="muted" style="font-size:12px">${T('改动实时显示在上方卡片', 'Changes show live on the card above')}</span>
      <span class="sp"></span>
      <button class="btn quiet sm" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <button class="btn primary sm" data-act="accept" id="acceptbtn2" ${canAccept && !u.busy ? '' : 'disabled'}>${ic('check', 'xs')}${T(`接受 · 修订 ${nextRevision(d)}`, `Accept · revision ${nextRevision(d)}`)}</button>
      <button class="btn ghost sm" data-act="edit">${T('完成', 'Done')}<kbd>Esc</kbd></button>
    </div>
  </section>`;
}

export function vAnki(d: NoteDraft | null, f: NoteFields | null, preview: Loadable<NotePreview>, notes: Loadable<NoteList>, u: AnkiUi): Html {
  if (notes.status === 'error' && !d) return errorCard(notes.error, retryButton('reload'));
  if (notes.status !== 'ready' && notes.status !== 'error' && !d) return loadingCard();
  const tgl = <K extends string>(act: string, label: string, cur: K, items: readonly (readonly [K, string, string, Html | ''])[]) =>
    html`<div class="tgl" role="group" aria-label="${label}">${items.map(
      ([k, z, e, i]) => html`<button data-act="${act}" data-v="${k}" aria-pressed="${cur === k}">${i}${T(z, e)}</button>`,
    )}</div>`;
  const pushBtn = html`<button class="btn ghost" data-act="push" ${u.busy ? 'disabled' : ''}>${ic('arrow', 'xs')}${T('推送到 Anki', 'Push to Anki')}</button>`;
  if (!d)
    return html`<div class="scene anki" id="ankiscene">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记草稿', 'Note draft')}</span><span id="astatus">${ankiStatus(null, false)}</span><span class="muted">${T(
        '在反馈里提议一条笔记，或先在练习文章中标记文字。',
        'Propose a note from feedback, or mark passage text during practice first.',
      )}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table rise" style="--i:1" id="preview"><div class="pv-row"><p class="fine">${T('没有草稿可预览。', 'No draft to preview.')}</p></div></div>
    <div class="an-tools"></div>
    <div class="an-acts settle" style="--i:3">${pushBtn}</div>
  </div>`;
  const fields = f ?? d.fields;
  const canAccept = (d.status !== 'accepted' || u.dirty) && !u.busy;
  return html`<div class="scene anki ${u.editing ? 'editing' : ''}" id="ankiscene" data-draft="${d.draft_id}">
    <div class="an-top settle" style="--i:0">
      <div class="src"><span class="t">${T('笔记草稿', 'Note draft')}</span><span id="astatus">${ankiStatus(d, u.dirty)}</span><span class="muted">${sourceLine(d)}</span></div>
      ${syncs(notes)}
    </div>
    <div class="an-table ${u.width === 'phone' ? 'phone' : ''} rise" style="--i:1" id="preview">${previewHTML(preview, u)}</div>
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
      <button class="btn primary" data-act="accept" id="acceptbtn" ${canAccept ? '' : 'disabled'}>${ic('check', 'xs')}${T(`接受 · 修订 ${nextRevision(d)}`, `Accept · revision ${nextRevision(d)}`)}</button>
      <button class="btn ghost" data-act="edit">${ic('edit', 'xs')}${T('编辑字段', 'Edit fields')}<kbd>E</kbd></button>
      <button class="btn quiet" data-act="askip" ${u.busy ? 'disabled' : ''}>${T('跳过', 'Skip')}</button>
      <span class="div"></span>
      ${pushBtn}
    </div>
    ${u.editing ? editSheetHTML(d, fields, u) : ''}
  </div>`;
}

// ---------------------------------------------------------------- sheets

function noteRow(n: NoteSummary): Html {
  const st = ANKI_STATE[n.state];
  return html`<div class="note ${st[3]}" data-note="${n.note_id}">${ic(st[2], 's')}<span class="f" lang="en">${n.front}</span><span class="st">${T(st[0], st[1])}${
    n.revision ? html` · ${T(`修订 ${n.revision}`, `rev ${n.revision}`)}` : ''
  }${n.state === 'conflict' ? html`<button class="btn ghost sm" data-act="sheet" data-v="conflict">${T('处理', 'Resolve')}</button>` : ''}</span></div>`;
}

function reach(l: NoteList): Html {
  const r = l.anki.reachable;
  const state =
    r === null
      ? html`<span class="muted">${T('未检查', 'not checked')}</span>`
      : r
        ? html`<span style="color:var(--ok)">${T('可连接', 'reachable')}</span>`
        : html`<span style="color:var(--bad)">${T('无法连接', 'unreachable')}</span>`;
  return html`${T('本地', 'local')} · <code>${l.anki.endpoint}</code> · ${state}`;
}

export function ankiSheet(key: string, notes: Loadable<NoteList>, busy: boolean): Html | null {
  if (notes.status !== 'ready') return null;
  const l = notes.value;
  if (key === 'sync')
    return sheetWrap(
      'anki',
      T('笔记与同步', 'Notes & sync'),
      reach(l),
      html`${l.notes.length ? l.notes.map(noteRow) : html`<p class="fine">${T('还没有接受的笔记。', 'No accepted notes yet.')}</p>`}
      <div class="sh-acts"><button class="btn ghost sm" data-act="push" ${busy ? 'disabled' : ''}>${T('推送排队的笔记', 'Push queued notes')}</button></div>
      <p class="fine" style="display:flex;gap:8px;align-items:center;margin-top:14px">${ic('phone', 'xs')}${T('手机同步通过 AnkiWeb，由你自己操作。', 'Phone sync via AnkiWeb is your own action.')}</p>`,
    );
  if (key === 'conflict') {
    const conflicts = l.notes.filter((n) => n.state === 'conflict');
    return sheetWrap(
      'conflict',
      T('冲突', 'Conflict'),
      T('你在 Anki 中改过这些笔记，不会被静默覆盖', 'These notes changed in Anki. Nothing is overwritten silently'),
      html`${
        conflicts.length
          ? conflicts.map(
              (n) => html`<div class="blk"><h3 lang="en">${n.front}</h3><p>${n.code ? html`<code>${n.code}</code> ` : ''}${n.detail ?? ''}</p></div>`,
            )
          : html`<p class="fine">${T('没有冲突。', 'No conflicts.')}</p>`
      }
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
