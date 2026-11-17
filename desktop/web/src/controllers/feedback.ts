// The feedback room: the evidence shown for an option, and the coach beside it — the send preview of
// the current mode, a send, its reply or failure, and the attempt's recorded coach calls.
import type { CoachMode } from '../api/types';
import { lightSpan } from '../components/passage';
import { COACH_MODES } from '../core/i18n';
import { renderOrb } from '../core/render';
import { data, idle, ui } from '../core/state';
import { optionSpan } from '../views/feedback';
import { $, reduced, type Act } from './dom';
import { call, loadToday, ready } from './service';

/**
 * The send preview of the current mode. `afterSend`: a reply just arrived; the preview is read again
 * in place (no loading state) so the call strip shows the budget slot the send spent.
 */
let previewSeq = 0;

export async function loadCoachPreview(afterSend = false): Promise<void> {
  const fb = data.feedback;
  if (!fb) return;
  const mode = ui.coachMode;
  // Every request (even one that stops here) supersedes reads still in flight: only the newest lands.
  const seq = ++previewSeq;
  if (data.coach.replies[mode] && !afterSend) return;
  if (!afterSend) {
    data.coach.preview = { status: 'loading' };
    renderOrb();
  }
  const r = await call((a) => a.coachPreview(fb.attempt_id, mode));
  if (seq !== previewSeq || ui.coachMode !== mode || data.feedback !== fb) return;
  data.coach.preview = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  renderOrb();
}

/** The attempt's recorded coach calls; rendered in the coach sheet's history view. */
async function loadCoachHistory(): Promise<void> {
  const fb = data.feedback;
  if (!fb) return;
  data.coach.history = { status: 'loading' };
  renderOrb();
  const r = await call((a) => a.coachHistory(fb.attempt_id));
  if (data.feedback !== fb) return;
  data.coach.history = r.ok ? ready(r.value) : { status: 'error', error: r.error };
  renderOrb();
}

export function resetCoach(): void {
  data.coach = { preview: idle(), replies: {}, failed: null, failure: null, sending: false, history: idle() };
  ui.coachHistory = false;
}

export function setEv(label: string): void {
  const fb = data.feedback;
  if (!fb || ui.feedback.ev === label) return;
  ui.feedback.ev = label;
  document.querySelectorAll<HTMLElement>('.fo').forEach((f) => f.classList.toggle('on', f.dataset.ev === label));
  const span = optionSpan(fb, label);
  if (span) lightSpan(fb.passage.paragraphs, span, true, !reduced());
}

export const coachActs: Record<string, Act> = {
  mode(v) {
    const m = COACH_MODES.find(([k]) => k === v)?.[0];
    if (!m) return;
    ui.coachMode = m as CoachMode;
    ui.coachHistory = false;
    data.coach.failed = null;
    data.coach.failure = null;
    data.coach.preview = idle();
    renderOrb();
    $(`[data-act="mode"][data-v="${m}"]`)?.focus({ preventScroll: true });
    void loadCoachPreview();
  },
  'coach-send'() {
    const fb = data.feedback;
    const mode = ui.coachMode;
    if (!fb || data.coach.sending) return;
    data.coach.sending = true;
    data.coach.failed = null;
    data.coach.failure = null;
    renderOrb();
    void call((a) => a.coachAsk(fb.attempt_id, mode)).then((r) => {
      data.coach.sending = false;
      if (data.feedback !== fb) return;
      // The call is recorded either way: the history is stale.
      data.coach.history = idle();
      // Only an advisory reply is kept for the mode; a failure stays retryable (coach-retry).
      if (r.ok && r.value.state === 'advisory') data.coach.replies[mode] = r.value;
      else if (ui.coachMode === mode) {
        if (r.ok) data.coach.failed = r.value;
        else data.coach.failure = r.error;
      }
      renderOrb();
      void loadToday();
      // The call spent a budget slot: the strip reads the core's count again.
      if (ui.coachMode === mode) void loadCoachPreview(true);
    });
  },
  'coach-history'() {
    if (!data.feedback) return;
    ui.coachHistory = !ui.coachHistory;
    renderOrb();
    $('[data-act="coach-history"]')?.focus({ preventScroll: true });
    if (ui.coachHistory) void loadCoachHistory();
  },
  'coach-retry'() {
    if (data.coach.sending) return;
    data.coach.failed = null;
    data.coach.failure = null;
    data.coach.preview = idle();
    renderOrb();
    void loadCoachPreview();
  },
};
