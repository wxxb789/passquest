// The study API the window talks to: exactly the generated `Study` client (./mygo.ts), called
// through `window.mygo`. ./mygo.ts imports `mygo-runtime`, which is not a dependency here, so it is
// imported for its types only (erased by the bundler); the runtime MyGo injects into the page is
// the same `call` (see ./mygo-runtime.d.ts).
//
// Error convention (internal/desktop/study.go): a failed Go call rejects with a CallError whose
// message is the JSON of a studio.Error, `{"code","message","action"}`. `parseCallError` turns it
// back into a StudioError; a message that is not that JSON becomes code `internal`.
import type { Study } from './mygo';
import type { StudioError } from './types';

type StudyClient = typeof Study;
export type StudyApi = { [K in keyof StudyClient]: StudyClient[K] };

/** Every bound method, by its client name. The type check below fails when one is missing. */
const METHODS = [
  'abandonAssessment',
  'acceptNote',
  'answer',
  'answerAssessmentItem',
  'assessmentCompare',
  'assessmentPool',
  'assessmentReport',
  'assessmentRuns',
  'assessmentState',
  'briefStatus',
  'budget',
  'cancelBrief',
  'clearCredential',
  'coachAsk',
  'coachPreview',
  'consents',
  'grantConsent',
  'models',
  'notes',
  'openHint',
  'presentAssessmentItem',
  'previewNote',
  'proposeNote',
  'pushNotes',
  'resumeAssessment',
  'resumeBrief',
  'resumePractice',
  'revokeConsent',
  'services',
  'setCredential',
  'setLearnerSettings',
  'skip',
  'skipNote',
  'startAssessment',
  'startPractice',
  'testModel',
  'today',
  'useModel',
] as const satisfies readonly (keyof StudyClient)[];

type Unlisted = Exclude<keyof StudyClient, (typeof METHODS)[number]>;
const everyMethodListed: [Unlisted] extends [never] ? true : Unlisted = true;
void everyMethodListed;

let bound: StudyApi | null = null;

export function bindApi(api: StudyApi): void {
  bound = api;
}

export function api(): StudyApi | null {
  return bound;
}

function isStudioError(value: unknown): value is { code: string; message: string; action?: unknown } {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { code?: unknown }).code === 'string' &&
    typeof (value as { message?: unknown }).message === 'string'
  );
}

function normalise(v: { code: string; message: string; action?: unknown }): StudioError {
  return { code: v.code, message: v.message, action: typeof v.action === 'string' && v.action !== '' ? v.action : null };
}

/** Parses what a failed call rejects with (a CallError carrying the JSON) into a StudioError. */
export function parseCallError(e: unknown): StudioError {
  if (isStudioError(e)) return normalise(e);
  const message = e instanceof Error ? e.message : String(e);
  try {
    const doc: unknown = JSON.parse(message);
    if (isStudioError(doc)) return normalise(doc);
  } catch {
    // not the convention's JSON
  }
  return { code: 'internal', message, action: null };
}

export const NOT_CONNECTED: StudioError = {
  code: 'not_connected',
  message: 'The study service is not connected to this window.',
  action: null,
};

/**
 * The client method `name` over `window.mygo`. Go method names are the client's names with the
 * first letter upper-cased (`assessmentPool` ↔ `Study.AssessmentPool`); no Study method starts
 * with an acronym, so the mapping is exact.
 */
function studyCall(runtime: MygoRuntime, name: string): (...args: unknown[]) => Promise<unknown> {
  const method = `Study.${name.charAt(0).toUpperCase()}${name.slice(1)}`;
  return (...args) =>
    runtime.call(method, ...args).catch((e: unknown) => {
      throw parseCallError(e);
    });
}

/** The StudyApi over the bound Go service, or null outside a MyGo window. */
export function connectStudy(): StudyApi | null {
  const runtime = window.mygo;
  if (!runtime) return null;
  return Object.fromEntries(METHODS.map((name) => [name, studyCall(runtime, name)])) as unknown as StudyApi;
}

/** Subscribes to a Go event (e.g. `brief:changed`); a no-op outside a MyGo window. */
export function onGoEvent<T>(name: string, listener: (payload: T) => void): void {
  window.mygo?.on<T>(name, listener);
}
