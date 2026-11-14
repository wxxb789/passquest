// PROVISIONAL — hand-written description of the data each screen needs.
//
// This file is a stand-in until MyGo generates `api/mygo.ts` from the bound Go service
// (`desktop.Study` → `cli.Workspace`, `internal/studio` DTOs). When that client exists, these types are
// replaced by (or re-exported from) the generated ones and every mismatch becomes a type error in the
// views — which is the point. Field names are snake_case to match the Go JSON tags.
//
// Rules carried by these shapes (design brief, docs/core-design.md):
// - nothing here is invented by the window: every number comes from the Go core;
// - the pass threshold is always the literal 'unknown';
// - exam date and daily minutes are optional learner settings, null until set (Q-0011-06);
// - times are RFC 3339 strings stamped by the server; the window only displays them;
// - credentials are reported as presence only, never as values.

/** RFC 3339 timestamp produced by the Go core. */
export type Iso = string;

/** Typed error returned by every workspace method (`studio.Error`). Codes reuse the CLI error codes. */
export interface StudioError {
  code: ErrorCode;
  message: string;
  /** A next step the learner can take, e.g. a command or a settings location. */
  action: string | null;
}
export type ErrorCode =
  | 'invalid_input'
  | 'assessment_active'
  | 'pool_floor'
  | 'no_credential'
  | 'no_consent'
  | 'budget_exhausted'
  | 'node_unavailable'
  | 'anki_unreachable'
  | 'provider_unusable'
  | 'not_found'
  | 'internal'
  | (string & {});

/** Attempt condition as recorded by the core. */
export type Condition = 'independent' | 'assisted' | 're_exposure' | 'revisit' | 'assessment';

/** A half-open range of Unicode code points inside one passage paragraph. */
export interface TextSpan {
  paragraph: number;
  start: number;
  end: number;
}

export interface SourceLabel {
  /** `embedded` sample content, `import` for a local imported source, `assessment` pool unit. */
  kind: 'embedded' | 'import' | 'assessment';
  source_id: string | null;
  page: number | null;
  unit_id: string | null;
  reviewed: boolean;
}

export interface Passage {
  content_id: string;
  title: string;
  source: SourceLabel;
  /** Plain-text paragraphs, exactly as stored. Never HTML. */
  paragraphs: string[];
}

export interface OptionText {
  label: string;
  text: string;
}

export interface TacticRef {
  id: string;
  name: string;
  summary: string;
}

// ---------------------------------------------------------------- Today

export interface LearnerSettings {
  /** YYYY-MM-DD as entered by the learner, or null when not set. */
  exam_date: string | null;
  daily_minutes: number | null;
}

export interface SkillEvidence {
  skill_id: string;
  kind: 'scoring' | 'language' | 'unclassified';
  independent: number;
  independent_correct: number;
  assisted: number;
  re_exposure: number;
  /** Policy action id for this skill, e.g. `practice_new`, `review`. */
  action: string;
  causal_diagnosis: 'unresolved' | (string & {});
  /** Policy text produced locally by the transparent policy. */
  reason: string | null;
}

export type PracticeKind = 'next' | 'skill' | 'tactic' | 'sample' | 'source';

export interface Recommendation {
  action: string;
  kind: PracticeKind;
  skill_id: string | null;
  skill_kind: SkillEvidence['kind'] | null;
  content_id: string | null;
  question_id: string | null;
  source_id: string | null;
  reason: string;
  /** True when the recommended material is fresh, so the attempt can count as independent. */
  can_count: boolean;
  /** Equivalent CLI command, shown for transparency. */
  command: string;
}

export interface PoolSummary {
  items: number;
  units: number;
  spent_units: number;
  floor_items: number;
  can_start: boolean;
}

export interface RunSummary {
  run_id: number;
  state: 'active' | 'completed' | 'abandoned';
  started_at: Iso;
  completed_at: Iso | null;
  size: number;
  minutes: number;
  correct: number | null;
}

export interface AnkiCounts {
  drafts: number;
  accepted: number;
  pushed: number;
  queued: number;
  already_present: number;
  conflict: number;
  failed: number;
}

export type BriefState = 'running' | 'paused' | 'completed' | 'failed' | 'needs_action' | 'cancelling' | 'cancelled';

export interface BriefSummary {
  run_id: string;
  state: BriefState;
  step: number;
  max_steps: number;
  provider: string | null;
  model: string | null;
  /** Node host availability: a brief cannot resume without it. */
  node_available: boolean;
}

export interface FindStatus {
  configured: boolean;
  calls_today: number;
}

export interface ActiveModel {
  provider: string;
  provider_name: string;
  model: string;
  verified_at: Iso | null;
  used_today: number;
  limit: number;
}

