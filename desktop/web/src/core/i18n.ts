// Inline bilingual strings: T(zh, en), as in the prototype. Exam content stays English in both
// languages (rendered with lang="en"); only the chrome switches.
import type { AnkiState, BriefState, CapabilityState, CoachMode, Condition, ItemOutcome } from '../api/types';
import type { IconName } from '../icons';
import { ui, type Route } from './state';

export type Pair = readonly [zh: string, en: string];

export const T = (zh: string, en: string): string => (ui.lang === 'zh' ? zh : en);
export const TP = (pair: Pair): string => T(pair[0], pair[1]);

/** Known skill names; unknown ids fall back to the id itself (never a guessed name). */
export const SK: Record<string, Pair> = {
  'not-except-polarity': ['NOT/EXCEPT 极性', 'NOT/EXCEPT polarity'],
  'vocabulary-in-context': ['语境词汇', 'Vocabulary in context'],
  'sentence-structure': ['句子结构', 'Sentence structure'],
  unclassified: ['未分类推断', 'Unclassified inference'],
};
export const skillName = (id: string): string => (SK[id] ? TP(SK[id]) : id);

export const SKILL_KIND: Record<string, Pair> = {
  scoring: ['得分技能', 'Scoring skill'],
  language: ['语言技能', 'Language skill'],
  unclassified: ['未分类', 'Unclassified'],
};
export const skillKindName = (k: string): string => (SKILL_KIND[k] ? TP(SKILL_KIND[k]) : k);

export const ACTN: Record<string, Pair> = {
  practice_new: ['新练习', 'New practice'],
  review: ['复习', 'Review'],
};
export const actName = (id: string): string => (ACTN[id] ? TP(ACTN[id]) : id);

export const DOCK: readonly (readonly [Route & IconName, string, string])[] = [
  ['today', '今日', 'Today'],
  ['practice', '练习', 'Practice'],
  ['assess', '测评', 'Assess'],
  ['report', '报告', 'Report'],
  ['anki', 'Anki 笔记', 'Anki'],
  ['settings', '设置', 'Settings'],
];

/** The shape of a study session walk (UI flow, not data). */
export const SESSION: readonly (readonly [string, string, Route])[] = [
  ['今日', 'Today', 'today'],
  ['练习', 'Practice', 'practice'],
  ['反馈', 'Feedback', 'feedback'],
  ['笔记', 'Note', 'anki'],
  ['下一篇', 'Next passage', 'practice'],
];
export const SESSION_IDX: Partial<Record<Route, number>> = { today: 0, practice: 1, feedback: 2, anki: 3 };

export const TITLES: Record<Route, Pair> = {
  today: ['今日', 'Today'],
  practice: ['练习', 'Practice'],
  feedback: ['反馈', 'Feedback'],
  assess: ['测评', 'Assessment'],
  report: ['报告', 'Report'],
  anki: ['Anki 笔记', 'Anki'],
  settings: ['设置', 'Settings'],
};

export const RSTATE: Record<ItemOutcome, readonly [string, string, IconName, 'c' | 'w' | 't']> = {
  correct: ['正确', 'correct', 'check', 'c'],
  incorrect: ['不正确', 'incorrect', 'x', 'w'],
  timed_out: ['超时', 'timed out', 'tout', 't'],
  unanswered: ['未作答', 'unanswered', 'minus', 't'],
  excluded: ['已排除', 'excluded', 'shield', 't'],
};

export const CONDITION: Record<Condition, { title: Pair; can: Pair; cannot: Pair; room: 'cool' | 'warm' | 'still' }> = {
  independent: { title: ['独立', 'Independent'], can: ['本次可计入独立迁移', 'This attempt can count'], cannot: ['本次不计入', 'This attempt will not count'], room: 'cool' },
  assisted: { title: ['有辅助', 'Assisted'], can: ['本次可计入', 'This attempt can count'], cannot: ['本次不计入独立迁移', 'This attempt will not count'], room: 'warm' },
  re_exposure: { title: ['重复接触', 'Re-exposure'], can: ['本次可计入', 'This attempt can count'], cannot: ['文章已见过 · 本次不计入', 'Passage already seen · will not count'], room: 'warm' },
  revisit: { title: ['重做', 'Revisit'], can: ['本次可计入', 'This attempt can count'], cannot: ['重做 · 本次不计入', 'Revisit · will not count'], room: 'warm' },
  assessment: { title: ['测评', 'Assessment'], can: ['限时、无辅助', 'Timed, no help'], cannot: ['限时、无辅助', 'Timed, no help'], room: 'still' },
};

export const COACH_MODES: readonly (readonly [CoachMode, string, string, number])[] = [
  ['explain', '解释', 'Explain', 1],
  ['probe', '追问', 'Probe', 2],
  ['micro_lesson', '微课', 'Micro-lesson', 3],
];

export const CAPABILITY: Record<CapabilityState, readonly [string, string, IconName, 'v' | 'u' | 'f' | 'n']> = {
  verified: ['已验证', 'Verified', 'seal', 'v'],
  declared: ['已声明 · 未验证', 'Declared, not tested', 'adv', 'u'],
  failed: ['验证失败', 'Check failed', 'x', 'f'],
  unsupported: ['不支持', 'Unsupported', 'minus', 'n'],
};

export const ANKI_STATE: Record<AnkiState, readonly [string, string, IconName, '' | 'q' | 'c' | 'p']> = {
  local: ['本地草稿', 'local', 'edit', ''],
  queued: ['排队 · 可连接时推送一次', 'queued · pushes once when reachable', 'clock', 'q'],
  pushed: ['已推送', 'pushed', 'check', 'p'],
  already_present: ['已存在 · 未重复添加', 'already present · not added', 'present', ''],
  conflict: ['冲突', 'conflict', 'conflict', 'c'],
  failed: ['失败', 'failed', 'x', 'c'],
};

export const BRIEF_STATE: Record<BriefState, Pair> = {
  running: ['运行中', 'Running'],
  paused: ['已暂停', 'Paused'],
  completed: ['已完成', 'Completed'],
  failed: ['失败', 'Failed'],
  needs_action: ['需要处理', 'Needs action'],
  cancelling: ['取消中', 'Cancelling'],
  cancelled: ['已取消', 'Cancelled'],
};

/** Local date-time for a server timestamp: MM-DD HH:MM (the prototype's compact form). */
export function shortStamp(iso: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** Local date: YYYY-MM-DD. */
export function shortDate(iso: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Local date: MM-DD. */
export function monthDay(iso: string | null): string {
  return iso ? shortDate(iso).slice(5) : '—';
}

export const sec = (ms: number | null): string => (ms === null ? '—' : String(Math.round(ms / 1000)));
