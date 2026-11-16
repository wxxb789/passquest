// Actions (prototype ACT table) and data loading. UI-only actions change `ui` and re-render; domain
// actions call the bound StudyApi and replace the cached data with whatever the core returns. The
// window never fabricates a result: without a bound service every call resolves to `not_connected`.
import { api, NOT_CONNECTED, onGoEvent, parseCallError, type StudyApi } from './api/client';
import type { AssessmentState, CoachMode, Feedback, NoteFields, NoteRefinement, NoteRequest, PracticeRequest, StudioError } from './api/types';
import { hideMarkPop, lightSpan, markCountText, takeMark, passageHTML, practiceFoot } from './components/passage';
import { labelForKey } from './components/options';
import { COACH_MODES, errorTitle, nextStep, T } from './core/i18n';
import { closeLayer, focusKey, openLayer, render, renderLayer, renderOrb } from './core/render';
import { go, subAction, type Location } from './core/router';
import { data, idle, ui, valueOf, type Loadable, type Route } from './core/state';
import { savePref } from './core/prefs';
import { toast } from './core/toast';
import { activeRun, assessState, practiceItem } from './screens';
import { previewHTML, ankiStatus, pushSummary, refinePanelHTML } from './views/anki';
import { opensItem } from './views/today';
import { optionSpan } from './views/feedback';

type Result<T> = { ok: true; value: T } | { ok: false; error: StudioError };

const $ = <E extends Element = HTMLElement>(sel: string) => document.querySelector<E>(sel);
const reduced = () => ui.shot || matchMedia('(prefers-reduced-motion: reduce)').matches;
const ready = <T>(value: T): Loadable<T> => ({ status: 'ready', value });

/**
 * A failure as one toast line in the window's words: the localized title and the localized next step.
 * The core's English message and CLI step are not toast text (failure cards keep them as details).
 */
const failureLine = (e: StudioError) => {
  const next = nextStep(e.code);
  return next ? `${errorTitle(e.code)} · ${next}` : errorTitle(e.code);
};

async function call<T>(fn: (a: StudyApi) => Promise<T>): Promise<Result<T>> {
  const a = api();
  if (!a) return { ok: false, error: NOT_CONNECTED };
  try {
    return { ok: true, value: await fn(a) };
  } catch (e) {
    const error = parseCallError(e);
    if (error.code === 'assessment_active' && ui.route !== 'assess') {
      toast(T('测评进行中，先回到测评', 'An assessment is in progress — back to the run'));
      go('assess');
    }
    return { ok: false, error };
  }
}

/** Loads into a slot and re-renders the stage when the route is still the one that asked. */
async function load<T>(fn: (a: StudyApi) => Promise<T>, set: (l: Loadable<T>) => void, prev?: T): Promise<T | undefined> {
  const route = ui.route;
  set(prev === undefined ? { status: 'loading' } : { status: 'loading', value: prev });
  const r = await call(fn);
  set(r.ok ? ready(r.value) : { status: 'error', error: r.error });
  if (ui.route === route) render(true, false);
  return r.ok ? r.value : undefined;
}

/**
 * True, with a toast, while another write is in flight: the new write is refused (never queued, so
 * nothing runs later against a screen that has moved on). Callers that recover from a failed write
 * check this first, so a refused call is never mistaken for a core failure.
 */
function busyRefused(): boolean {
  if (!ui.busy) return false;
  toast(T('上一步还在进行，稍后再试', 'Still working on the previous step — try again in a moment'));
  return true;
}

/** A write: one at a time, re-rendering around it. Failures surface as a toast. */
async function write<T>(fn: (a: StudyApi) => Promise<T>): Promise<T | undefined> {
  if (busyRefused()) return undefined;
  const route = ui.route;
  ui.busy = true;
  render(true, false);
  const r = await call(fn);
  ui.busy = false;
  // The learner moved on while the call ran: the room now shown was rendered busy (disabled).
  if (ui.route !== route) render(true, false);
  if (!r.ok) {
    toast(failureLine(r.error));
    render(true, false);
    return undefined;
  }
  return r.value;
}

/** A write whose result is not needed: resolves true on success. */
async function writeOk(fn: (a: StudyApi) => Promise<unknown>): Promise<boolean> {
  return (await write(async (a) => {
    await fn(a);
    return true as const;
  })) === true;
}

// ---------------------------------------------------------------- loaders

