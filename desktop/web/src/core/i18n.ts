// Inline bilingual strings: T(zh, en), as in the prototype. Exam content stays English in both
// languages (rendered with lang="en"); only the chrome switches. Core codes (states, actions,
// conditions, error codes, failure types, contamination reasons) are mapped here; a code without a
// mapping shows a generic localized label first, with the code itself only as a muted mono detail.
import type { CoachMode, SkillEvidence } from '../api/types';
import type { IconName } from '../icons';
import { html, type Html } from './html';
import { ui, type Route } from './state';

export type Pair = readonly [zh: string, en: string];

export const T = (zh: string, en: string): string => (ui.lang === 'zh' ? zh : en);
export const TP = (pair: Pair): string => T(pair[0], pair[1]);

/** Known skill names; an unknown id falls back to the core's title, then the id (never a guessed name). */
export const SK: Record<string, Pair> = {
  'not-except-polarity': ['NOT/EXCEPT 极性', 'NOT/EXCEPT polarity'],
  'vocabulary-in-context': ['语境词汇', 'Vocabulary in context'],
  'sentence-structure': ['句子结构', 'Sentence structure'],
  unclassified: ['未分类推断', 'Unclassified inference'],
};
export const skillName = (id: string, title?: string): string => (SK[id] ? TP(SK[id]) : title || id);

export const SKILL_KIND: Record<string, Pair> = {
  scoring: ['得分技能', 'Scoring skill'],
  language: ['语言技能', 'Language skill'],
  unclassified: ['未分类', 'Unclassified'],
};
export const skillKindName = (k: string): string => (SKILL_KIND[k] ? TP(SKILL_KIND[k]) : T('未分类', 'Unclassified'));

/** Policy actions (`adaptive.Action`). */
export const ACTN: Record<string, Pair> = {
  practice_new: ['新练习', 'New practice'],
  review: ['复习', 'Review'],
  explain: ['先看讲解', 'Explanation first'],
};
export const actName = (id: string): string => (ACTN[id] ? TP(ACTN[id]) : T('未知动作', 'unknown action'));

/** Policy skill states (`adaptive.State`). */
export const PSTATE: Record<string, Pair> = {
  no_evidence: ['没有证据', 'no evidence'],
  familiarity_only: ['只有熟悉度', 'familiarity only'],
  insufficient: ['证据不足', 'insufficient'],
  weak: ['薄弱', 'weak'],
  mixed: ['有对有错', 'mixed'],
  supported: ['有支持', 'supported'],
  no_material: ['没有材料', 'no material'],
};
export const policyStateName = (id: string): string => (PSTATE[id] ? TP(PSTATE[id]) : T('未知', 'unknown'));

/** Causal diagnosis (`adaptive.causalDiagnosis`): never a guessed cause. */
export function causalText(id: string): string {
  switch (id) {
    case 'unresolved':
      return T('错因 · 未定', 'Cause · unresolved');
    case 'no_error_observed':
      return T('还没有观察到错误', 'No error observed yet');
    case 'not_attempted':
      return T('还没有独立作答', 'No independent answer yet');
    default:
      return T('错因 · 未知', 'Cause · unknown');
  }
}
export const causalDetail = (id: string): string =>
  id === 'unresolved'
    ? T('答错的选项看不出是技能、词汇、时间还是读错题干。', 'A wrong option does not show whether the cause was the skill, vocabulary, time or a misread stem.')
    : '';

/**
 * The policy's reason in the window's language. The core writes the reason in English from the
 * skill's state, its action and the evidence counts; in Chinese the same facts are stated from
 * those structured fields (state, action, counts). English shows the core's own sentence.
 */
