// The window's data types are the MyGo-generated ones (./mygo.ts, from `internal/studio`): a field
// the Go core does not report does not exist here, so a view can only show it as unknown.
// This module adds the two window-side types the generated client has no name for.
export type * from './mygo';

/**
 * A failed call, parsed from the CallError message (`{"code","message","action"}`, see
 * internal/desktop/study.go). `message` is core text; `action` is the learner's next step, when any.
 */
export interface StudioError {
  code: string;
  message: string;
  action: string | null;
}

/** Coach request modes (`tutor preview|ask --mode`). */
export type CoachMode = 'explain' | 'probe' | 'micro_lesson';
