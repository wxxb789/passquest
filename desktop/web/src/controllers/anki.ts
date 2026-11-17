// The Anki room: the open draft, its edited fields and real preview, the refinement panel, accepting,
// skipping and pushing. Targeted updates keep the preview, the status chips and the refinement panel
// current without re-rendering the stage under the learner's focus.
import type { NoteFields, NoteRefinement, NoteRequest } from '../api/types';
import { T } from '../core/i18n';
import { closeLayer, focusKey, render } from '../core/render';
import { go } from '../core/router';
import { data, idle, ui, valueOf } from '../core/state';
import { toast } from '../core/toast';
import { ankiStatus, previewHTML, pushSummary, refineHold, refinePanelHTML } from '../views/anki';
import { $, syncPressed, type Act } from './dom';
import { call, load, loadToday, ready, write, writeOk } from './service';
import { loadModels, loadServices } from './settings';

const ankiFields = (): NoteFields | null => ui.anki.fields ?? data.anki.draft?.fields ?? null;

/** The latest refinement-preview request: an older reply never overwrites what came after it. */
let refinePreviewGen = 0;

function resetRefine(): void {
  refinePreviewGen++;
  ui.anki.refine = false;
  data.anki.refine = { preview: idle(), suggestion: null, failure: null, sending: false };
}

/**
 * The refinement state for a draft: a stored suggestion the learner has not used comes back beside
 * the draft's own fields, in the refinement panel, and fills nothing until "Use suggestion".
 */
export function seedRefine(suggestion: NoteRefinement | null | undefined): void {
  resetRefine();
  if (!suggestion) return;
  ui.anki.refine = true;
  data.anki.refine.suggestion = suggestion;
}

/** The panel's next step after its content changed under the focus: the result, else the send. */
const REFINE_NEXT = ['[data-act="refine-use"]', '[data-act="refine-retry"]', '[data-act="refine-send"]'];
const focusable = (el: HTMLElement | null): el is HTMLElement => !!el && !(el as HTMLButtonElement).disabled;

/**
 * Re-renders only the refinement panel when it is on screen; otherwise the whole stage. Focus inside
 * the panel stays inside it: on the same control when it is still there and enabled, else on the
 * panel's next step, else on the panel itself (never on <body>).
 */
function refreshRefine(): void {
  const panel = $('.es-refine');
  const d = data.anki.draft;
  if (!panel || !d || !ui.anki.refine) {
    render(true, false);
    return;
  }
  const inside = panel.contains(document.activeElement);
  const fk = inside && document.activeElement !== panel ? focusKey(document.activeElement) : null;
  panel.outerHTML = refinePanelHTML(d, data.anki.refine, ui.busy, valueOf(data.models) ?? null, valueOf(data.settings.consents) ?? null).value;
  refreshHold();
  if (!inside) return;
  const fresh = $('.es-refine');
  if (!fresh) return;
  const same = fk ? fresh.querySelector<HTMLElement>(fk) : null;
  const next = focusable(same) ? same : (REFINE_NEXT.map((s) => fresh.querySelector<HTMLElement>(s)).find(focusable) ?? fresh);
  next.focus({ preventScroll: true });
}

/** Accept and Skip, outside the panel, follow a send starting and settling (refineHold). */
function refreshHold(): void {
  const r = data.anki.refine;
  document.querySelectorAll<HTMLButtonElement>('#ankiscene [data-act="accept"], #ankiscene [data-act="askip"]').forEach((b) => (b.disabled = ui.busy || r.sending));
  document.querySelectorAll<HTMLElement>('#ankiscene [data-hold]').forEach((el) => (el.innerHTML = refineHold(r).value));
}

/**
 * What a refinement would send, from the core; nothing is sent. The consent ledger is read with it,
 * so the panel can tell the every-provider grant notes need from a provider's own refusal.
 */
export async function loadRefinePreview(): Promise<void> {
  const d = data.anki.draft;
  if (!d) return;
  const gen = ++refinePreviewGen;
  data.anki.refine.preview = { status: 'loading' };
  refreshRefine();
  const [r, ledger] = await Promise.all([call((a) => a.refineNotePreview(d.id)), call((a) => a.consents())]);
  if (gen !== refinePreviewGen || data.anki.draft !== d || !ui.anki.refine) return;
  if (ledger.ok) data.settings.consents = ready(ledger.value);
  data.anki.refine.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  refreshRefine();
}

