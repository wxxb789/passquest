// Actions (prototype ACT table) and data loading. UI-only actions change `ui` and re-render; domain
// actions call the bound StudyApi and replace the cached data with whatever the core returns. The
// window never fabricates a result: without a bound service every call resolves to `not_connected`.
import { api, NOT_CONNECTED, toStudioError, type StudyApi } from './api/client';
import type { CoachMode, NoteFields, PracticeRequest, StudioError } from './api/types';
import { hideMarkPop, lightSpan, markCountText, takeMark, passageHTML, practiceFoot } from './components/passage';
import { labelForKey } from './components/options';
import { COACH_MODES, SESSION_IDX, T } from './core/i18n';
import { closeLayer, openLayer, render, renderLayer, renderOrb } from './core/render';
import { go, subAction, type Location } from './core/router';
import { data, idle, ui, valueOf, type Loadable } from './core/state';
import { savePref } from './core/prefs';
import { toast } from './core/toast';
import { activeRun, practiceItem, today } from './screens';
import { previewHTML, ankiStatus, nextRevision } from './views/anki';
import { optionSpan } from './views/feedback';
import { qcolHTML } from './views/practice';
import { tickClocks } from './core/clock';

type Result<T> = { ok: true; value: T } | { ok: false; error: StudioError };

const $ = <E extends Element = HTMLElement>(sel: string) => document.querySelector<E>(sel);
const reduced = () => ui.shot || matchMedia('(prefers-reduced-motion: reduce)').matches;
const ready = <T>(value: T): Loadable<T> => ({ status: 'ready', value });

async function call<T>(fn: (a: StudyApi) => Promise<T>): Promise<Result<T>> {
  const a = api();
  if (!a) return { ok: false, error: NOT_CONNECTED };
  try {
    return { ok: true, value: await fn(a) };
  } catch (e) {
    const error = toStudioError(e);
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
    toast(r.error.message);
    render(true, false);
    return undefined;
  }
  return r.value;
}

/** A write whose method returns nothing: resolves true on success. */
async function writeOk(fn: (a: StudyApi) => Promise<void>): Promise<boolean> {
  return (await write(async (a) => {
    await fn(a);
    return true as const;
  })) === true;
}

// ---------------------------------------------------------------- loaders

export const loadToday = () => load((a) => a.today(), (l) => (data.today = l), valueOf(data.today));

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

async function presentItem(runId: number, position?: number): Promise<void> {
  ui.assess.sel = null;
  await load((a) => a.presentAssessmentItem(runId, position), (l) => (data.assess.item = l));
}

export async function refreshAssess(): Promise<void> {
  const s = await load((a) => a.assessmentState(), (l) => (data.assess.state = l), valueOf(data.assess.state));
  if (!s) return;
  if (s.completed_run_id !== null && !s.active) {
    ui.report.runId = s.completed_run_id;
    data.report.report = idle();
    if (ui.route === 'assess') go('report');
    return;
  }
  if (s.active) {
    const item = valueOf(data.assess.item);
    if (!item || item.run_id !== s.active.run_id || item.position !== s.active.current_position) await presentItem(s.active.run_id);
    else render(false, false);
  } else {
    await load((a) => a.assessmentPool(), (l) => (data.assess.pool = l));
  }
}

async function loadReport(): Promise<void> {
  const runs = await load((a) => a.assessmentRuns(), (l) => (data.report.runs = l));
  if (!runs) return;
  const completed = runs.filter((r) => r.state === 'completed');
  const id = ui.report.runId ?? completed[0]?.run_id ?? null;
  if (id === null) return;
  ui.report.runId = id;
  const current = valueOf(data.report.report);
  if (!current || current.run.run_id !== id) await load((a) => a.assessmentReport(id), (l) => (data.report.report = l));
  if (ui.report.compareWith !== null) await loadCompare(ui.report.compareWith);
}

async function loadCompare(other: number): Promise<void> {
  const base = ui.report.runId;
  if (base === null) return;
  await load((a) => a.assessmentCompare(base, other), (l) => (data.report.compare = l));
}

