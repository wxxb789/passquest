// The Today room: the learner's exam date and daily time, the practice starts it offers, and the
// brief that can resume or be cancelled (a brief resumed in the background reloads Today).
import { onGoEvent, parseCallError } from '../api/client';
import type { StudioError } from '../api/types';
import { T } from '../core/i18n';
import { closeLayer, openLayer } from '../core/render';
import { toast } from '../core/toast';
import { $, type Act } from './dom';
import { failureLine, loadToday, writeOk } from './service';
import { requestFor, startPractice } from './practice';

/** A brief resumed in the background reports through `brief:changed`: Today reloads. */
export function watchBrief(): void {
  onGoEvent<{ phase: string; error: StudioError | null }>('brief:changed', (e) => {
    if (e.phase === 'finished' && e.error) toast(failureLine(parseCallError(e.error)));
    void loadToday();
  });
}

export const todayActs: Record<string, Act> = {
  learner() {
    openLayer('learner');
  },
  'learner-save'() {
    const form = $<HTMLFormElement>('#learnerform');
    if (!form) return;
    const fd = new FormData(form);
    const date = String(fd.get('exam_date') ?? '').trim();
    const mins = String(fd.get('daily_minutes') ?? '').trim();
    const minutes = mins ? Number(mins) : 0;
    if (!(Number.isInteger(minutes) && minutes >= 0 && minutes <= 1440)) {
      toast(T('每日时长需为 1–1440 的整数分钟', 'Daily time must be a whole number of minutes, 1–1440'));
      return;
    }
    void writeOk((a) => a.setLearnerSettings({ exam_date: date, daily_minutes: minutes })).then((ok) => {
      if (!ok) return;
      closeLayer();
      void loadToday();
    });
  },
  start(v) {
    void startPractice(requestFor(v));
  },
  'start-skill'(v) {
    if (v) void startPractice({ kind: 'skill', skill: v });
  },
  'start-tactic'(v) {
    const [tactic, stage] = (v ?? '').split('|');
    if (tactic) void startPractice({ kind: 'tactic', tactic, stage: stage ?? '' });
  },
  'brief-resume'() {
    void writeOk((a) => a.resumeBrief()).then((ok) => {
      if (!ok) return;
      toast(T('简报已在本地继续', 'The brief resumed locally'));
      closeLayer();
      void loadToday();
    });
  },
  'brief-cancel'() {
    void writeOk((a) => a.cancelBrief()).then((ok) => {
      if (!ok) return;
      closeLayer();
      void loadToday();
    });
  },
};