export async function loadPreview(): Promise<void> {
  const d = data.anki.draft;
  const f = ankiFields();
  if (!d || !f) return;
  const r = await call((a) => a.previewNote(d.id, f));
  if (data.anki.draft !== d) return;
  data.anki.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  refreshPreview();
}

/**
 * A draft proposed before a restart and never accepted or skipped lives only in the store: bring it
 * back. A failed lookup leaves the room as it is; the note list below surfaces a broken connection.
 */
async function restoreDraft(): Promise<void> {
  const r = await call((a) => a.openNoteDraft());
  // A draft proposed while the lookup ran is the newer one.
  if (!r.ok || !r.value || data.anki.draft) return;
  data.anki.draft = r.value;
  data.anki.preview = idle();
  ui.anki.fields = null;
  ui.anki.dirty = false;
  seedRefine(r.value.suggestion);
}

export async function loadAnki(): Promise<void> {
  if (data.services.status === 'idle') void loadServices();
  if (data.models.status === 'idle') void loadModels();
  if (!data.anki.draft) await restoreDraft();
  await load((a) => a.notes(), (l) => (data.anki.notes = l), valueOf(data.anki.notes));
  if (data.anki.draft && data.anki.preview.status !== 'ready') await loadPreview();
}

export function refreshPreview(): void {
  const p = $('#preview');
  if (!p || !data.anki.draft) return;
  p.classList.toggle('phone', ui.anki.width === 'phone');
  p.innerHTML = previewHTML(data.anki.preview, { ...ui.anki, busy: ui.busy, theme: ui.theme, models: valueOf(data.models) ?? null, consents: valueOf(data.settings.consents) ?? null }, data.anki.draft).value;
}

function refreshAnkiChrome(): void {
  const d = data.anki.draft;
  if (!d) return;
  for (const s of ['#astatus', '#estatus']) {
    const el = $(s);
    if (el) el.innerHTML = ankiStatus(d, ui.anki.dirty).value;
  }
}

let previewTimer: ReturnType<typeof setTimeout> | undefined;

/** `input` on an Anki field: keep the edit locally and refresh the real preview shortly after. */
export function onFieldInput(field: string, value: string): void {
  const f = ankiFields();
  if (!f || !(field in f)) return;
  ui.anki.fields = { ...f, [field]: value };
  ui.anki.dirty = true;
  refreshAnkiChrome();
  clearTimeout(previewTimer);
  previewTimer = setTimeout(() => void loadPreview(), 250);
}

export async function propose(req: NoteRequest): Promise<void> {
  const d = await write((a) => a.proposeNote(req));
  if (!d) return;
  const prev = data.anki.draft;
  data.anki.draft = d;
  data.anki.preview = idle();
  ui.anki.fields = null;
  ui.anki.dirty = false;
  // Proposing the same note again returns the same draft: a refinement still on its way stays its own.
  if (!(data.anki.refine.sending && prev?.id === d.id)) resetRefine();
  if (ui.route === 'anki') void loadAnki();
  else go('anki');
}

async function push(force: boolean): Promise<void> {
  const res = await write((a) => a.pushNotes(force));
  if (!res) return;
  data.anki.lastPush = res;
  toast(pushSummary(res));
  closeLayer();
  await loadAnki();
  void loadToday();
}