export interface Today {
  now: Iso;
  /** Prep day derived from the first recorded attempt; null before any attempt. */
  prep_day: number | null;
  prep_started_at: Iso | null;
  learner: LearnerSettings;
  pass_threshold: 'unknown';
  /** Set when an assessment run is active: everything else routes to it. */
  active_run: RunSummary | null;
  recommendation: Recommendation | null;
  skills: SkillEvidence[];
  /** Null when no reviewed source exists to build a pool from. */
  pool: PoolSummary | null;
  last_run: RunSummary | null;
  anki: AnkiCounts;
  brief: BriefSummary | null;
  find: FindStatus;
  /** Null when no model is configured or none is usable. */
  model: ActiveModel | null;
  /** Aggregated line when every provider is unusable (Q-0011-09), else null. */
  providers_unusable: StudioError | null;
}

// ---------------------------------------------------------------- Practice

export interface PracticeRequest {
  kind: PracticeKind;
  skill_id?: string;
  tactic_id?: string;
  stage?: 'guided' | 'independent';
  source_id?: string;
  page?: number;
  question_id?: string;
}

export interface Hint {
  /** Authored guided walkthrough text (Q-0011-05). Never AI text. */
  title: string;
  steps: string[];
  tactic_id: string;
}

export interface PracticeItem {
  /** Presentation token binding an answer to the displayed item. */
  token: string;
  presented_at: Iso;
  passage: Passage;
  question: {
    question_id: string;
    number: number;
    count: number;
    stem: string;
    options: OptionText[];
    skill_ids: string[];
  };
  condition: Condition;
  can_count: boolean;
  hint_available: boolean;
  /** Present once the hint has been opened (the attempt is then assisted). */
  hint: Hint | null;
  tactic: TacticRef | null;
}

// ---------------------------------------------------------------- Feedback

export interface OptionFeedback extends OptionText {
  explanation: string | null;
  explanation_missing: boolean;
  /** Where this option's support sits in the passage, when the core knows it. */
  evidence: TextSpan | null;
}

export interface Feedback {
  attempt_id: number;
  token: string;
  status: 'answered' | 'skipped';
  verdict: 'correct' | 'incorrect' | 'skipped';
  selected_label: string | null;
  key: OptionText;
  options: OptionFeedback[];
  evidence: { text: string; span: TextSpan | null };
  condition: Condition;
  counts: boolean;
  elapsed_ms: number;
  answered_at: Iso;
  question: { question_id: string; number: number; count: number; stem: string; skill_ids: string[] };
  passage: Passage;
  tactic: TacticRef | null;
}

// ---------------------------------------------------------------- Coach

export type CoachMode = 'explain' | 'probe' | 'micro_lesson';

export interface ConsentState {
  source_id: string | null;
  provider: string;
  /** False for first-party content, which needs no consent. */
  required: boolean;
  granted: boolean;
  changed_at: Iso | null;
  /** Payload fields withheld because consent is missing. */
  withheld: string[];
}

export interface CoachPreview {
  attempt_id: number;
  mode: CoachMode;
  provider: string;
  provider_name: string;
  model: string;
  verified: boolean;
  external: boolean;
  /** Human-readable names of the payload fields that will be sent. */
  payload_summary: string[];
  request_sha256: string;
  request_bytes: number;
  consent: ConsentState;
  budget: { used: number; limit: number };
  /** Why sending is not possible right now, with its action; null when it is. */
  blocked: StudioError | null;
}

export interface CoachReply {
  attempt_id: number;
  mode: CoachMode;
  provider: string;
  model: string;
  /** Advisory model text. Rendered as text only, never as HTML. */
  text: string;
  at: Iso;
  failover_from: string | null;
}

// ---------------------------------------------------------------- Assessment

export type CellState = 'answered' | 'timed_out' | 'pending' | 'unavailable';

export interface AssessmentPool extends PoolSummary {
  default_size: number;
  default_minutes: number;
  /** `pool_floor` shortfall when a run cannot start. */
  shortfall: StudioError | null;
}

export interface ActiveRun {
  run_id: number;
  size: number;
  minutes: number;
  started_at: Iso;
  deadline_at: Iso;
  per_item_ms: number;
  current_position: number | null;
  resume_count: number;
  /** Navigator states. Never carries correctness. */
  cells: { position: number; state: CellState }[];
}

export interface AssessmentState {
  active: ActiveRun | null;
  /** The run that just completed, when the last call finished it. */
  completed_run_id: number | null;
}

export interface AssessmentItem {
  run_id: number;
  position: number;
  size: number;
  presented_at: Iso;
  item_deadline_at: Iso;
  run_deadline_at: Iso;
  per_item_ms: number;
  passage: Passage;
  /** Positions of the run that share this passage. */
  passage_positions: number[];
  stem: string;
  options: OptionText[];
  selected_label: string | null;
}

// ---------------------------------------------------------------- Report

export type ItemOutcome = 'correct' | 'incorrect' | 'timed_out' | 'unanswered' | 'excluded';