export const loadToday = () => load((a) => a.today(), (l) => (data.today = l), valueOf(data.today));
const loadServices = () => load((a) => a.services(), (l) => (data.services = l), valueOf(data.services));
const loadModels = () => load((a) => a.models(), (l) => (data.models = l), valueOf(data.models));

async function loadPracticeResume(): Promise<void> {
  if (data.practice.status === 'ready' || data.practice.status === 'loading') return;
  await load((a) => a.resumePractice(), (l) => (data.practice = l));
}

/**
 * The send preview of the current mode. `afterSend`: a reply just arrived; the preview is read again
 * in place (no loading state) so the call strip shows the budget slot the send spent.
 */
let previewSeq = 0;

async function loadCoachPreview(afterSend = false): Promise<void> {
  const fb = data.feedback;
  if (!fb) return;
  const mode = ui.coachMode;
  // Every request (even one that stops here) supersedes reads still in flight: only the newest lands.
  const seq = ++previewSeq;
  if (data.coach.replies[mode] && !afterSend) return;
  if (!afterSend) {
    data.coach.preview = { status: 'loading' };
    renderOrb();
  }
  const r = await call((a) => a.coachPreview(fb.attempt_id, mode));
  if (seq !== previewSeq || ui.coachMode !== mode || data.feedback !== fb) return;
  data.coach.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  renderOrb();
}

/** Puts a run's state on screen and follows it: a finished run opens its report. */
function showAssessment(s: AssessmentState): void {
  data.assess.state = ready(s);
  ui.assess.sel = null;
  if ((s.state === 'completed' || s.state === 'abandoned') && s.run) {
    ui.assess.attached = false;
    ui.report.runId = s.run.id;
    data.report.report = idle();
    data.report.runs = idle();
    toast(
      s.state === 'completed'
        ? T(`第 ${s.run.id} 轮已完成`, `Run #${s.run.id} is complete`)
        : T(`第 ${s.run.id} 轮已放弃（超过时限）`, `Run #${s.run.id} was abandoned (past its window)`),
    );
    void loadToday();
    go('report');
    return;
  }
  if (ui.route === 'assess') render(true, false);
}

/** Asks the core to present the run's current item; it records any timeout that came due. */
async function presentItem(runId: number): Promise<void> {
  const r = await call((a) => a.presentAssessmentItem(runId));
  if (r.ok) showAssessment(r.value);
  else {
    data.assess.state = { status: 'error', error: r.error };
    render(true, false);
  }
}

export async function refreshAssess(): Promise<void> {
  const s = await load((a) => a.assessmentState(), (l) => (data.assess.state = l), valueOf(data.assess.state));
  if (!s) return;
  if (s.state === 'item' && s.run) {
    if (ui.assess.attached && !s.item) await presentItem(s.run.id);
    return;
  }
  ui.assess.attached = false;
  await load((a) => a.assessmentPool(), (l) => (data.assess.pool = l));
}

async function loadReport(): Promise<void> {
  const runs = await load((a) => a.assessmentRuns(), (l) => (data.report.runs = l));
  if (!runs) return;
  const finished = runs.filter((r) => r.status !== 'active');
  const id = ui.report.runId ?? (finished.find((r) => r.status === 'completed') ?? finished[0])?.id ?? null;
  if (id === null) return;
  ui.report.runId = id;
  const current = valueOf(data.report.report);
  if (!current || current.run.id !== id) await load((a) => a.assessmentReport(id), (l) => (data.report.report = l));
  // By default the shown run is compared with the newest earlier completed run of the same shape
  // (size, window, pool family), else with the newest earlier one, whose refusal says what differs.
  if (ui.report.compareWith === null) {
    const base = runs.find((r) => r.id === id);
    const earlier = runs.filter((r) => r.id !== id && r.status === 'completed' && (!base || r.started_at < base.started_at));
    const like = base ? earlier.find((r) => r.actual_size === base.actual_size && r.window_minutes === base.window_minutes && r.pool_family === base.pool_family) : undefined;
    ui.report.compareWith = (like ?? earlier[0])?.id ?? null;
  }
  if (ui.report.compareWith !== null) await loadCompare(ui.report.compareWith);
}

/** Compares the other run (first) with the shown one (second): changes read as shown − other. */
async function loadCompare(other: number): Promise<void> {
  const base = ui.report.runId;
  if (base === null) return;
  await load((a) => a.assessmentCompare(other, base), (l) => (data.report.compare = l));
}