const ankiFields = (): NoteFields | null => ui.anki.fields ?? data.anki.draft?.fields ?? null;

async function loadPreview(): Promise<void> {
  const d = data.anki.draft;
  const f = ankiFields();
  if (!d || !f) return;
  const r = await call((a) => a.previewNote(d.draft_id, f));
  if (data.anki.draft !== d) return;
  data.anki.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  refreshPreview();
}

async function loadAnki(): Promise<void> {
  await load((a) => a.notes(), (l) => (data.anki.notes = l), valueOf(data.anki.notes));
  if (data.anki.draft && data.anki.preview.status !== 'ready') await loadPreview();
}

async function loadSettings(): Promise<void> {
  const s = data.settings;
  await Promise.all([
    load((a) => a.models(), (l) => (s.models = l), valueOf(s.models)),
    load((a) => a.budget(), (l) => (s.budget = l), valueOf(s.budget)),
    load((a) => a.consents(), (l) => (s.consents = l), valueOf(s.consents)),
    load((a) => a.services(), (l) => (s.services = l), valueOf(s.services)),
  ]);
  if (ui.layer?.startsWith('sheet:')) renderLayer(false);
}

export async function loadRoute(force = false): Promise<void> {
  if (force || data.today.status === 'idle') void loadToday();
  switch (ui.route) {
    case 'today':
      if (!force && data.today.status !== 'idle') await loadToday();
      break;
    case 'practice':
      if (force) data.practice = idle();
      await loadPracticeResume();
      break;
    case 'feedback':
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
  if (!p) return;
  p.classList.toggle('phone', ui.anki.width === 'phone');
  p.innerHTML = previewHTML(data.anki.preview, { ...ui.anki, busy: ui.busy, theme: ui.theme }).value;
}

function refreshAnkiChrome(): void {
  const d = data.anki.draft;
  if (!d) return;
  const can = (d.status !== 'accepted' || ui.anki.dirty) && !ui.busy;
  for (const s of ['#astatus', '#estatus']) {
    const el = $(s);
    if (el) el.innerHTML = ankiStatus(d, ui.anki.dirty).value;
  }
  for (const s of ['#acceptbtn', '#acceptbtn2']) {
    const el = $<HTMLButtonElement>(s);
    if (!el) continue;
    el.disabled = !can;
    if (el.lastChild) el.lastChild.textContent = T(`接受 · 修订 ${nextRevision(d)}`, `Accept · revision ${nextRevision(d)}`);
  }
}

export function setEv(label: string): void {
  const fb = data.feedback;
  if (!fb || ui.feedback.ev === label) return;
  ui.feedback.ev = label;
  document.querySelectorAll<HTMLElement>('.fo').forEach((f) => f.classList.toggle('on', f.dataset.ev === label));
  lightSpan(fb.passage.paragraphs, optionSpan(fb, label), label === fb.key.label, !reduced());
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
  if (v === 'sample') return { kind: 'sample' };
  const r = today()?.recommendation;
  if (v === 'recommended' && r) {
    const req: PracticeRequest = { kind: r.kind };
    if (r.skill_id) req.skill_id = r.skill_id;
    if (r.source_id) req.source_id = r.source_id;
    if (r.question_id) req.question_id = r.question_id;
    return req;
  }
  return { kind: 'next' };
}

async function startPractice(req: PracticeRequest): Promise<void> {
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

async function answered(fbPromise: (a: StudyApi, token: string) => Promise<import('./api/types').Feedback>): Promise<void> {
  const item = practiceItem();
  if (!item) return;
  const fb = await write((a) => fbPromise(a, item.token));
  if (!fb) return;
  data.feedback = fb;
  data.practice = ready(null);
  ui.feedback.ev = null;
  ui.coachOpen = true;
  resetCoach();
  void loadToday();
  go('feedback');
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
    const settings = { exam_date: date || null, daily_minutes: mins ? Number(mins) : null };
    if (settings.daily_minutes !== null && !(Number.isInteger(settings.daily_minutes) && settings.daily_minutes > 0)) {
      toast(T('每日时长需为正整数分钟', 'Daily time must be a positive whole number of minutes'));
      return;
    }
    void writeOk((a) => a.setLearnerSettings(settings)).then((ok) => {
      if (!ok) return;
      closeLayer();
      void loadToday();
    });
  },
  start(v) {
    void startPractice(requestFor(v));
  },
  'start-skill'(v) {
    if (v) void startPractice({ kind: 'skill', skill_id: v });
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
      if (!next) return;
      data.practice = ready(next);
      document.body.dataset.room = next.condition === 'independent' && next.can_count ? 'cool' : 'warm';
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
    void write((a) => a.proposeNote(mark ? { attempt_id: fb.attempt_id, mark } : { attempt_id: fb.attempt_id })).then((d) => {
      if (!d) return;
      data.anki.draft = d;
      data.anki.preview = idle();
      ui.anki.fields = null;
      ui.anki.dirty = false;
      go('anki');
    });
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
      data.coach.preview = idle();
      renderOrb();
      void loadCoachPreview();
      void loadToday();
    });
  },
  'consent-ask'(v) {
    const [source_id, provider] = (v ?? '').split('|');
    if (!source_id || !provider) return;
    ui.settings.confirming = { source_id, provider };
    if (ui.route !== 'settings') go('settings', 'consent');
    else renderLayer(false);
  },
  'consent-cancel'() {
    ui.settings.confirming = null;
    renderLayer(false);
  },
  'consent-yes'() {
    const k = ui.settings.confirming;
    if (!k) return;
    void writeOk((a) => a.grantConsent(k.source_id, k.provider)).then((ok) => {
      if (!ok) return;
      ui.settings.confirming = null;
      data.coach.preview = idle();
      void loadSettings();
    });
  },
  'consent-revoke'(v) {
    const [source_id, provider] = (v ?? '').split('|');
    if (!source_id || !provider) return;
    void writeOk((a) => a.revokeConsent(source_id, provider)).then((ok) => {
      if (!ok) return;
      data.coach.preview = idle();
      void loadSettings();
    });
  },

  // assessment
  'run-ask'() {
    openLayer('start-run');
  },
  'run-start'() {
    const pool = valueOf(data.assess.pool);
    if (!pool) return;
    closeLayer();
    void write((a) => a.startAssessment(pool.default_size, pool.default_minutes)).then((s) => {
      if (!s) return;
      data.assess.state = ready(s);
      if (s.active) void presentItem(s.active.run_id);
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
    const run = activeRun();
    const item = valueOf(data.assess.item);
    const sel = ui.assess.sel;
    if (!run || !item || !sel) return;
    void write((a) => a.answerAssessmentItem(run.run_id, item.position, sel)).then((s) => {
      if (!s) return;
      data.assess.state = ready(s);
      ui.assess.sel = null;
      if (!s.active) {
        if (s.completed_run_id !== null) {
          ui.report.runId = s.completed_run_id;
          data.report.report = idle();
          go('report');
        } else render(true, false);
        return;
      }
      void presentItem(s.active.run_id);
    });
  },
  goto(v) {
    const run = activeRun();
    if (run && v) void presentItem(run.run_id, Number(v));
  },
  leave() {
    openLayer('leave');
  },
  'leave-yes'() {
    const n = activeRun()?.current_position;
    ui.layer = null;
    go('today');
    toast(n ? T(`测评停在第 ${n} 题 · 计时继续`, `Run left at item ${n} · the clock keeps running`) : T('已离开测评 · 计时继续', 'Left the run · the clock keeps running'));
  },

  // report
  cmp(v) {
    const id = Number(v);
    if (!Number.isFinite(id)) return;
    ui.report.compareWith = id;
    void loadCompare(id);
  },
  ann(v) {
    ui.report.annItem = Number(v);
    ui.layer = 'sheet:ann';
    renderLayer(false);
    $(`#layer [data-act="ann"][data-v="${v}"]`)?.focus({ preventScroll: true });
  },

  // anki
  side(v) {
    if (v !== 'both' && v !== 'front' && v !== 'back') return;
    ui.anki.side = v;
    refreshPreview();
    syncPressed('side', v);
  },
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
    if (!d || !f || (d.status === 'accepted' && !ui.anki.dirty)) return;
    void write((a) => a.acceptNote(d.draft_id, f)).then((next) => {
      if (!next) return;
      data.anki.draft = next;
      ui.anki.fields = null;
      ui.anki.dirty = false;
      toast(T(`已接受修订 ${next.revision ?? ''} · 已加入推送队列`, `Accepted revision ${next.revision ?? ''} · queued for push`));
      void loadAnki();
    });
  },
  askip() {
    const d = data.anki.draft;
    if (!d) return;
    void write((a) => a.skipNote(d.draft_id)).then((next) => {
      if (!next) return;
      data.anki.draft = next;
      ui.anki.fields = null;
      ui.anki.dirty = false;
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
    const p = valueOf(data.settings.models)?.providers.find((x) => x.id === v);
    if (!p) return;
    void write((a) => a.useModel(p.id, p.endpoint)).then((reg) => {
      if (!reg) return;
      data.settings.models = ready(reg);
      render();
      void loadToday();
    });
  },
  'model-test'(v) {
    const p = valueOf(data.settings.models)?.providers.find((x) => x.id === v);
    if (!p) return;
    void write((a) => a.testModel(p.id, p.endpoint)).then((res) => {
      if (!res) return;
      toast(res.failure ? res.failure.message : T(`${p.name} 已验证`, `${p.name} verified`));
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
      void loadSettings();
    });
  },
  'cred-clear'(v) {
    if (!v) return;
    void writeOk((a) => a.clearCredential(v)).then((ok) => {
      if (ok) void loadSettings();
    });
  },
};

async function push(force: boolean): Promise<void> {
  const res = await write((a) => a.pushNotes(force));
  if (!res) return;
  const prev = valueOf(data.anki.notes);
  if (prev) data.anki.notes = ready({ anki: { ...prev.anki, reachable: res.unreachable ? false : true }, counts: res.counts, notes: res.notes });
  if (res.unreachable) toast(res.unreachable.message);
  else toast(T(`已推送 ${res.counts.pushed} · 排队 ${res.counts.queued}`, `pushed ${res.counts.pushed} · queued ${res.counts.queued}`));
  render();
  if (ui.layer?.startsWith('sheet:')) renderLayer(false);
}

// ---------------------------------------------------------------- keyboard bindings

export const keyActions = {
  closeLayer,
  exitAnkiEdit: () => ACT.edit!(undefined, document.body),
  hideMarkPop,
  keys: () => ACT.keys!(undefined, document.body),
  theme: () => ACT.theme!(undefined, document.body),
  lang: () => ACT.lang!(undefined, document.body),
  go: (r: import('./core/state').Route) => go(r),
  todayStart: () => {
    const t = today();
    if (t?.active_run) go('assess');
    else ACT.start!(t?.recommendation ? 'recommended' : 'sample', document.body);
  },
  practicePick: (key: string) => {
    const item = practiceItem();
    const label = item ? labelForKey(item.question.options, key) : null;
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
    const item = valueOf(data.assess.item);
    const label = item ? labelForKey(item.options, key) : null;
    if (label) ACT.apick!(label, document.body);
    return label !== null;
  },
  assessConfirm: () => ACT.anext!(undefined, document.body),
  assessMove: (step: 1 | -1) => {
    const run = activeRun();
    if (!run || run.current_position === null) return;
    const open = (p: number) => run.cells.some((c) => c.position === p && c.state !== 'timed_out' && c.state !== 'unavailable');
    let p = run.current_position + step;
    while (p >= 1 && p <= run.size && !open(p)) p += step;
    if (p >= 1 && p <= run.size) void presentItem(run.run_id, p);
  },
  ankiFlip: () => ACT.side!(ui.anki.side === 'back' ? 'front' : 'back', document.body),
  ankiEdit: () => ACT.edit!(undefined, document.body),
};

/** Called by the display clock when a server deadline passes: ask the core what happened. */
export function onDeadline(): void {
  if (ui.route === 'assess' || activeRun()) void refreshAssess();
}

