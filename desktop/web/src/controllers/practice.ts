// The practice room: starting an item, choosing, the hint, marking the passage, answering or skipping
// (which opens feedback), and proposing a note from the item or its feedback.
import type { Feedback, PracticeRequest } from '../api/types';
import type { StudyApi } from '../api/client';
import { markCountText, passageHTML, practiceFoot, takeMark } from '../components/passage';
import { closeLayer, openLayer, render, renderLayer } from '../core/render';
import { go } from '../core/router';
import { data, idle, ui } from '../core/state';
import { practiceItem } from '../screens';
import { $, setChecked, type Act } from './dom';
import { busyRefused, load, loadToday, ready, write } from './service';
import { propose } from './anki';
import { resetCoach } from './feedback';

export async function loadPracticeResume(): Promise<void> {
  if (data.practice.status === 'ready' || data.practice.status === 'loading') return;
  await load((a) => a.resumePractice(), (l) => (data.practice = l));
}

export function requestFor(v: string | undefined): PracticeRequest {
  // `next` opens exactly what the adaptive recommendation names (its follow-up for a lesson).
  return v === 'sample' ? { kind: 'sample' } : { kind: 'next' };
}

export async function startPractice(req: PracticeRequest): Promise<void> {
  if (busyRefused()) return;
  closeLayer();
  const item = await write((a) => a.startPractice(req));
  if (!item) return;
  data.practice = ready(item);
  data.feedback = null;
  ui.practice = { sel: null, marks: [] };
  if (ui.route === 'practice') render(true, !ui.shot);
  else go('practice');
}

async function answered(fbPromise: (a: StudyApi, token: string) => Promise<Feedback>): Promise<void> {
  const item = practiceItem();
  // A second submit while the first is in flight is refused here, before the failure path below.
  if (!item || busyRefused()) return;
  const fb = await write((a) => fbPromise(a, item.token));
  if (!fb) {
    // The core refused: a closed or changed item cannot be answered, so show its current state.
    data.practice = idle();
    void loadPracticeResume();
    return;
  }
  data.feedback = fb;
  data.practice = ready(null);
  ui.feedback.ev = null;
  ui.coachOpen = true;
  resetCoach();
  void loadToday();
  go('feedback');
}

export function doMark(): boolean {
  const m = takeMark();
  if (!m) return false;
  const item = practiceItem();
  if (!item) return false;
  if (!ui.practice.marks.some((x) => x.p === m.p && x.a < m.b && m.a < x.b)) ui.practice.marks.push(m);
  const box = $('#passage');
  if (!box) return true;
  const top = box.scrollTop;
  const fresh = document.createElement('div');
  fresh.innerHTML = passageHTML(item.passage, { mode: 'practice', marks: ui.practice.marks, foot: practiceFoot(ui.practice.marks.length) }).value;
  box.innerHTML = fresh.querySelector('#passage')?.innerHTML ?? '';
  box.scrollTop = top;
  const foot = box.parentElement?.querySelector('.pas-foot');
  const freshFoot = fresh.querySelector('.pas-foot');
  if (foot && freshFoot) foot.innerHTML = freshFoot.innerHTML;
  const count = $('#markcount');
  if (count) count.textContent = markCountText(ui.practice.marks.length);
  return true;
}

export const practiceActs: Record<string, Act> = {
  pick(v) {
    if (!v) return;
    ui.practice.sel = v;
    setChecked(v);
    const s = $<HTMLButtonElement>('#submit');
    if (s) s.disabled = ui.busy;
  },
  hint() {
    const item = practiceItem();
    if (item?.hint_available && !item.hint) openLayer('hint');
  },
  'hint-yes'() {
    const item = practiceItem();
    if (!item || busyRefused()) return;
    ui.layer = null;
    renderLayer(false);
    void write((a) => a.openHint(item.token)).then((next) => {
      if (!next) {
        render(true, false);
        return;
      }
      data.practice = ready(next);
      // The hint opens in the drawer under the passage; the passage keeps its scroll.
      render(true, false);
      $('#stage')?.classList.add('enter-hint');
      ($('.opt[aria-checked="true"]') ?? $('.opt'))?.focus({ preventScroll: true });
    });
  },
  skip() {
    void answered((a, token) => a.skip(token));
  },
  submit() {
    const sel = ui.practice.sel;
    if (!sel) return;
    void answered((a, token) => a.answer(token, sel));
  },
  next() {
    void startPractice({ kind: 'next' });
  },
  'propose-note'() {
    const fb = data.feedback;
    if (!fb) return;
    const mark = ui.practice.marks.at(-1)?.text;
    void propose(mark ? { attempt_id: fb.attempt_id, mark } : { attempt_id: fb.attempt_id });
  },
  'mark-note'() {
    const item = practiceItem();
    const mark = ui.practice.marks.at(-1)?.text;
    if (!item || !mark) return;
    void propose({ content_id: item.content_id, question_id: item.question_id, mark });
  },
};
