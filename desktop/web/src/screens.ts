// Composition root of the views: turns UI state + the cached service data into the stage, chrome
// and layer markup that core/render.ts writes into the DOM.
import type { ActiveRun, Today } from './api/types';
import { coachHTML } from './views/coach';
import { credentialDialog, hintGate, keysDialog, learnerDialog, leaveDialog, startRunDialog } from './components/dialog';
import { centerEvidence } from './components/passage';
import { sheetWrap } from './components/sheet';
import { dockHTML } from './chrome/dock';
import { orbHTML, popHTML } from './chrome/orb';
import { topHTML } from './chrome/top';
import { html, type Html } from './core/html';
import { CONDITION, T, TITLES, TP } from './core/i18n';
import type { Screens } from './core/render';
import { data, ui, valueOf } from './core/state';
import { vAnki, ankiSheet } from './views/anki';
import { vAssess } from './views/assess';
import { vFeedback } from './views/feedback';
import { vPractice } from './views/practice';
import { reportPopLines, reportSheet, vReport } from './views/report';
import { settingsSheet, vSettings } from './views/settings';
import { errorCard, loadingCard, retryButton, stateCard, withLoadable } from './views/states';
import { todayPopLines, todaySheet, vToday } from './views/today';
import { ic } from './icons';

export const today = (): Today | null => valueOf(data.today) ?? null;
export const activeRun = (): ActiveRun | null => valueOf(data.assess.state)?.active ?? null;
export const practiceItem = () => valueOf(data.practice) ?? null;

function ankiConflicts(): number {
  const n = valueOf(data.anki.notes);
  return n ? n.counts.conflict : (today()?.anki.conflict ?? 0);
}

function stage(): Html {
  const retry = retryButton('reload');
  switch (ui.route) {
    case 'today':
      return withLoadable(data.today, vToday, retry);
    case 'practice':
      return withLoadable(data.practice, (item) => vPractice(item, { sel: ui.practice.sel, marks: ui.practice.marks, busy: ui.busy }), retry);
    case 'feedback':
      return data.feedback
        ? vFeedback(data.feedback, ui.feedback.ev)
        : stateCard({
            ico: 'practice',
            eyebrow: T('反馈', 'Feedback'),
            title: T('还没有可显示的反馈', 'No feedback to show yet'),
            body: T('先回答一道题；评分来自已核验答案，不来自模型。', 'Answer an item first; the grade comes from the verified key, never from a model.'),
            action: html`<a class="btn primary" href="#/practice">${T('去练习', 'Go to practice')}${ic('arrow', 'xs')}</a>`,
          });
    case 'assess':
      return withLoadable(data.assess.state, (s) => vAssess(s.active, data.assess.item, data.assess.pool, { sel: ui.assess.sel, busy: ui.busy }), retry);
    case 'report': {
      const runs = data.report.runs;
      if (runs.status === 'error') return errorCard(runs.error, retry);
      if (runs.status !== 'ready') return loadingCard();
      const completed = runs.value.filter((r) => r.state === 'completed');
      if (completed.length === 0) return vReport(null, runs.value, null, data.report.compare);
      return withLoadable(data.report.report, (r) => vReport(r, runs.value, ui.report.compareWith, data.report.compare), retry);
    }
    case 'anki':
      return vAnki(data.anki.draft, ui.anki.fields, data.anki.preview, data.anki.notes, {
        side: ui.anki.side,
        card: ui.anki.card,
        width: ui.anki.width,
        editing: ui.anki.editing,
        dirty: ui.anki.dirty,
        busy: ui.busy,
        theme: ui.theme,
      });
    case 'settings':
      return vSettings(data.settings, { open: ui.settings.open, confirming: ui.settings.confirming, busy: ui.busy });
  }
}

