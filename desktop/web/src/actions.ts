// Actions (prototype ACT table) and data loading. UI-only actions change `ui` and re-render; domain
// actions call the bound StudyApi and replace the cached data with whatever the core returns. The
// window never fabricates a result: without a bound service every call resolves to `not_connected`.
import { api, NOT_CONNECTED, onGoEvent, parseCallError, type StudyApi } from './api/client';
import type { AssessmentState, CoachMode, Feedback, NoteFields, NoteRequest, PracticeRequest, StudioError } from './api/types';
import { hideMarkPop, lightSpan, markCountText, takeMark, passageHTML, practiceFoot } from './components/passage';
import { labelForKey } from './components/options';
import { COACH_MODES, errorTitle, SESSION_IDX, T } from './core/i18n';
import { closeLayer, openLayer, render, renderLayer, renderOrb } from './core/render';
import { go, subAction, type Location } from './core/router';
import { data, idle, ui, valueOf, type Loadable, type Route } from './core/state';
import { savePref } from './core/prefs';
import { toast } from './core/toast';
import { activeRun, assessState, practiceItem, today } from './screens';
import { previewHTML, ankiStatus, pushSummary } from './views/anki';
import { optionSpan } from './views/feedback';
import { qcolHTML } from './views/practice';
import { tickClocks } from './core/clock';

type Result<T> = { ok: true; value: T } | { ok: false; error: StudioError };

const $ = <E extends Element = HTMLElement>(sel: string) => document.querySelector<E>(sel);
const reduced = () => ui.shot || matchMedia('(prefers-reduced-motion: reduce)').matches;
const ready = <T>(value: T): Loadable<T> => ({ status: 'ready', value });

/** A failure as one toast line: the localized title, the core's message and its next step. */
const failureLine = (e: StudioError) => `${errorTitle(e.code)} · ${e.message}${e.action ? ' — ' + e.action : ''}`;

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

/** A write: one at a time, re-rendering around it. Failures surface as a toast. */
async function write<T>(fn: (a: StudyApi) => Promise<T>): Promise<T | undefined> {
  if (ui.busy) return undefined;
  ui.busy = true;
  render(true, false);
  const r = await call(fn);
  ui.busy = false;
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

async function loadCoachPreview(): Promise<void> {
  const fb = data.feedback;
  if (!fb) return;
  const mode = ui.coachMode;
  if (data.coach.replies[mode]) return;
  data.coach.preview = { status: 'loading' };
  renderOrb();
  const r = await call((a) => a.coachPreview(fb.attempt_id, mode));
  if (ui.coachMode !== mode || data.feedback !== fb) return;
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
  if (ui.report.compareWith !== null) await loadCompare(ui.report.compareWith);
}

/** Compares the other run (first) with the shown one (second): changes read as shown − other. */
async function loadCompare(other: number): Promise<void> {
  const base = ui.report.runId;
  if (base === null) return;
  await load((a) => a.assessmentCompare(other, base), (l) => (data.report.compare = l));
}

const ankiFields = (): NoteFields | null => ui.anki.fields ?? data.anki.draft?.fields ?? null;

async function loadPreview(): Promise<void> {
  const d = data.anki.draft;
  const f = ankiFields();
  if (!d || !f) return;
  const r = await call((a) => a.previewNote(d.id, f));
  if (data.anki.draft !== d) return;
  data.anki.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  refreshPreview();
}

async function loadAnki(): Promise<void> {
  if (data.services.status === 'idle') void loadServices();
  await load((a) => a.notes(), (l) => (data.anki.notes = l), valueOf(data.anki.notes));
  if (data.anki.draft && data.anki.preview.status !== 'ready') await loadPreview();
}

async function loadSettings(): Promise<void> {
  const s = data.settings;
  await Promise.all([
    loadModels(),
    loadServices(),
    load((a) => a.budget(), (l) => (s.budget = l), valueOf(s.budget)),
    load((a) => a.consents(), (l) => (s.consents = l), valueOf(s.consents)),
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

export function onRoute(loc: Location): void {
  ui.route = loc.route;
  ui.sub = loc.sub;
  const idx = SESSION_IDX[loc.route];
  if (idx !== undefined) ui.sessionAt = idx;
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
  p.innerHTML = previewHTML(data.anki.preview, { ...ui.anki, busy: ui.busy, theme: ui.theme }, data.anki.draft).value;
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
  data.coach = { preview: idle(), replies: {}, failure: null, sending: false };
}

async function answered(fbPromise: (a: StudyApi, token: string) => Promise<Feedback>): Promise<void> {
  const item = practiceItem();
  if (!item) return;
  const fb = await write((a) => fbPromise(a, item.token));
  if (!fb) {
    // A closed or changed item cannot be answered: show the core's current state instead.
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
    if (!item) return;
    ui.layer = null;
    renderLayer(false);
    void write((a) => a.openHint(item.token)).then((next) => {
      if (!next) {
        render(true, false);
        return;
      }
      data.practice = ready(next);
      document.body.dataset.room = next.can_count ? 'cool' : 'warm';
      const q = $('#qcol');
      if (q) {
        q.classList.add('enter-hint');
        q.innerHTML = qcolHTML(next, { sel: ui.practice.sel, marks: ui.practice.marks, busy: ui.busy }).value;
        tickClocks();
      }
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
    data.coach.failure = null;
    renderOrb();
    void call((a) => a.coachAsk(fb.attempt_id, mode)).then((r) => {
      data.coach.sending = false;
      if (data.feedback !== fb) return;
      if (r.ok) data.coach.replies[mode] = r.value;
      else data.coach.failure = r.error;
      renderOrb();
      void loadToday();
    });
  },
  'consent-grant'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.grantConsent(k[0], k[1])).then((ok) => ok && consentChanged());
  },
  'consent-revoke'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.revokeConsent(k[0], k[1])).then((ok) => ok && consentChanged());
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
    if (!run) return;
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
    openLayer('leave');
  },
  'leave-yes'() {
    const pos = assessState()?.item?.position;
    ui.layer = null;
    ui.assess.attached = false;
    go('today');
    toast(
      pos !== undefined
        ? T(`测评停在第 ${pos + 1} 题 · 计时继续`, `Run left at item ${pos + 1} · the clock keeps running`)
        : T('已离开测评 · 计时继续', 'Left the run · the clock keeps running'),
    );
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
      toast(
        res.failure
          ? `${T('测试未通过', 'Test failed')} · ${res.failure.message}${res.failure.action ? ' — ' + res.failure.action : ''}`
          : T(`${res.provider_name} · ${res.model} 已验证`, `${res.provider_name} · ${res.model} verified`),
      );
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
  if (!s?.run || !item) return;
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
  go: (r: Route) => go(r),
  todayStart: () => {
    const t = today();
    if (t?.active_run) go('assess');
    else ACT.start!(t?.recommendation ? 'recommended' : 'sample', document.body);
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
