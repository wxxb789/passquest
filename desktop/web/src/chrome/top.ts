// Top bar (prototype topHTML): brand + prep day, the middle slot (run clock in the assessment,
// session walk on study routes, resume link elsewhere) and the language/theme/shortcut utilities.
import type { AssessmentRun, Today } from '../api/types';
import { html, type Html } from '../core/html';
import { SESSION, SESSION_IDX, shortDate, T } from '../core/i18n';
import type { Lang, Route, Theme } from '../core/state';
import { ic } from '../icons';
import { runClockHTML } from '../views/assess';

export interface TopInput {
  route: Route;
  lang: Lang;
  theme: Theme;
  sessionAt: number;
  today: Today | null;
  run: AssessmentRun | null;
}

function brandDay(t: Today | null): Html {
  if (!t) return html``;
  if (t.prep.day <= 0) return html`<span class="brand-day">${T('还没有作答记录', 'No attempts yet')}</span>`;
  return html`<span class="brand-day" data-testid="prep-day">${T('备考第 ', 'Prep day ')}<b class="num">${t.prep.day}</b>${T(' 天 · 始于 ', ' · since ')}${shortDate(t.prep.first_attempt_at)}</span>`;
}

export function topHTML(i: TopInput): Html {
  const brand = html`<div class="brand">
      <span class="brand-mark" aria-hidden="true"></span>
      <span class="brand-name">Pass Quest</span>
      ${brandDay(i.today)}
    </div>`;
  let mid: Html = html`<span></span>`;
  const now = SESSION_IDX[i.route];
  const total = SESSION.length;
  if (i.route === 'assess' && i.run) {
    mid = runClockHTML(i.run);
  } else if (now !== undefined) {
    const cur = SESSION[now]!;
    mid = html`<div class="tl" role="group" aria-label="${T(`今日会话 · 第 ${now + 1} / ${total} 步：${cur[0]}`, `Today’s session · step ${now + 1} of ${total}: ${cur[1]}`)}">
      <div class="tl-dots" aria-hidden="true">${SESSION.map(
        (s, idx) => html`<span class="tl-dot ${idx < now ? 'done' : ''} ${idx === now ? 'now' : ''}"><span class="tip">${idx + 1} · ${T(s[0], s[1])}</span></span>`,
      )}</div>
      <span class="tl-label">${T(cur[0], cur[1])}<span class="n num">${now + 1} / ${total}</span></span>
    </div>`;
  } else if (i.sessionAt > 0) {
    const s = SESSION[i.sessionAt]!;
    mid = html`<a class="resume" href="#/${s[2]}"><span class="d"></span>${T(`回到今日会话 · ${s[0]} ${i.sessionAt + 1}/${total}`, `Resume session · ${s[1]} ${i.sessionAt + 1}/${total}`)}${ic('arrow', 'xs')}</a>`;
  }
  const util = html`<div class="util">
      <div class="seg" role="group" aria-label="${T('界面语言', 'Interface language')}">
        <button data-act="lang" data-v="zh" aria-pressed="${i.lang === 'zh'}" lang="zh-CN">中</button>
        <button data-act="lang" data-v="en" aria-pressed="${i.lang === 'en'}" lang="en">EN</button>
      </div>
      <button class="icon-btn" data-act="theme" id="themebtn" aria-label="${i.theme === 'light' ? T('切换到深色主题', 'Switch to dark theme') : T('切换到浅色主题', 'Switch to light theme')}" title="${T('主题', 'Theme')} · Shift+T">${ic(
        i.theme === 'light' ? 'moon' : 'sun',
      )}</button>
      <button class="icon-btn" data-act="keys" aria-label="${T('快捷键', 'Keyboard shortcuts')}" title="${T('快捷键', 'Shortcuts')} · ?">${ic('help')}</button>
    </div>`;
  return html`${brand}${mid}${util}`;
}
