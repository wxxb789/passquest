// The assessment room: a run's state as the core reports it, the item it presents, answers, and the
// window attaching to and detaching from the run it drives. A finished run opens its report.
import type { AssessmentState } from '../api/types';
import { T } from '../core/i18n';
import { closeLayer, openLayer, render } from '../core/render';
import { go } from '../core/router';
import { data, idle, ui, valueOf } from '../core/state';
import { toast } from '../core/toast';
import { activeRun, assessState } from '../screens';
import { $, setChecked, type Act } from './dom';
import { busyRefused, call, load, loadToday, ready, write } from './service';

/** Puts a run's state on screen and follows it: a finished run opens its report. */
function showAssessment(s: AssessmentState): void {
  data.assess.state = ready(s);
  ui.assess.sel = null;
  if ((s.state === 'completed' || s.state === 'abandoned') && s.run) {
    ui.assess.attached = false;
    ui.report.runId = s.run.id;
    data.report.report = idle();
    data.report.runs = idle();
    toast(
      s.state === 'completed'
        ? T(`第 ${s.run.id} 轮已完成`, `Run #${s.run.id} is complete`)
        : T(`第 ${s.run.id} 轮已放弃（超过时限）`, `Run #${s.run.id} was abandoned (past its window)`),
    );
    void loadToday();
    go('report');
    return;
  }
  if (ui.route === 'assess') render(true, false);
}

/** Asks the core to present the run's current item; it records any timeout that came due. */
async function presentItem(runId: number): Promise<void> {
  const r = await call((a) => a.presentAssessmentItem(runId));
  if (r.ok) showAssessment(r.value);
  else {
    data.assess.state = { status: 'error', error: r.error };
    render(true, false);
  }
}

export async function refreshAssess(): Promise<void> {
  const s = await load((a) => a.assessmentState(), (l) => (data.assess.state = l), valueOf(data.assess.state));
  if (!s) return;
  if (s.state === 'item' && s.run) {
    if (ui.assess.attached && !s.item) await presentItem(s.run.id);
    return;
  }
  ui.assess.attached = false;
  await load((a) => a.assessmentPool(), (l) => (data.assess.pool = l));
}

/**
 * Leaving a run this window drives, by any path: the window stops driving it, so coming back goes
 * through the resume card and `assess resume` (one resume counted), whichever way the learner left.
 */
export function detachRun(): void {
  const pos = assessState()?.item?.position;
  ui.assess.attached = false;
  ui.assess.leaveTo = null;
  toast(
    pos !== undefined
      ? T(`测评停在第 ${pos + 1} 题 · 计时继续`, `Run left at item ${pos + 1} · the clock keeps running`)
      : T('已离开测评 · 计时继续', 'Left the run · the clock keeps running'),
  );
}

async function answerItem(label: string): Promise<void> {
  const s = assessState();
  const item = s?.item;
  if (!s?.run || !item || busyRefused()) return;
  const res = await write((a) => a.answerAssessmentItem(item.run_id, item.position, label));
  if (!res) {
    void refreshAssess();
    return;
  }
  if (res.outcome === 'timed_out') toast(T(`第 ${item.position + 1} 题已超时，记为超时`, `Item ${item.position + 1} was past its clock and is recorded as timed out`));
  if (res.state.state !== 'item' || !res.state.run) {
    showAssessment(res.state);
    return;
  }
  await presentItem(res.state.run.id);
}

/** Called by the display clock when a server deadline passes: the core decides what happened. */
export function onDeadline(): void {
  const s = assessState();
  if (ui.assess.attached && s?.run && s.state === 'item') void presentItem(s.run.id);
  else if (ui.route === 'assess') void refreshAssess();
}

export const assessActs: Record<string, Act> = {
  'run-ask'() {
    openLayer('start-run');
  },
  'run-start'() {
    const form = $<HTMLFormElement>('#runform');
    if (!form) return;
    const fd = new FormData(form);
    const size = Number(fd.get('size'));
    const minutes = Number(fd.get('minutes'));
    if (!Number.isInteger(size) || size < 1 || !Number.isInteger(minutes) || minutes < 1 || minutes > 1440) {
      toast(T('题数和时长需为正整数', 'Size and minutes must be positive whole numbers'));
      return;
    }
    closeLayer();
    void write((a) => a.startAssessment(size, minutes)).then((s) => {
      if (!s) return;
      ui.assess.attached = true;
      showAssessment(s);
      void loadToday();
    });
  },
  'run-resume'() {
    const run = activeRun();
    if (!run || busyRefused()) return;
    void write((a) => a.resumeAssessment(run.id)).then((s) => {
      if (!s) {
        void refreshAssess();
        return;
      }
      ui.assess.attached = true;
      showAssessment(s);
    });
  },
  'run-abandon-ask'() {
    openLayer('abandon-run');
  },
  'run-abandon'() {
    const run = activeRun();
    closeLayer();
    if (!run) return;
    void write((a) => a.abandonAssessment(run.id)).then((s) => {
      if (s) showAssessment(s);
    });
  },
  apick(v) {
    if (!v) return;
    ui.assess.sel = v;
    setChecked(v);
    const n = $<HTMLButtonElement>('#anext');
    if (n) n.disabled = ui.busy;
  },
  anext() {
    const sel = ui.assess.sel;
    if (sel) void answerItem(sel);
  },
  'askip-item'() {
    void answerItem('skip');
  },
  leave() {
    ui.assess.leaveTo = null;
    openLayer('leave');
  },
  'leave-yes'() {
    const to = ui.assess.leaveTo ?? 'today';
    ui.layer = null;
    detachRun();
    go(to);
  },
};