export function policyReason(reason: string, s: SkillEvidence | null): string {
  if (ui.lang === 'en' || !s) return reason;
  const e = s.evidence;
  const i = s.inference;
  const answered = e.independent_correct + e.independent_incorrect;
  const skipped = e.independent_skipped ? `、跳过 ${e.independent_skipped}` : '';
  let first: string;
  switch (i.state) {
    case 'no_material':
      return '当前材料里没有标注这个技能的可练习题。导入并审核一份带标注的来源后再练。';
    case 'no_evidence':
      first = '这个技能还没有作答记录。';
      break;
    case 'familiarity_only':
      first = `已有 ${e.attempts} 次作答，但没有一次是独立迁移（有辅助 ${e.assisted_attempts}、重做 ${e.revisits}、重复接触 ${e.passage_repeats}）。`;
      break;
    case 'insufficient':
      first = `只有 ${answered} 次已作答的独立迁移（答对 ${e.independent_correct}、答错 ${e.independent_incorrect}${skipped}），一次观察分不清是技能还是运气。`;
      break;
    case 'weak':
      first = `已作答的 ${answered} 次独立迁移全部答错。`;
      break;
    case 'mixed':
      first = `${answered} 次已作答的独立迁移中答对 ${e.independent_correct} 次，最近一次独立作答${e.latest_independent_status === 'incorrect' ? '答错' : '答对'}。`;
      break;
    case 'supported':
      first = `已作答的 ${answered} 次独立迁移全部答对。`;
      break;
    default:
      return reason;
  }
  const slow = e.latest_independent_ms > 0 && e.fastest_independent_ms > 0 && e.latest_independent_ms >= 2 * e.fastest_independent_ms;
  switch (i.action) {
    case 'practice_new':
      return first + '选一篇新文章，不给提示，可以计入独立迁移。';
    case 'review':
      return (
        first +
        (i.state === 'supported' && slow
          ? `最近一次答对用了 ${Math.round(e.latest_independent_ms / 1000)} 秒，至少是最快 ${Math.round(e.fastest_independent_ms / 1000)} 秒的两倍，选一道见过的题复习；重做不算独立迁移。`
          : '没有新文章了，选一道见过的题复习；重做不算独立迁移。')
      );
    case 'explain':
      if (s.skill.kind === 'scoring' && e.guided_attempts === 0) return first + '得分技能先做一次引导讲解（那一次记为有辅助），再做独立练习。';
      if (i.state === 'weak') return first + '先看讲解：同样的缺口重复出现，而错选项本身看不出原因。';
      if (i.state === 'mixed' && e.latest_independent_status === 'incorrect') return first + '在下一道新题之前先看讲解。';
      return first + '没有新文章或可复习的题了，因此先看讲解。';
    default:
      return reason;
  }
}

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

/** Report item outcomes (the window's reading of `assess report` items; see views/report.ts). */
export type ItemOutcome = 'correct' | 'incorrect' | 'skipped' | 'timed_out' | 'excluded' | 'unavailable' | 'pending';
export const RSTATE: Record<ItemOutcome, readonly [string, string, IconName, 'c' | 'w' | 't']> = {
  correct: ['正确', 'correct', 'check', 'c'],
  incorrect: ['不正确', 'incorrect', 'x', 'w'],
  skipped: ['跳过', 'skipped', 'minus', 't'],
  timed_out: ['超时', 'timed out', 'tout', 't'],
  excluded: ['已排除', 'excluded', 'shield', 't'],
  unavailable: ['不可用', 'unavailable', 'minus', 't'],
  pending: ['未作答', 'not answered', 'minus', 't'],
};

/** Attempt conditions (`studio.Condition*`). `cannot` states why the attempt will not count. */
export const CONDITION: Record<string, { title: Pair; can: Pair; cannot: Pair; room: 'cool' | 'warm' }> = {
  independent: { title: ['独立', 'Independent'], can: ['本次可计入独立迁移', 'This attempt can count'], cannot: ['本次不计入', 'This attempt will not count'], room: 'cool' },
  assisted: { title: ['有辅助', 'Assisted'], can: ['本次可计入', 'This attempt can count'], cannot: ['提示可见 · 本次不计入独立迁移', 'Help is visible · will not count'], room: 'warm' },
  re_exposure: { title: ['重复接触', 'Re-exposure'], can: ['本次可计入', 'This attempt can count'], cannot: ['文章已见过 · 本次不计入', 'Passage already seen · will not count'], room: 'warm' },
  revisit: { title: ['重做', 'Revisit'], can: ['本次可计入', 'This attempt can count'], cannot: ['这道题答过 · 本次不计入', 'Answered before · will not count'], room: 'warm' },
};
const UNKNOWN_CONDITION = { title: ['条件未知', 'Unknown condition'] as Pair, can: ['', ''] as Pair, cannot: ['本次不计入', 'Will not count'] as Pair, room: 'warm' as const };
export const conditionOf = (c: string) => CONDITION[c] ?? UNKNOWN_CONDITION;

