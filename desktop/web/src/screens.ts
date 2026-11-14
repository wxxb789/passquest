// Composition root of the views: turns UI state + the cached service data into the stage, chrome
// and layer markup that core/render.ts writes into the DOM.
import type { AssessmentRun, AssessmentState, Today } from './api/types';
import { coachHTML } from './views/coach';
import { abandonDialog, consentAllDialog, credentialDialog, hintGate, keysDialog, learnerDialog, leaveDialog, startRunDialog } from './components/dialog';
import { centerEvidence } from './components/passage';
import { sheetWrap } from './components/sheet';
import { dockHTML } from './chrome/dock';
import { orbHTML, popHTML } from './chrome/orb';
import { topHTML } from './chrome/top';
import { html, type Html } from './core/html';
import { conditionOf, T, TITLES, TP } from './core/i18n';
import type { Screens } from './core/render';
import { data, ui, valueOf } from './core/state';
import { vAnki, ankiSheet } from './views/anki';
import { vAssess } from './views/assess';
import { vFeedback } from './views/feedback';
import { vPractice } from './views/practice';
import { reportPopLines, reportSheet, vReport } from './views/report';
import { settingsSheet, vSettings, type SettingsData } from './views/settings';
import { errorCard, loadingCard, retryButton, stateCard, withLoadable } from './views/states';
import { todayPopLines, todaySheet, vToday } from './views/today';
import { ic } from './icons';

export const today = (): Today | null => valueOf(data.today) ?? null;
export const assessState = (): AssessmentState | null => valueOf(data.assess.state) ?? null;
/** The active run: from the assessment state when loaded, else from Today. */
export const activeRun = (): AssessmentRun | null => {
  const s = assessState();
  if (s) return s.run && s.state === 'item' ? s.run : null;
  return today()?.active_run ?? null;
};
export const practiceItem = () => valueOf(data.practice) ?? null;

function ankiConflicts(): number {
  const n = valueOf(data.anki.notes);
  return n ? n.counts.conflict : (today()?.anki.conflict ?? 0);
}

/** Private imported sources known locally: the pool's units and the consent ledger. */
function knownSources(): string[] {
  const ids = new Set<string>();
  for (const u of today()?.pool.content_units ?? []) if (u.source_id) ids.add(u.source_id);
  for (const c of valueOf(data.settings.consents) ?? []) ids.add(c.source_id);
  return [...ids].sort();
}

function settingsData(): SettingsData {
  return { models: data.models, budget: data.settings.budget, consents: data.settings.consents, services: data.services, sources: knownSources() };
}

function stage(): Html {
  const retry = retryButton('reload');
  switch (ui.route) {
    case 'today':
      return withLoadable(data.today, (t) => vToday(t, ui.busy), retry);
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
      return withLoadable(data.assess.state, (s) => vAssess(s, data.assess.pool, { sel: ui.assess.sel, busy: ui.busy, attached: ui.assess.attached }), retry);
    case 'report': {
      const runs = data.report.runs;
      if (runs.status === 'error') return errorCard(runs.error, retry);
      if (runs.status !== 'ready') return loadingCard();
      if (!runs.value.some((r) => r.status !== 'active')) return vReport(null, runs.value, null, data.report.compare);
      return withLoadable(data.report.report, (r) => vReport(r, runs.value, ui.report.compareWith, data.report.compare), retry);
    }
    case 'anki':
      return vAnki(data.anki.draft, ui.anki.fields, data.anki.preview, data.anki.notes, {
        card: ui.anki.card,
        width: ui.anki.width,
        editing: ui.anki.editing,
        dirty: ui.anki.dirty,
        busy: ui.busy,
        theme: ui.theme,
      });
    case 'settings':
      return vSettings(settingsData(), { open: ui.settings.open, busy: ui.busy });
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
      busy: ui.busy,
      models: valueOf(data.models) ?? null,
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
  const b = m?.budget.providers.find((p) => p.provider === m.provider);
  const foot =
    m?.configured && m.model
      ? html`<p class="fine" style="margin-top:10px;padding-top:10px;border-top:1px solid var(--line)">${T(`模型 ${m.model} · 今日已用`, `Model ${m.model} · used today`)} <span class="num">${b ? `${b.calls} / ${b.limit}` : '—'}</span></p>`
      : '';
  return popHTML(t ? todayPopLines(t) : [], foot);
}

function sheet(key: string): Html {
  let out: Html | null = null;
  const t = today();
  if ((ui.route === 'today' || ui.route === 'practice') && t) out = todaySheet(key, t, data.services);
  else if (ui.route === 'report') {
    const r = valueOf(data.report.report);
    if (r) out = reportSheet(key, r, ui.report.annItem);
  } else if (ui.route === 'anki') out = ankiSheet(key, data.anki.notes, valueOf(data.services)?.anki_endpoint ?? null, ui.busy);
  else if (ui.route === 'settings') out = settingsSheet(key, settingsData(), { open: ui.settings.open, busy: ui.busy });
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
    case 'leave': {
      const pos = assessState()?.item?.position;
      return leaveDialog(pos === undefined ? null : pos + 1);
    }
    case 'abandon-run': {
      const run = activeRun();
      return run ? abandonDialog(run.id) : html``;
    }
    case 'pop':
      return pop();
    case 'learner': {
      const t = today();
      return t ? learnerDialog(t.prep) : html``;
    }
    case 'credential': {
      const p = valueOf(data.models)?.providers.find((x) => x.id === ui.settings.credentialFor);
      const s = valueOf(data.services);
      return credentialDialog(p?.name ?? ui.settings.credentialFor ?? '', s ? s.credential_store : null, p?.credential_env ?? '');
    }
    case 'start-run': {
      const pool = valueOf(data.assess.pool);
      return pool ? startRunDialog(pool) : html``;
    }
    case 'consent-all': {
      const source = ui.anki.consentSource;
      const m = data.models;
      const names = m.status === 'ready' ? m.value.providers.map((p) => p.name) : m.status === 'error' ? 'unreadable' : 'loading';
      return source ? consentAllDialog(source, names) : html``;
    }
  }
  return html``;
}

function room(): string {
  if (ui.route === 'assess') return 'still';
  if (ui.route === 'practice') {
    const item = practiceItem();
    if (!item) return 'calm';
    return item.can_count ? 'cool' : conditionOf(item.condition).room;
  }
  if (ui.route === 'feedback' && data.feedback) return data.feedback.counted ? 'cool' : 'warm';
  return 'calm';
}

export const screens: Screens = {
  top: () => topHTML({ route: ui.route, lang: ui.lang, theme: ui.theme, sessionAt: ui.sessionAt, today: today(), run: ui.assess.attached ? activeRun() : null }),
  stage,
  dock: () => dockHTML(ui.route, assessState(), ui.assess.attached, ankiConflicts()),
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