export interface ReportItem {
  position: number;
  outcome: ItemOutcome;
  skill_ids: string[];
  stem: string;
  options: OptionText[];
  key_label: string;
  selected_label: string | null;
  annotations: {
    withheld: boolean;
    evidence: string | null;
    reasoning: string | null;
    option_reasoning: OptionText[];
    option_reasoning_missing: string[];
  };
}

export interface AssessmentReport {
  run: RunSummary;
  pass_threshold: 'unknown';
  counts: { correct: number; incorrect: number; timed_out: number; unanswered: number; excluded: number; scored: number };
  skills: { skill_id: string; correct: number; total: number }[];
  speed: { median_ms: number | null; p90_ms: number | null };
  per_item_ms: number;
  contamination: {
    prior_exposure: number;
    repeated_variant: number;
    hints_during_run: number;
    resumed_after_window: number;
    excluded_positions: number[];
  };
  pool_after: { items: number; units: number; floor_items: number; items_before: number } | null;
  items: ReportItem[];
  unknowns: string[];
  limitations: string[];
}

export interface AssessmentComparison {
  base: RunSummary;
  other: RunSummary;
  like_for_like: boolean;
  correct_delta: number | null;
  checks: { name: 'same_size' | 'same_window' | 'same_pool_family' | (string & {}); ok: boolean }[];
  refusal: StudioError | null;
}

// ---------------------------------------------------------------- Anki

export interface NoteRequest {
  attempt_id?: number;
  content_id?: string;
  question_id?: string;
  /** Passage selection text (≤120 runes after the core's NormalizeMark). */
  mark?: string;
}

export interface NoteFields {
  front: string;
  back_explanation: string;
  back_example: string;
  tactic: string;
}

export interface NoteDraft {
  draft_id: number;
  status: 'draft' | 'accepted' | 'skipped';
  /** Accepted revision, null before the first accept. */
  revision: number | null;
  fields: NoteFields;
  source: { source_id: string | null; page: number | null; question_id: string | null } | null;
  attempt_id: number | null;
}

export interface NotePreview {
  draft_id: number;
  /** Real `note.RenderCard` output. Rendered only inside a sandboxed iframe, never into the app DOM. */
  front_html: string;
  back_html: string;
  /** Real `note.ModelCSS`. */
  css: string;
  html_sha256: string;
}

export type AnkiState = 'local' | 'queued' | 'pushed' | 'already_present' | 'conflict' | 'failed';

export interface NoteSummary {
  note_id: number;
  front: string;
  state: AnkiState;
  revision: number;
  /** Conflict / failure code and detail from the outbox. */
  code: string | null;
  detail: string | null;
}

export interface NoteList {
  anki: { endpoint: string; reachable: boolean | null };
  counts: AnkiCounts;
  notes: NoteSummary[];
}

export interface PushResult {
  counts: AnkiCounts;
  notes: NoteSummary[];
  /** Set when AnkiConnect could not be reached: notes stay queued. */
  unreachable: StudioError | null;
}

// ---------------------------------------------------------------- Settings

export type CapabilityState = 'verified' | 'declared' | 'failed' | 'unsupported';

export interface ProviderRow {
  id: string;
  name: string;
  model: string | null;
  endpoint: string | null;
  in_use: boolean;
  text_chat: { state: CapabilityState; checked_at: Iso | null; failure_type: string | null; action: string | null };
  capabilities: { name: string; state: CapabilityState }[];
  credential: 'present' | 'missing' | 'not_required';
  credential_env: string | null;
  /** True when an environment variable supplies the key (it wins over the OS store, Q-0011-03). */
  credential_from_env: boolean;
}

export interface ModelRegistry {
  active_id: string | null;
  providers: ProviderRow[];
  /** Providers eligible for failover from the active one (verified, budget left, equal consent scope). */
  failover: string[];
  credential_store: { available: boolean; reason: string | null };
}

export interface ModelTestResult {
  provider: string;
  model: string;
  state: CapabilityState;
  checked_at: Iso;
  failure: StudioError | null;
}

export interface BudgetView {
  per_provider_limit: number;
  providers: { provider: string; name: string; used: number; limit: number; usable: boolean; reason: string | null }[];
  total_calls: number;
  total_limit: number;
}

export interface ConsentEvent {
  at: Iso;
  source_id: string;
  provider: string;
  granted: boolean;
  reviewer: string;
}

export interface ConsentLedger {
  events: ConsentEvent[];
  current: { source_id: string; provider: string; granted: boolean; at: Iso }[];
  /** Fields a grant allows to be sent. */
  scope_fields: string[];
}

export interface Services {
  node: { available: boolean; version: string | null; reason: string | null };
  anki_connect: { endpoint: string; reachable: boolean | null };
  search: { configured: boolean; calls_today: number };
}