/** The attempt's recorded coach calls; rendered in the coach sheet's history view. */
async function loadCoachHistory(): Promise<void> {
  const fb = data.feedback;
  if (!fb) return;
  data.coach.history = { status: 'loading' };
  renderOrb();
  const r = await call((a) => a.coachHistory(fb.attempt_id));
  if (data.feedback !== fb) return;
  data.coach.history = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  renderOrb();
}

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
function seedRefine(suggestion: NoteRefinement | null | undefined): void {
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
  if (!inside) return;
  const fresh = $('.es-refine');
  if (!fresh) return;
  const same = fk ? fresh.querySelector<HTMLElement>(fk) : null;
  const next = focusable(same) ? same : (REFINE_NEXT.map((s) => fresh.querySelector<HTMLElement>(s)).find(focusable) ?? fresh);
  next.focus({ preventScroll: true });
}

/**
 * What a refinement would send, from the core; nothing is sent. The consent ledger is read with it,
 * so the panel can tell the every-provider grant notes need from a provider's own refusal.
 */
async function loadRefinePreview(): Promise<void> {
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

async function loadPreview(): Promise<void> {
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

async function loadAnki(): Promise<void> {
  if (data.services.status === 'idle') void loadServices();
  if (data.models.status === 'idle') void loadModels();
  if (!data.anki.draft) await restoreDraft();
  await load((a) => a.notes(), (l) => (data.anki.notes = l), valueOf(data.anki.notes));
  if (data.anki.draft && data.anki.preview.status !== 'ready') await loadPreview();
}

const loadConsents = () => load((a) => a.consents(), (l) => (data.settings.consents = l), valueOf(data.settings.consents));

async function loadSettings(): Promise<void> {
  const s = data.settings;
  await Promise.all([
    loadModels(),
    loadServices(),
    load((a) => a.budget(), (l) => (s.budget = l), valueOf(s.budget)),
    loadConsents(),
  ]);
  if (ui.layer?.startsWith('sheet:')) renderLayer(false);
}

export async function loadRoute(force = false): Promise<void> {
  if (force || data.today.status === 'idle') void loadToday();
  if (data.services.status === 'idle') void loadServices();
  switch (ui.route) {
    case 'today':
      if (!force && data.today.status !== 'idle') await loadToday();
      break;
    case 'practice':
      if (force) data.practice = idle();
      await loadPracticeResume();
      break;
    case 'feedback':
      if (data.models.status === 'idle') void loadModels();
      await loadCoachPreview();
      break;
    case 'assess':
      await refreshAssess();
      break;
    case 'report':
      await loadReport();
      break;
    case 'anki':
      await loadAnki();
      break;
    case 'settings':
      await loadSettings();
      break;
  }
  if (ui.layer?.startsWith('sheet:')) renderLayer(false);
}

/**
 * Interruption recovery on boot: an active assessment takes the window to its resume card; else an
 * item that was displayed and never answered comes back with its original token and clock.
 */
export async function recoverOnBoot(): Promise<void> {
  const t = await loadToday();
  if (t?.active_run) {
    if (ui.route !== 'assess') go('assess');
    return;
  }
  const r = await call((a) => a.resumePractice());
  if (r.ok && r.value) {
    data.practice = ready(r.value);
    if (ui.route === 'today' || ui.route === 'practice') {
      toast(T('回到上次没答完的题 · 用时从当初展示时算起', 'Back to the item you left · your time counts from when it was shown'));
      if (ui.route === 'practice') render(true, false);
      else go('practice');
    }
  }
}

/** A brief resumed in the background reports through `brief:changed`: Today reloads. */
export function watchBrief(): void {
  onGoEvent<{ phase: string; error: StudioError | null }>('brief:changed', (e) => {
    if (e.phase === 'finished' && e.error) toast(failureLine(parseCallError(e.error)));
    void loadToday();
  });
}

// ---------------------------------------------------------------- routing

/**
 * Leaving a run this window drives, by any path: the window stops driving it, so coming back goes
 * through the resume card and `assess resume` (one resume counted), whichever way the learner left.
 */
function detachRun(): void {
  const pos = assessState()?.item?.position;
  ui.assess.attached = false;
  ui.assess.leaveTo = null;
  toast(
    pos !== undefined
      ? T(`测评停在第 ${pos + 1} 题 · 计时继续`, `Run left at item ${pos + 1} · the clock keeps running`)
      : T('已离开测评 · 计时继续', 'Left the run · the clock keeps running'),
  );
}

/** Route changes asked for inside the window: an attached run is left through the leave dialog. */
function navigate(route: Route): void {
  if (ui.route === 'assess' && route !== 'assess' && ui.assess.attached && activeRun()) {
    ui.assess.leaveTo = route;
    openLayer('leave');
    return;
  }
  go(route);
}

export function onRoute(loc: Location): void {
  // Any other way out (a link, history) still detaches, exactly as the leave dialog would.
  if (ui.route === 'assess' && loc.route !== 'assess' && ui.assess.attached) detachRun();
  // A refinement preview is only true for the provider, consent and budget of the moment it was read:
  // leaving the room drops it, so a return never shows (or sends from) a stale one. A suggestion
  // already received stays: it sends nothing, and "Preview again" reads a fresh preview. A send in
  // flight keeps the whole state, so its paid, stored reply lands in the panel the learner returns to
  // (its preview cannot send again while it waits, nor after the reply or a failure).
  if (ui.route === 'anki' && loc.route !== 'anki' && !data.anki.refine.sending) seedRefine(data.anki.refine.suggestion);
  ui.route = loc.route;
  ui.sub = loc.sub;
  ui.layer = null;
  hideMarkPop();
  const sa = subAction(loc);
  if (loc.route === 'anki') ui.anki.editing = sa?.kind === 'edit';
  if (sa?.kind === 'nocoach') ui.coachOpen = false;
  render(true, !ui.shot);
  if (sa?.kind === 'ann') {
    ui.report.annItem = sa.item;
    openLayer('sheet:ann');
  } else if (sa?.kind === 'sheet') openLayer(`sheet:${sa.name}`);
  else if (sa?.kind === 'hint') {
    const item = practiceItem();
    if (item?.hint_available && !item.hint) openLayer('hint');
  }
  const stage = $('#stage');
  if (stage) stage.scrollTop = 0;
  void loadRoute();
}

// ---------------------------------------------------------------- targeted updates

function setChecked(v: string): void {
  document.querySelectorAll<HTMLElement>('.opt').forEach((o) => o.setAttribute('aria-checked', String(o.dataset.v === v)));
}

function syncPressed(act: string, v: string): void {
  document.querySelectorAll<HTMLElement>(`[data-act="${act}"]`).forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.v === v)));
}

