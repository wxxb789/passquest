// Boot-path adapters from the generated Go DTOs (./mygo.ts) to the provisional view types
// (./types.ts). They only rename and reshape: a field the Go side does not report is either derived
// from a reported one (each derivation says from what) or given the value that makes the view claim
// nothing. The wiring phase replaces these by moving the views onto the generated types.
import type {
  AnkiCounts as GoAnkiCounts,
  AssessmentPool as GoAssessmentPool,
  AssessmentRun as GoAssessmentRun,
  BriefSummary as GoBriefSummary,
  ModelStatus as GoModelStatus,
  Recommendation as GoRecommendation,
  SkillEvidence as GoSkillEvidence,
  Today as GoToday,
} from './mygo';
import type {
  ActiveModel,
  AnkiCounts,
  BriefState,
  BriefSummary,
  LearnerSettings,
  PoolSummary,
  Recommendation,
  RunSummary,
  SkillEvidence,
  Today,
} from './types';

type SkillKind = SkillEvidence['kind'];
const skillKind = (kind: string): SkillKind => (kind === 'scoring' || kind === 'language' ? kind : 'unclassified');

function run(r: GoAssessmentRun | null): RunSummary | null {
  if (!r) return null;
  return {
    run_id: r.id,
    state: r.status === 'completed' || r.status === 'abandoned' ? r.status : 'active',
    started_at: r.started_at,
    completed_at: r.completed_at ?? null,
    size: r.actual_size,
    minutes: r.window_minutes,
    // Today carries no score; the report does.
    correct: null,
  };
}

function recommendation(r: GoRecommendation | null): Recommendation | null {
  if (!r) return null;
  const t = r.target;
  return {
    action: r.action,
    // A lesson target opens skill practice in this window; Go has no "lesson" practice kind.
    kind: t.kind === 'tactic' ? 'tactic' : 'skill',
    skill_id: r.skill.id || null,
    skill_kind: r.skill.kind ? skillKind(r.skill.kind) : null,
    content_id: t.content_id ?? null,
    question_id: t.question_id ?? null,
    source_id: t.source_id ?? null,
    reason: r.reason,
    // Derived: the policy's practice_new action targets fresh material; a guided stage is assisted.
    can_count: r.action === 'practice_new' && t.kind !== 'lesson' && t.stage !== 'guided',
    command: r.command,
  };
}

function skill(s: GoSkillEvidence): SkillEvidence {
  return {
    skill_id: s.skill.id,
    kind: skillKind(s.skill.kind),
    independent: s.evidence.independent_attempts,
    independent_correct: s.evidence.independent_correct,
    assisted: s.evidence.assisted_attempts,
    re_exposure: s.evidence.passage_repeats,
    action: s.inference.action,
    causal_diagnosis: s.inference.causal_diagnosis,
    reason: s.inference.reason || null,
  };
}

function pool(p: GoAssessmentPool): PoolSummary | null {
  if (p.source_count === 0) return null;
  return {
    items: p.qualifying_items,
    units: p.qualifying_units,
    spent_units: p.spent_units,
    floor_items: p.floor_items,
    can_start: p.floor_met,
  };
}

function anki(a: GoAnkiCounts): AnkiCounts {
  return {
    drafts: a.open_drafts,
    accepted: a.total,
    pushed: a.pushed,
    queued: a.queued,
    already_present: a.already_present,
    conflict: a.conflict,
    // deck_mismatch has no slot in the provisional view; it is a failed push.
    failed: a.failed + a.deck_mismatch,
  };
}

function brief(b: GoBriefSummary | null): BriefSummary | null {
  if (!b) return null;
  return {
    run_id: b.run_id,
    state: b.state as BriefState,
    step: b.step,
    max_steps: b.max_steps,
    provider: b.provider ?? null,
    model: b.model || null,
    // Not in Today: resume reports node_unavailable itself, so the view does not claim it here.
    node_available: true,
  };
}

function model(m: GoModelStatus): ActiveModel | null {
  if (!m.configured || !m.provider || !m.model) return null;
  const usage = m.budget.providers.find((p) => p.provider === m.provider);
  return {
    provider: m.provider,
    provider_name: usage?.label || m.provider,
    model: m.model,
    // Today does not report a verification time; Settings → Models has the capability facts.
    verified_at: null,
    used_today: usage?.calls ?? 0,
    limit: usage?.limit ?? m.budget.per_provider_limit,
  };
}

export function adaptToday(t: GoToday): Today {
  return {
    now: t.generated_at,
    prep_day: t.prep.day > 0 ? t.prep.day : null,
    prep_started_at: t.prep.first_attempt_at ?? null,
    learner: {
      exam_date: t.prep.exam_date || null,
      daily_minutes: t.prep.daily_minutes > 0 ? t.prep.daily_minutes : null,
    },
    pass_threshold: 'unknown',
    active_run: run(t.active_run),
    recommendation: recommendation(t.recommendation),
    skills: t.skills.map(skill),
    pool: pool(t.pool),
    last_run: run(t.last_run),
    anki: anki(t.anki),
    brief: brief(t.brief),
    find: { configured: t.find.configured, calls_today: t.find.queries_used },
    model: model(t.model),
    providers_unusable: t.model.problem ? { code: 'provider_unusable', message: t.model.problem, action: null } : null,
  };
}

/** Learner settings as the Go side takes them: an empty date or 0 minutes clears the value. */
export function goLearnerSettings(s: LearnerSettings): { exam_date: string; daily_minutes: number } {
  return { exam_date: s.exam_date ?? '', daily_minutes: s.daily_minutes ?? 0 };
}