function coachSheet(): Html | null {
  if (!data.feedback) return null;
  return coachHTML(
    {
      preview: data.coach.preview,
      replies: data.coach.replies,
      failure: data.coach.failure,
      sending: data.coach.sending,
      unusable: today()?.providers_unusable ?? null,
    },
    ui.coachMode,
    ui.lang,
  );
}

function pop(): Html {
  const t = today();
  if (ui.route === 'report') {
    const r = valueOf(data.report.report);
    return popHTML(r ? reportPopLines(r, data.report.compare) : [T('及格线未知：这里只有观察到的结果。', 'The pass threshold is unknown: this is observation only.')], '');
  }
  const m = t?.model;
  const foot = m
    ? html`<p class="fine" style="margin-top:10px;padding-top:10px;border-top:1px solid var(--line)">${T(`模型 ${m.model} · 今日已用`, `Model ${m.model} · used today`)} <span class="num">${m.used_today} / ${m.limit}</span></p>`
    : '';
  return popHTML(t ? todayPopLines(t) : [], foot);
}

function sheet(key: string): Html {
  let out: Html | null = null;
  const t = today();
  if (ui.route === 'today' && t) out = todaySheet(key, t);
  else if (ui.route === 'report') {
    const r = valueOf(data.report.report);
    if (r) out = reportSheet(key, r, ui.report.annItem);
  } else if (ui.route === 'anki') out = ankiSheet(key, data.anki.notes, ui.busy);
  else if (ui.route === 'settings') out = settingsSheet(key, data.settings, { open: ui.settings.open, confirming: ui.settings.confirming, busy: ui.busy });
  return out ?? sheetWrap('info', T('正在读取……', 'Loading…'), '', html`<p class="fine">${T('数据还没有就绪。', 'The data is not ready yet.')}</p>`);
}

function layer(): Html {
  const L = ui.layer;
  if (!L) return html``;
  if (L.startsWith('sheet:')) return sheet(L.slice(6));
  switch (L) {
    case 'keys':
      return keysDialog();
    case 'hint':
      return hintGate(practiceItem()?.can_count ?? false);
    case 'leave':
      return leaveDialog(activeRun()?.current_position ?? null);
    case 'pop':
      return pop();
    case 'learner':
      return learnerDialog(today()?.learner ?? { exam_date: null, daily_minutes: null });
    case 'credential': {
      const reg = valueOf(data.settings.models);
      const p = reg?.providers.find((x) => x.id === ui.settings.credentialFor);
      return credentialDialog(p?.name ?? ui.settings.credentialFor ?? '', reg?.credential_store.available ?? false, reg?.credential_store.reason ?? null);
    }
    case 'start-run': {
      const pool = valueOf(data.assess.pool);
      return pool ? startRunDialog(pool) : html``;
    }
  }
  return html``;
}

function room(): string {
  if (ui.route === 'assess') return 'still';
  if (ui.route === 'practice') {
    const item = practiceItem();
    if (!item) return 'calm';
    return item.condition === 'independent' && item.can_count ? 'cool' : CONDITION[item.condition].room;
  }
  if (ui.route === 'feedback' && data.feedback) return data.feedback.counts ? 'cool' : 'warm';
  return 'calm';
}

export const screens: Screens = {
  top: () => topHTML({ route: ui.route, lang: ui.lang, theme: ui.theme, sessionAt: ui.sessionAt, today: today(), run: activeRun() }),
  stage,
  dock: () => dockHTML(ui.route, activeRun(), ankiConflicts()),
  orb: () => orbHTML({ route: ui.route, coachOpen: ui.coachOpen, popOpen: ui.layer === 'pop', model: today()?.model ?? null, coachSheet: coachSheet() }),
  layer,
  room,
  title: () => 'Pass Quest — ' + TP(TITLES[ui.route]),
  withCoach: () => ui.route === 'feedback' && ui.coachOpen && data.feedback !== null,
  editing: () => ui.route === 'anki' && ui.anki.editing,
  afterStage: (entered) => {
    if (ui.route === 'feedback' && entered) centerEvidence();
  },
};