export const ankiActs: Record<string, Act> = {
  ctheme(v) {
    if (v !== 'light' && v !== 'dark') return;
    ui.anki.card = v;
    refreshPreview();
    syncPressed('ctheme', v);
  },
  side(v) {
    if (v !== 'both' && v !== 'front' && v !== 'back') return;
    ui.anki.side = v;
    refreshPreview();
    syncPressed('side', v);
  },
  cwidth(v) {
    if (v !== 'desk' && v !== 'phone') return;
    ui.anki.width = v;
    refreshPreview();
    syncPressed('cwidth', v);
  },
  edit() {
    if (!data.anki.draft) return;
    ui.anki.editing = !ui.anki.editing;
    render();
    if (ui.anki.editing) $('[data-field="front"]')?.focus();
    else $('[data-act="edit"]')?.focus();
    // The sheet reopens with the refinement panel: what Send would do is read again, never reused.
    if (ui.anki.editing && ui.anki.refine && !data.anki.refine.suggestion && !data.anki.refine.failure && !data.anki.refine.sending) void loadRefinePreview();
  },
  refine() {
    if (!data.anki.draft) return;
    ui.anki.editing = true;
    const r = data.anki.refine;
    // A send closed away while it waited comes back as it stands: pending, then its reply or failure,
    // never a fresh preview that could pay for the same draft a second time.
    const held = r.sending || !!r.suggestion || !!r.failure;
    if (!ui.anki.refine && !held) data.anki.refine = { preview: idle(), suggestion: null, failure: null, sending: false };
    ui.anki.refine = true;
    render();
    $('.es-refine [data-act="refine-close"]')?.focus({ preventScroll: true });
    if (!held) void loadRefinePreview();
  },
  'refine-close'() {
    // Closing hides a send in flight but keeps it: its paid reply lands in the panel when it reopens.
    if (data.anki.refine.sending) ui.anki.refine = false;
    else resetRefine();
    render();
    // The editor's own button: the room's one is hidden while the sheet is open.
    $(ui.anki.editing ? '.esheet [data-act="refine"]' : '[data-act="refine"]')?.focus({ preventScroll: true });
  },
  'refine-send'() {
    const d = data.anki.draft;
    const r = data.anki.refine;
    // Send only from a preview that is on screen and allows it.
    if (!d || r.sending || r.suggestion || r.failure || r.preview.status !== 'ready' || !r.preview.value.consent.granted) return;
    r.sending = true;
    r.failure = null;
    refreshRefine();
    void call((a) => a.refineNote(d.id)).then((res) => {
      r.sending = false;
      // The budget moved whether or not the panel is still there to show the reply.
      void loadToday();
      // The same draft proposed again while this waited is still this draft.
      if (data.anki.draft?.id !== d.id || data.anki.refine !== r) return;
      if (res.ok) r.suggestion = res.value;
      else r.failure = res.error;
      // Away from the room the state waits; the return renders it.
      if (ui.route === 'anki') refreshRefine();
    });
  },
  'refine-retry'() {
    const r = data.anki.refine;
    if (r.sending) return;
    // Retry only shows the preview again; the learner sends from it.
    r.failure = null;
    r.suggestion = null;
    void loadRefinePreview();
  },
  'refine-use'() {
    const s = data.anki.refine.suggestion;
    const f = ankiFields();
    if (!s || !f) return;
    // Fills the editor only; accepting stays the learner's action and sends these full fields.
    ui.anki.fields = { ...f, front: s.suggestion.front, back: s.suggestion.back };
    ui.anki.dirty = true;
    render();
    $('[data-field="front"]')?.focus({ preventScroll: true });
    void loadPreview();
  },
  accept() {
    const d = data.anki.draft;
    const f = ankiFields();
    // A refinement on its way holds the draft open (the buttons say so).
    if (!d || !f || data.anki.refine.sending) return;
    void write((a) => a.acceptNote(d.id, f)).then((res) => {
      if (!res) return;
      data.anki.draft = null;
      data.anki.preview = idle();
      ui.anki.fields = null;
      ui.anki.dirty = false;
      ui.anki.editing = false;
      resetRefine();
      toast(T(`已接受 · 修订 ${res.note.revision} · 推送时写入 Anki`, `Accepted · revision ${res.note.revision} · written to Anki on push`));
      void loadAnki();
      void loadToday();
    });
  },
  askip() {
    const d = data.anki.draft;
    if (!d || data.anki.refine.sending) return;
    void writeOk((a) => a.skipNote(d.id)).then((ok) => {
      if (!ok) return;
      data.anki.draft = null;
      data.anki.preview = idle();
      ui.anki.fields = null;
      ui.anki.dirty = false;
      ui.anki.editing = false;
      resetRefine();
      toast(T('已跳过 · 不生成笔记', 'Skipped · no note'));
      render();
    });
  },
  push() {
    void push(false);
  },
  'push-force'() {
    void push(true);
  },
};
