// Actions (prototype ACT table) and data loading. UI-only actions change `ui` and re-render; domain
// actions call the bound StudyApi and replace the cached data with whatever the core returns. The
// window never fabricates a result: without a bound service every call resolves to `not_connected`.
//
// This module is the registry and the routing glue: the window-wide actions, the ACT table composed
// from each room's controller (src/controllers/), route loading and route changes, boot recovery, and
// the keyboard bindings. Each room's state transitions live in its controller.
import { applyWindowButtonsTheme } from './chrome/titlebar';
import { labelForKey } from './components/options';
import { hideMarkPop } from './components/passage';
import { COACH_MODES, T } from './core/i18n';
import { closeLayer, openLayer, render, renderLayer, renderOrb } from './core/render';
import { go, subAction, type Location } from './core/router';
import { data, idle, ui, type Route } from './core/state';
import { savePref } from './core/prefs';
import { toast } from './core/toast';
import { activeRun, assessState, practiceItem } from './screens';
import { opensItem } from './views/today';
import { ankiActs, loadAnki, refreshPreview, seedRefine } from './controllers/anki';
import { assessActs, detachRun, refreshAssess } from './controllers/assess';
import { consentActs } from './controllers/consent';
import { $, reduced, syncPressed, type Act } from './controllers/dom';
import { coachActs, loadCoachPreview, setEv } from './controllers/feedback';
import { doMark, loadPracticeResume, practiceActs } from './controllers/practice';
import { loadReport, reportActs } from './controllers/report';
import { call, loadToday, ready } from './controllers/service';
import { loadModels, loadServices, loadSettings, settingsActs } from './controllers/settings';
import { todayActs } from './controllers/today';

export { doMark } from './controllers/practice';
export { onFieldInput } from './controllers/anki';
export { onDeadline, refreshAssess } from './controllers/assess';
export { setEv } from './controllers/feedback';
export { loadToday } from './controllers/service';
export { watchBrief } from './controllers/today';

// ---------------------------------------------------------------- loading

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

// ---------------------------------------------------------------- routing

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

// ---------------------------------------------------------------- the action table

/** Window-wide actions: the chrome (theme, language, keys, layers, the coach orb) on any route. */
const shellActs: Record<string, Act> = {
  theme() {
    ui.theme = ui.theme === 'light' ? 'dark' : 'light';
    savePref('theme', ui.theme);
    applyWindowButtonsTheme(ui.theme);
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
};

export const ACT: Record<string, Act> = {
  ...shellActs,
  ...todayActs,
  ...practiceActs,
  ...coachActs,
  ...consentActs,
  ...assessActs,
  ...reportActs,
  ...ankiActs,
  ...settingsActs,
};

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