export const COACH_MODES: readonly (readonly [CoachMode, string, string, number])[] = [
  ['explain', '解释', 'Explain', 1],
  ['probe', '追问', 'Probe', 2],
  ['micro_lesson', '微课', 'Micro-lesson', 3],
];

export const CAPABILITY: Record<string, readonly [string, string, IconName, 'v' | 'u' | 'f' | 'n']> = {
  verified: ['已验证', 'Verified', 'seal', 'v'],
  declared: ['已声明 · 未验证', 'Declared, not tested', 'adv', 'u'],
  failed: ['验证失败', 'Check failed', 'x', 'f'],
  unsupported: ['不支持', 'Unsupported', 'minus', 'n'],
  legacy: ['旧配置 · 未验证', 'Legacy · not tested', 'adv', 'u'],
};
export const capabilityOf = (state: string) => CAPABILITY[state] ?? (['未知', 'Unknown', 'minus', 'n'] as const);

export const ANKI_STATE: Record<string, readonly [string, string, IconName, '' | 'q' | 'c' | 'p']> = {
  local: ['本地 · 未推送', 'local · not pushed', 'edit', ''],
  queued: ['排队 · 可连接时推送一次', 'queued · pushes once when reachable', 'clock', 'q'],
  pushed: ['已推送', 'pushed', 'check', 'p'],
  already_present: ['已存在 · 未重复添加', 'already present · not added', 'present', ''],
  conflict: ['冲突', 'conflict', 'conflict', 'c'],
  failed: ['失败', 'failed', 'x', 'c'],
  deck_mismatch: ['牌组不符', 'wrong deck', 'conflict', 'c'],
};
export const ankiStateOf = (state: string) => ANKI_STATE[state] ?? (['未知', 'unknown', 'info', ''] as const);

export const BRIEF_STATE: Record<string, Pair> = {
  running: ['运行中', 'Running'],
  paused: ['已暂停', 'Paused'],
  completed: ['已完成', 'Completed'],
  failed: ['失败', 'Failed'],
  needs_action: ['需要处理', 'Needs action'],
  cancelling: ['取消中', 'Cancelling'],
  cancelled: ['已取消', 'Cancelled'],
};
export const briefStateName = (s: string): string => (BRIEF_STATE[s] ? TP(BRIEF_STATE[s]) : T('未知', 'unknown'));

/** Source label kinds (`studio.Source*`). */
export function sourceKindName(kind: string): string {
  switch (kind) {
    case 'imported':
      return T('本地导入', 'Local import');
    case 'sample':
      return T('内置示例', 'Built-in sample');
    case 'first_party':
      return T('自有内容', 'First-party content');
    default:
      return T('来源未知', 'Unknown source');
  }
}

/**
 * A code the window has no wording for: a generic localized label first, then the code itself in a
 * muted mono span (secondary, for a bug report), never as the primary text.
 */
function coded(map: Record<string, Pair>, code: string, generic: Pair): Html {
  const known = map[code];
  return known ? html`${TP(known)}` : html`${TP(generic)} <span class="id" data-unmapped-code="${code}">${code}</span>`;
}

/** Plain-text form of `coded`, for toasts and popover lines (no markup there). */
function codedText(map: Record<string, Pair>, code: string, generic: Pair): string {
  const known = map[code];
  return known ? TP(known) : `${TP(generic)} (${code})`;
}

/**
 * Titles for the typed errors a learner can meet (studio errors, refusals, coach and push failures);
 * the core's message follows as the detail.
 */
