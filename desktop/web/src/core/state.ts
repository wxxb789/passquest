// UI-only state plus a cache of the last values the study service returned. Domain truth is never
// computed here: the cache is replaced wholesale by the next service response.
import type {
  AssessmentComparison,
  AssessmentPool,
  AssessmentReport,
  AssessmentRun,
  AssessmentState,
  Budget,
  CoachMode,
  CoachPreview,
  CoachReply,
  Consent,
  Feedback,
  ModelList,
  NoteDraft,
  NoteFields,
  NoteList,
  NotePreview,
  PracticeItem,
  Services,
  StudioError,
  Today,
} from '../api/types';
import { loadPrefs, type Lang, type Theme } from './prefs';

export type { Lang, Theme } from './prefs';

export const ROUTES = ['today', 'practice', 'feedback', 'assess', 'report', 'anki', 'settings'] as const;
export type Route = (typeof ROUTES)[number];

/** Modal and non-modal layers. `pop` is the only non-modal one. */
export type Layer =
  | 'keys'
  | 'hint'
  | 'leave'
  | 'pop'
  | 'learner'
  | 'credential'
  | 'start-run'
  | 'abandon-run'
  | `sheet:${string}`;

export type Loadable<T> =
  | { status: 'idle' }
  | { status: 'loading'; value?: T }
  | { status: 'ready'; value: T }
  | { status: 'error'; error: StudioError; value?: T };

export const idle = <T>(): Loadable<T> => ({ status: 'idle' });

/** A passage selection kept for a later Anki note: UTF-16 offsets within one paragraph, plus its text. */
export interface Mark {
  p: number;
  a: number;
  b: number;
  text: string;
}

const prefs = loadPrefs();

export const ui = {
  theme: prefs.theme as Theme,
  lang: prefs.lang as Lang,
  shot: prefs.shot,
  route: 'today' as Route,
  sub: null as string | null,
  layer: null as Layer | null,
  /** Selector of the element to refocus when a layer closes. */
  ret: null as string | null,
  /** Last visited step of the session walk. */
  sessionAt: -1,
  /** A call is in flight; actions that write are ignored until it settles. */
  busy: false,
  coachOpen: true,
  coachMode: 'probe' as CoachMode,
  practice: { sel: null as string | null, marks: [] as Mark[] },
  feedback: { ev: null as string | null },
  /**
   * `attached`: this window is driving the active run. A run found on boot or after leaving is
   * shown as resumable first; resuming counts one resume (`assess resume`).
   */
  assess: { sel: null as string | null, attached: false },
  report: { runId: null as number | null, compareWith: null as number | null, annItem: null as number | null },
  anki: {
    card: null as Theme | null,
    width: 'desk' as 'desk' | 'phone',
    editing: false,
    /** Local edits of the draft's fields; null = unedited. */
    fields: null as NoteFields | null,
    dirty: false,
  },
  settings: {
    open: null as string | null,
    credentialFor: null as string | null,
  },
};

export const data = {
  today: idle<Today>(),
  /** Local helpers (Node, AnkiConnect endpoint, search, credential store); Today's brief and Settings read it. */
  services: idle<Services>(),
  /** The provider registry; Settings and the coach's aggregated "nothing usable" line read it. */
  models: idle<ModelList>(),
  practice: idle<PracticeItem | null>(),
  feedback: null as Feedback | null,
  coach: {
    preview: idle<CoachPreview>(),
    replies: {} as Partial<Record<CoachMode, CoachReply>>,
    failure: null as StudioError | null,
    sending: false,
  },
  assess: {
    state: idle<AssessmentState>(),
    pool: idle<AssessmentPool>(),
  },
  report: {
    runs: idle<AssessmentRun[]>(),
    report: idle<AssessmentReport>(),
    compare: idle<AssessmentComparison>(),
  },
  anki: {
    draft: null as NoteDraft | null,
    preview: idle<NotePreview>(),
    notes: idle<NoteList>(),
  },
  settings: {
    budget: idle<Budget>(),
    consents: idle<Consent[]>(),
  },
};

export function valueOf<T>(l: Loadable<T>): T | undefined {
  return l.status === 'ready' ? l.value : 'value' in l ? l.value : undefined;
}
