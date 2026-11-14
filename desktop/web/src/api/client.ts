// The study API the window talks to: one async method per `cli.Workspace` method in the 0011 plan.
//
// PROVISIONAL like ./types.ts: the implementation is a thin wrapper over the MyGo-generated
// `./mygo.ts` client and is bound at boot with `bindApi()`. Until one is bound, every screen renders
// its "service not connected" state — the window never substitutes demo data.
import type {
  AssessmentComparison,
  AssessmentItem,
  AssessmentPool,
  AssessmentReport,
  AssessmentState,
  BriefSummary,
  BudgetView,
  CoachMode,
  CoachPreview,
  CoachReply,
  ConsentLedger,
  Feedback,
  LearnerSettings,
  ModelRegistry,
  ModelTestResult,
  NoteDraft,
  NoteFields,
  NoteList,
  NotePreview,
  NoteRequest,
  PracticeItem,
  PracticeRequest,
  PushResult,
  RunSummary,
  Services,
  StudioError,
  Today,
} from './types';

export interface StudyApi {
  // Today
  today(): Promise<Today>;
  setLearnerSettings(settings: LearnerSettings): Promise<void>;

  // Practice
  startPractice(request: PracticeRequest): Promise<PracticeItem>;
  /** The open presentation after an interruption, or null. */
  resumePractice(): Promise<PracticeItem | null>;
  openHint(token: string): Promise<PracticeItem>;
  answer(token: string, label: string): Promise<Feedback>;
  skip(token: string): Promise<Feedback>;

  // Coach
  coachPreview(attemptId: number, mode: CoachMode): Promise<CoachPreview>;
  coachAsk(attemptId: number, mode: CoachMode): Promise<CoachReply>;
  grantConsent(sourceId: string, provider: string): Promise<void>;
  revokeConsent(sourceId: string, provider: string): Promise<void>;

  // Assessment
  assessmentPool(): Promise<AssessmentPool>;
  startAssessment(size: number, minutes: number): Promise<AssessmentState>;
  assessmentState(): Promise<AssessmentState>;
  /**
   * Presents the run's current item. `position` is a provisional extension for the navigator's
   * goto; the plan's signature takes only the run.
   */
  presentAssessmentItem(runId: number, position?: number): Promise<AssessmentItem>;
  answerAssessmentItem(runId: number, position: number, label: string): Promise<AssessmentState>;
  resumeAssessment(runId: number): Promise<AssessmentState>;
  abandonAssessment(runId: number): Promise<AssessmentState>;

  // Report
  assessmentReport(runId: number): Promise<AssessmentReport>;
  assessmentCompare(baseRunId: number, otherRunId: number): Promise<AssessmentComparison>;
  assessmentRuns(): Promise<RunSummary[]>;

  // Anki
  proposeNote(request: NoteRequest): Promise<NoteDraft>;
  previewNote(draftId: number, fields: NoteFields): Promise<NotePreview>;
  acceptNote(draftId: number, fields: NoteFields): Promise<NoteDraft>;
  skipNote(draftId: number): Promise<NoteDraft>;
  notes(): Promise<NoteList>;
  pushNotes(force: boolean): Promise<PushResult>;

  // Settings
  models(): Promise<ModelRegistry>;
  useModel(providerId: string, endpoint: string | null): Promise<ModelRegistry>;
  testModel(providerId: string, endpoint: string | null): Promise<ModelTestResult>;
  budget(): Promise<BudgetView>;
  consents(): Promise<ConsentLedger>;
  /** Stores the key in the OS credential store. The key is never echoed back. */
  setCredential(providerId: string, key: string): Promise<void>;
  clearCredential(providerId: string): Promise<void>;
  services(): Promise<Services>;

  // Brief
  briefStatus(): Promise<BriefSummary | null>;
  resumeBrief(): Promise<void>;
  cancelBrief(): Promise<void>;
}

let bound: StudyApi | null = null;

export function bindApi(api: StudyApi): void {
  bound = api;
}

export function api(): StudyApi | null {
  return bound;
}

export function isStudioError(value: unknown): value is StudioError {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { code?: unknown }).code === 'string' &&
    typeof (value as { message?: unknown }).message === 'string'
  );
}

/** Normalises anything a call rejects with into a StudioError. */
export function toStudioError(value: unknown): StudioError {
  if (isStudioError(value)) return { code: value.code, message: value.message, action: value.action ?? null };
  const message = value instanceof Error ? value.message : String(value);
  return { code: 'internal', message, action: null };
}

export const NOT_CONNECTED: StudioError = {
  code: 'not_connected',
  message: 'The study service is not connected to this window.',
  action: null,
};