const ERRORS: Record<string, Pair> = {
  assessment_active: ['测评进行中', 'An assessment is in progress'],
  pool_floor: ['未见测评池不足', 'The unseen pool is too small'],
  pool_short: ['未见测评池不足', 'The unseen pool is too small'],
  run_active: ['已有一轮测评在进行', 'A run is already active'],
  run_completed: ['这轮测评已结束', 'The run is finished'],
  no_credential: ['没有密钥', 'No credential'],
  missing_credential: ['没有密钥', 'No credential'],
  consent_required: ['需要先授权', 'Consent is needed first'],
  consent_scope_differs: ['授权范围不同', 'The consent scope differs'],
  node_unavailable: ['本地 Node 不可用', 'Node is not available'],
  agent_unavailable: ['本地 Agent 宿主不可用', 'The local agent host is not available'],
  budget_exhausted: ['今日预算已用完', "Today's budget is spent"],
  budget_exceeded: ['今日预算已用完', "Today's budget is spent"],
  total_limit: ['已到今日调用上限', "Today's call limit is reached"],
  per_provider_limit: ['已到这个提供方的调用上限', "This provider's call limit is reached"],
  capability_unverified: ['模型还没有验证', 'The model is not verified'],
  capability_not_declared: ['模型没有声明这项能力', 'The model does not declare this capability'],
  not_configured: ['还没有选择模型', 'No model chosen yet'],
  no_provider: ['没有可用的提供方', 'No usable provider'],
  no_eligible_provider: ['没有可用的提供方', 'No usable provider'],
  unknown_provider: ['提供方不在注册表里', 'The provider is not in the registry'],
  unknown_model: ['模型不在注册表里', 'The model is not in the registry'],
  invalid_registry: ['模型注册表无效', 'The model registry is invalid'],
  unauthorized: ['密钥被拒绝', 'The key was rejected'],
  auth_failed: ['密钥被拒绝', 'The key was rejected'],
  rate_limited: ['调用过于频繁', 'Rate-limited by the provider'],
  model_missing: ['提供方不认识这个模型', 'The provider does not know this model'],
  endpoint_unreachable: ['连不上端点', 'The endpoint could not be reached'],
  unreachable: ['连不上端点', 'The endpoint could not be reached'],
  network_error: ['网络错误', 'Network error'],
  timeout: ['调用超时', 'The call timed out'],
  invalid_response: ['回复无法读取', 'The response could not be read'],
  provider_error: ['提供方报告了错误', 'The provider reported an error'],
  config_error: ['配置无法使用', 'The configuration could not be used'],
  invalid_config: ['配置无效', 'Invalid configuration'],
  invalid_endpoint: ['端点地址无效', 'Invalid endpoint'],
  host_error: ['本地宿主出错', 'The local host failed'],
  host_protocol: ['本地宿主的回复无法读取', 'The local host reply could not be read'],
  storage_error: ['本地存储出错', 'Local storage failed'],
  search_unavailable: ['搜索不可用', 'Search is not available'],
  nothing_to_find: ['没有可查找的内容', 'Nothing to search for'],
  candidate_limit: ['候选结果超出上限', 'Too many candidates'],
  candidate_limit_exceeded: ['候选结果超出上限', 'Too many candidates'],
  hint_unavailable: ['这道题没有提示', 'No hint for this item'],
  guided_required: ['先完成引导阶段', 'The guided stage comes first'],
  not_practiceable: ['这份内容还不能练习', 'This content cannot be practised yet'],
  unknown_token: ['这道题已不在屏幕上', 'This item is no longer on screen'],
  presentation_closed: ['这道题已经关闭', 'This item was closed'],
  content_changed: ['题目内容已更新', 'The item changed'],
  content_unavailable: ['内容已不可用', 'The content is no longer available'],
  invalid_answer: ['答案无效', 'Invalid answer'],
  invalid_input: ['输入无效', 'Invalid input'],
  invalid_request: ['请求无效', 'Invalid request'],
  invalid_mode: ['导师模式无效', 'Invalid coach mode'],
  invalid_model: ['模型无效', 'Invalid model'],
  invalid_mark: ['标记无效', 'Invalid mark'],
  invalid_tactic: ['战术无效', 'Invalid tactic'],
  invalid_front: ['正面内容无效', 'Invalid front'],
  invalid_back: ['背面内容无效', 'Invalid back'],
  invalid_annotations: ['注释无效', 'Invalid annotations'],
  low_value: ['这一题不值得做成卡片', 'Not worth a card'],
  license_not_open: ['来源的许可不允许', 'The source licence does not allow this'],
  no_draft: ['没有打开的草稿', 'No open draft'],
  draft_closed: ['草稿已关闭', 'The draft is closed'],
  no_notes: ['还没有笔记', 'No notes yet'],
  no_attempts: ['还没有作答记录', 'No attempts yet'],
  not_found: ['没有找到', 'Not found'],
  unknown_attempt: ['作答记录不存在', 'Unknown attempt'],
  unknown_question: ['题目不存在', 'Unknown question'],
  unknown_draft: ['草稿不存在', 'Unknown draft'],
  unknown_note: ['笔记不存在', 'Unknown note'],
  unknown_source: ['来源不存在', 'Unknown source'],
  unknown_call: ['调用记录不存在', 'Unknown call'],
  anki_unavailable: ['Anki 连接不上', 'Anki is not reachable'],
  anki_error: ['Anki 报告了错误', 'Anki reported an error'],
  anki_failed: ['推送到 Anki 失败', 'The push to Anki failed'],
  anki_protocol: ['Anki 的回复无法读取', 'The Anki reply could not be read'],
  anki_conflict: ['你在 Anki 里改过这张卡', 'The card was edited in Anki'],
  anki_deck_mismatch: ['卡片进了别的牌组', 'The card landed in another deck'],
  incomparable_runs: ['两轮不可比较', 'The runs are not comparable'],
  run_abandoned: ['这轮测评已放弃', 'The run was abandoned'],
  run_state: ['测评状态已变化', 'The run changed state'],
  no_recommendation: ['没有可打开的练习', 'No practice to open'],
  unknown_skill: ['技能未注册', 'Unknown skill'],
  unknown_tactic: ['战术未注册', 'Unknown tactic'],
  credential_store_unavailable: ['系统密钥库不可用', 'No OS credential store'],
  credential_store_error: ['密钥库拒绝了密钥', 'The credential store refused the key'],
  brief_running: ['简报正在运行', 'The brief is already running'],
  internal_error: ['内部错误', 'Internal error'],
  quitting: ['正在退出', 'Closing'],
  starting: ['正在启动', 'Starting'],
  not_connected: ['学习服务未连接', 'The study service is not connected'],
};
const ERROR_GENERIC: Pair = ['没有完成', 'Could not complete'];