function refreshPreview(): void {
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

export function setEv(label: string): void {
  const fb = data.feedback;
  if (!fb || ui.feedback.ev === label) return;
  ui.feedback.ev = label;
  document.querySelectorAll<HTMLElement>('.fo').forEach((f) => f.classList.toggle('on', f.dataset.ev === label));
  const span = optionSpan(fb, label);
  if (span) lightSpan(fb.passage.paragraphs, span, true, !reduced());
}

export function doMark(): boolean {
  const m = takeMark();
  if (!m) return false;
  const item = practiceItem();
  if (!item) return false;
  if (!ui.practice.marks.some((x) => x.p === m.p && x.a < m.b && m.a < x.b)) ui.practice.marks.push(m);
  const box = $('#passage');
  if (!box) return true;
  const top = box.scrollTop;
  const fresh = document.createElement('div');
  fresh.innerHTML = passageHTML(item.passage, { mode: 'practice', marks: ui.practice.marks, foot: practiceFoot(ui.practice.marks.length) }).value;
  box.innerHTML = fresh.querySelector('#passage')?.innerHTML ?? '';
  box.scrollTop = top;
  const foot = box.parentElement?.querySelector('.pas-foot');
  const freshFoot = fresh.querySelector('.pas-foot');
  if (foot && freshFoot) foot.innerHTML = freshFoot.innerHTML;
  const count = $('#markcount');
  if (count) count.textContent = markCountText(ui.practice.marks.length);
  return true;
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

// ---------------------------------------------------------------- practice helpers

function requestFor(v: string | undefined): PracticeRequest {
  // `next` opens exactly what the adaptive recommendation names (its follow-up for a lesson).
  return v === 'sample' ? { kind: 'sample' } : { kind: 'next' };
}

async function startPractice(req: PracticeRequest): Promise<void> {
  if (busyRefused()) return;
  closeLayer();
  const item = await write((a) => a.startPractice(req));
  if (!item) return;
  data.practice = ready(item);
  data.feedback = null;
  ui.practice = { sel: null, marks: [] };
  if (ui.route === 'practice') render(true, !ui.shot);
  else go('practice');
}

function resetCoach(): void {
  data.coach = { preview: idle(), replies: {}, failed: null, failure: null, sending: false, history: idle() };
  ui.coachHistory = false;
}

async function answered(fbPromise: (a: StudyApi, token: string) => Promise<Feedback>): Promise<void> {
  const item = practiceItem();
  // A second submit while the first is in flight is refused here, before the failure path below.
  if (!item || busyRefused()) return;
  const fb = await write((a) => fbPromise(a, item.token));
  if (!fb) {
    // The core refused: a closed or changed item cannot be answered, so show its current state.
    data.practice = idle();
    void loadPracticeResume();
    return;
  }
  data.feedback = fb;
  data.practice = ready(null);
  ui.feedback.ev = null;
  ui.coachOpen = true;
  resetCoach();
  void loadToday();
  go('feedback');
}

async function propose(req: NoteRequest): Promise<void> {
  const d = await write((a) => a.proposeNote(req));
  if (!d) return;
  data.anki.draft = d;
  data.anki.preview = idle();
  ui.anki.fields = null;
  ui.anki.dirty = false;
  resetRefine();
  if (ui.route === 'anki') void loadAnki();
  else go('anki');
}

function consentArgs(v: string | undefined): [string, string] | null {
  const [source, provider] = (v ?? '').split('|');
  return source ? [source, provider ?? ''] : null;
}

/** After a consent change the preview and the ledger are stale. */
function consentChanged(): void {
  data.coach.preview = idle();
  if (ui.route === 'feedback') void loadCoachPreview();
  if (ui.route === 'settings') void loadSettings();
  if (ui.route === 'anki') {
    data.anki.preview = idle();
    void loadPreview();
    if (ui.anki.refine && !data.anki.refine.suggestion) void loadRefinePreview();
  }
  render(true, false);
}

// ---------------------------------------------------------------- the action table

type Act = (v: string | undefined, el: HTMLElement) => void;

export const ACT: Record<string, Act> = {
  theme() {
    ui.theme = ui.theme === 'light' ? 'dark' : 'light';
    savePref('theme', ui.theme);
    render(false, false);
    $('#themebtn')?.focus({ preventScroll: true });
    if (ui.route === 'anki') {
      refreshPreview();
      syncPressed('ctheme', ui.anki.card ?? ui.theme);
    }
  },
  lang(v) {
    ui.lang = v === 'zh' || v === 'en' ? v : ui.lang === 'zh' ? 'en' : 'zh';
    savePref('lang', ui.lang);
    render();
    if (ui.layer) renderLayer(false);
  },
  keys() {
    if (ui.layer === 'keys') closeLayer();
    else openLayer('keys');
  },
  close() {
    closeLayer();
  },
  sheet(v) {
    if (v) openLayer(`sheet:${v}`);
  },
  reload() {
    void loadRoute(true);
  },
  coach() {
    if (ui.route === 'assess') return;
    if (ui.route === 'feedback') {
      ui.coachOpen = !ui.coachOpen;
      $('#stage')?.classList.toggle('with-coach', ui.coachOpen);
      renderOrb();
      $('.orb')?.focus({ preventScroll: true });
      return;
    }
    if (ui.layer === 'pop') closeLayer();
    else openLayer('pop');
  },
  more() {
    const sc = $('#coscroll');
    sc?.scrollBy({ top: sc.clientHeight * 0.7, behavior: reduced() ? 'auto' : 'smooth' });
  },

  // today
  learner() {
    openLayer('learner');
  },
  'learner-save'() {
    const form = $<HTMLFormElement>('#learnerform');
    if (!form) return;
    const fd = new FormData(form);
    const date = String(fd.get('exam_date') ?? '').trim();
    const mins = String(fd.get('daily_minutes') ?? '').trim();
    const minutes = mins ? Number(mins) : 0;
    if (!(Number.isInteger(minutes) && minutes >= 0 && minutes <= 1440)) {
      toast(T('每日时长需为 1–1440 的整数分钟', 'Daily time must be a whole number of minutes, 1–1440'));
      return;
    }
    void writeOk((a) => a.setLearnerSettings({ exam_date: date, daily_minutes: minutes })).then((ok) => {
      if (!ok) return;
      closeLayer();
      void loadToday();
    });
  },
  start(v) {
    void startPractice(requestFor(v));
  },
  'start-skill'(v) {
    if (v) void startPractice({ kind: 'skill', skill: v });
  },
  'start-tactic'(v) {
    const [tactic, stage] = (v ?? '').split('|');
    if (tactic) void startPractice({ kind: 'tactic', tactic, stage: stage ?? '' });
  },
  'brief-resume'() {
    void writeOk((a) => a.resumeBrief()).then((ok) => {
      if (!ok) return;
      toast(T('简报已在本地继续', 'The brief resumed locally'));
      closeLayer();
      void loadToday();
    });
  },
  'brief-cancel'() {
    void writeOk((a) => a.cancelBrief()).then((ok) => {
      if (!ok) return;
      closeLayer();
      void loadToday();
    });
  },

  // practice
  pick(v) {
    if (!v) return;
    ui.practice.sel = v;
    setChecked(v);
    const s = $<HTMLButtonElement>('#submit');
    if (s) s.disabled = ui.busy;
  },
  hint() {
    const item = practiceItem();
    if (item?.hint_available && !item.hint) openLayer('hint');
  },
  'hint-yes'() {
    const item = practiceItem();
    if (!item || busyRefused()) return;
    ui.layer = null;
    renderLayer(false);
    void write((a) => a.openHint(item.token)).then((next) => {
      if (!next) {
        render(true, false);
        return;
      }
      data.practice = ready(next);
      // The hint opens in the drawer under the passage; the passage keeps its scroll.
      render(true, false);
      $('#stage')?.classList.add('enter-hint');
      ($('.opt[aria-checked="true"]') ?? $('.opt'))?.focus({ preventScroll: true });
    });
  },
  skip() {
    void answered((a, token) => a.skip(token));
  },
  submit() {
    const sel = ui.practice.sel;
    if (!sel) return;
    void answered((a, token) => a.answer(token, sel));
  },
  next() {
    void startPractice({ kind: 'next' });
  },
  'propose-note'() {
    const fb = data.feedback;
    if (!fb) return;
    const mark = ui.practice.marks.at(-1)?.text;
    void propose(mark ? { attempt_id: fb.attempt_id, mark } : { attempt_id: fb.attempt_id });
  },
  'mark-note'() {
    const item = practiceItem();
    const mark = ui.practice.marks.at(-1)?.text;
    if (!item || !mark) return;
    void propose({ content_id: item.content_id, question_id: item.question_id, mark });
  },

  // coach
  mode(v) {
    const m = COACH_MODES.find(([k]) => k === v)?.[0];
    if (!m) return;
    ui.coachMode = m as CoachMode;
    ui.coachHistory = false;
    data.coach.failed = null;
    data.coach.failure = null;
    data.coach.preview = idle();
    renderOrb();
    $(`[data-act="mode"][data-v="${m}"]`)?.focus({ preventScroll: true });
    void loadCoachPreview();
  },
  'coach-send'() {
    const fb = data.feedback;
    const mode = ui.coachMode;
    if (!fb || data.coach.sending) return;
    data.coach.sending = true;
    data.coach.failed = null;
    data.coach.failure = null;
    renderOrb();
    void call((a) => a.coachAsk(fb.attempt_id, mode)).then((r) => {
      data.coach.sending = false;
      if (data.feedback !== fb) return;
      // The call is recorded either way: the history is stale.
      data.coach.history = idle();
      // Only an advisory reply is kept for the mode; a failure stays retryable (coach-retry).
      if (r.ok && r.value.state === 'advisory') data.coach.replies[mode] = r.value;
      else if (ui.coachMode === mode) {
        if (r.ok) data.coach.failed = r.value;
        else data.coach.failure = r.error;
      }
      renderOrb();
      void loadToday();
      // The call spent a budget slot: the strip reads the core's count again.
      if (ui.coachMode === mode) void loadCoachPreview(true);
    });
  },
  'coach-history'() {
    if (!data.feedback) return;
    ui.coachHistory = !ui.coachHistory;
    renderOrb();
    $('[data-act="coach-history"]')?.focus({ preventScroll: true });
    if (ui.coachHistory) void loadCoachHistory();
  },
  'coach-retry'() {
    if (data.coach.sending) return;
    data.coach.failed = null;
    data.coach.failure = null;
    data.coach.preview = idle();
    renderOrb();
    void loadCoachPreview();
  },
  'consent-grant'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.grantConsent(k[0], k[1])).then((ok) => ok && consentChanged());
  },
  'consent-revoke'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.revokeConsent(k[0], k[1])).then((ok) => ok && consentChanged());
  },
  'consent-all-ask'(v) {
    if (!v) return;
    ui.anki.consentSource = v;
    // The dialog lists the registry's providers the grant reaches and names the source by its title
    // and the providers keeping their own refusal (from a fresh ledger read); the loads re-render it in place.
    if (data.models.status === 'idle' || data.models.status === 'error') void loadModels();
    void loadConsents();
    openLayer('consent-all');
  },
  'consent-all'(v) {
    if (!v || v !== ui.anki.consentSource) return;
    closeLayer();
    ui.anki.consentSource = null;
    void writeOk((a) => a.grantConsent(v, '')).then((ok) => ok && consentChanged());
  },
  'consent-form'() {
    const form = $<HTMLFormElement>('#consentform');
    if (!form) return;
    const fd = new FormData(form);
    const source = String(fd.get('source') ?? '');
    const provider = String(fd.get('provider') ?? '');
    if (source && provider) ACT['consent-grant']!(`${source}|${provider}`, form);
  },

  // assessment
  'run-ask'() {
    openLayer('start-run');
  },
  'run-start'() {
    const form = $<HTMLFormElement>('#runform');
    if (!form) return;
    const fd = new FormData(form);
    const size = Number(fd.get('size'));
    const minutes = Number(fd.get('minutes'));
    if (!Number.isInteger(size) || size < 1 || !Number.isInteger(minutes) || minutes < 1 || minutes > 1440) {
      toast(T('题数和时长需为正整数', 'Size and minutes must be positive whole numbers'));
      return;
    }
    closeLayer();
    void write((a) => a.startAssessment(size, minutes)).then((s) => {
      if (!s) return;
      ui.assess.attached = true;
      showAssessment(s);
      void loadToday();
    });
  },
  'run-resume'() {
    const run = activeRun();
    if (!run || busyRefused()) return;
    void write((a) => a.resumeAssessment(run.id)).then((s) => {
      if (!s) {
        void refreshAssess();
        return;
      }
      ui.assess.attached = true;
      showAssessment(s);
    });
  },
  'run-abandon-ask'() {
    openLayer('abandon-run');
  },
  'run-abandon'() {
    const run = activeRun();
    closeLayer();
    if (!run) return;
    void write((a) => a.abandonAssessment(run.id)).then((s) => {
      if (s) showAssessment(s);
    });
  },
  apick(v) {
    if (!v) return;
    ui.assess.sel = v;
    setChecked(v);
    const n = $<HTMLButtonElement>('#anext');
    if (n) n.disabled = ui.busy;
  },
  anext() {
    const sel = ui.assess.sel;
    if (sel) void answerItem(sel);
  },
  'askip-item'() {
    void answerItem('skip');
  },
  leave() {
    ui.assess.leaveTo = null;
    openLayer('leave');
  },
  'leave-yes'() {
    const to = ui.assess.leaveTo ?? 'today';
    ui.layer = null;
    detachRun();
    go(to);
  },

  // report
  cmp(v) {
    const id = Number(v);
    if (!Number.isFinite(id)) return;
    ui.report.compareWith = id;
    void loadCompare(id);
  },
  'report-run'(v) {
    const id = Number(v);
    if (!Number.isFinite(id)) return;
    ui.report.runId = id;
    ui.report.compareWith = null;
    data.report.compare = idle();
    data.report.report = idle();
    void loadReport();
  },
  ann(v) {
    ui.report.annItem = Number(v);
    ui.layer = 'sheet:ann';
    renderLayer(false);
    $(`#layer [data-act="ann"][data-v="${v}"]`)?.focus({ preventScroll: true });
  },

  // anki
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
    if (!ui.anki.refine) {
      ui.anki.refine = true;
      data.anki.refine = { preview: idle(), suggestion: null, failure: null, sending: false };
    }
    render();
    $('.es-refine [data-act="refine-close"]')?.focus({ preventScroll: true });
    void loadRefinePreview();
  },
  'refine-close'() {
    resetRefine();
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
      if (data.anki.draft !== d || data.anki.refine !== r) return;
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
    if (!d || !f) return;
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
    if (!d) return;
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

  // settings
  row(v) {
    if (!v) return;
    ui.settings.open = ui.settings.open === v ? null : v;
    render();
  },
  'model-use'(v) {
    if (!v) return;
    void write((a) => a.useModel(v, '')).then((res) => {
      if (!res) return;
      toast(T(`已使用 ${res.provider_name} · ${res.active.model ?? ''}`, `Using ${res.provider_name} · ${res.active.model ?? ''}`));
      void loadSettings();
      void loadToday();
    });
  },
  'model-test'(v) {
    if (!v) return;
    void write((a) => a.testModel(v, '')).then((res) => {
      if (!res) return;
      toast(res.failure ? `${T('测试未通过', 'Test failed')} · ${failureLine({ code: res.failure.code, message: res.failure.message, action: null })}` : T(`${res.provider_name} · ${res.model} 已验证`, `${res.provider_name} · ${res.model} verified`));
      void loadSettings();
    });
  },
  cred(v) {
    if (!v) return;
    ui.settings.credentialFor = v;
    openLayer('credential');
  },
  'credential-save'() {
    const provider = ui.settings.credentialFor;
    const input = $<HTMLInputElement>('#credform input[name="key"]');
    if (!provider || !input) return;
    const key = input.value;
    // the key must not linger in the DOM, in state, or in a re-render
    input.value = '';
    if (!key.trim()) return;
    closeLayer();
    void writeOk((a) => a.setCredential(provider, key)).then((ok) => {
      if (!ok) return;
      ui.settings.credentialFor = null;
      toast(T('密钥已存入系统密钥库', 'The key is in the OS credential store'));
      void loadSettings();
      void loadToday();
    });
  },
  'cred-clear'(v) {
    if (!v) return;
    void writeOk((a) => a.clearCredential(v)).then((ok) => {
      if (!ok) return;
      void loadSettings();
      void loadToday();
    });
  },
};