/** An error title as markup: the mapped title, or the generic title with the code muted beside it. */
export const errorLabel = (code: string): Html => coded(ERRORS, code, ERROR_GENERIC);
/** An error title as plain text (toasts, popover lines). */
export const errorTitle = (code: string): string => codedText(ERRORS, code, ERROR_GENERIC);

/** Typed provider failures (`provider.Failure*`), as recorded by a model test. */
const FAILURE_TYPES: Record<string, Pair> = {
  no_credential: ['没有密钥', 'no credential'],
  unauthorized: ['密钥被拒绝', 'key rejected'],
  rate_limited: ['调用过于频繁', 'rate-limited'],
  model_missing: ['提供方不认识这个模型', 'model unknown to the provider'],
  endpoint_unreachable: ['连不上端点', 'endpoint unreachable'],
  invalid_response: ['回复无法读取', 'unreadable response'],
  provider_error: ['提供方报告了错误', 'provider error'],
  agent_unavailable: ['本地 Agent 宿主不可用', 'local agent host unavailable'],
  config_error: ['配置无法使用', 'configuration error'],
};
export const failureTypeLabel = (type: string): Html => coded(FAILURE_TYPES, type, ['未知失败', 'unknown failure']);

/** Why a report item is excluded from the score (`assess.Reason*`), in the contamination sheet's words. */
const CONTAMINATION: Record<string, Pair> = {
  prior_exposure: ['运行前见过', 'seen before the run'],
  repeated_variant: ['重复变体', 'repeated variant'],
  hints_during_run: ['运行中看过提示', 'hint during the run'],
  resumed_after_window: ['超过时限后恢复', 'resumed after the window'],
};
export const contaminationLabel = (reason: string): Html => coded(CONTAMINATION, reason, ['其他原因', 'another reason']);

/** Local date-time for a server timestamp: MM-DD HH:MM (the prototype's compact form). */
export function shortStamp(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** Local date: YYYY-MM-DD. */
export function shortDate(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Local date: MM-DD. */
export function monthDay(iso: string | null | undefined): string {
  return iso ? shortDate(iso).slice(5) : '—';
}

export const sec = (ms: number | null): string => (ms === null ? '—' : String(Math.round(ms / 1000)));