async function answerItem(label: string): Promise<void> {
  const s = assessState();
  const item = s?.item;
  if (!s?.run || !item || busyRefused()) return;
  const res = await write((a) => a.answerAssessmentItem(item.run_id, item.position, label));
  if (!res) {
    void refreshAssess();
    return;
  }
  if (res.outcome === 'timed_out') toast(T(`第 ${item.position + 1} 题已超时，记为超时`, `Item ${item.position + 1} was past its clock and is recorded as timed out`));
  if (res.state.state !== 'item' || !res.state.run) {
    showAssessment(res.state);
    return;
  }
  await presentItem(res.state.run.id);
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

// ---------------------------------------------------------------- keyboard bindings

export const keyActions = {
  closeLayer,
  exitAnkiEdit: () => ACT.edit!(undefined, document.body),
  hideMarkPop,
  keys: () => ACT.keys!(undefined, document.body),
  theme: () => ACT.theme!(undefined, document.body),
  lang: () => ACT.lang!(undefined, document.body),
  go: navigate,
  todayStart: () => {
    // Only a Today that loaded and is on screen has a start action; loading or failed → nothing.
    if (data.today.status !== 'ready') return;
    const t = data.today.value;
    if (t.active_run) go('assess');
    else if (!t.recommendation) ACT.start!('sample', document.body);
    else if (opensItem(t.recommendation)) ACT.start!('recommended', document.body);
  },
  practicePick: (key: string) => {
    const item = practiceItem();
    const label = item ? labelForKey(item.options, key) : null;
    if (label) ACT.pick!(label, document.body);
    return label !== null;
  },
  practiceSubmit: () => ACT.submit!(undefined, document.body),
  practiceHint: () => ACT.hint!(undefined, document.body),
  practiceSkip: () => ACT.skip!(undefined, document.body),
  practiceMark: () => doMark(),
  coachMode: (i: number) => ACT.mode!(COACH_MODES[i]?.[0], document.body),
  feedbackNext: () => ACT.next!(undefined, document.body),
  feedbackNote: () => ACT['propose-note']!(undefined, document.body),
  feedbackCycle: (step: 1 | -1) => {
    const fb = data.feedback;
    if (!fb || fb.options.length === 0) return;
    const labels = fb.options.map((o) => o.label);
    const i = labels.indexOf(ui.feedback.ev ?? fb.key.label);
    setEv(labels[(i + step + labels.length) % labels.length]!);
  },
  assessPick: (key: string) => {
    const item = assessState()?.item;
    const label = item && ui.assess.attached ? labelForKey(item.options, key) : null;
    if (label) ACT.apick!(label, document.body);
    return label !== null;
  },
  assessConfirm: () => ACT.anext!(undefined, document.body),
  assessSkip: () => {
    if (ui.assess.attached) ACT['askip-item']!(undefined, document.body);
  },
  ankiEdit: () => ACT.edit!(undefined, document.body),
};

/** Called by the display clock when a server deadline passes: the core decides what happened. */
export function onDeadline(): void {
  const s = assessState();
  if (ui.assess.attached && s?.run && s.state === 'item') void presentItem(s.run.id);
  else if (ui.route === 'assess') void refreshAssess();
}
