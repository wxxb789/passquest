// The report room: a finished run's report, the run it is compared with, and the per-item sheet.
import { renderLayer } from '../core/render';
import { data, idle, ui, valueOf } from '../core/state';
import { $, type Act } from './dom';
import { load } from './service';

export async function loadReport(): Promise<void> {
  const runs = await load((a) => a.assessmentRuns(), (l) => (data.report.runs = l));
  if (!runs) return;
  const finished = runs.filter((r) => r.status !== 'active');
  const id = ui.report.runId ?? (finished.find((r) => r.status === 'completed') ?? finished[0])?.id ?? null;
  if (id === null) return;
  ui.report.runId = id;
  const current = valueOf(data.report.report);
  if (!current || current.run.id !== id) await load((a) => a.assessmentReport(id), (l) => (data.report.report = l));
  // By default the shown run is compared with the newest earlier completed run of the same shape
  // (size, window, pool family), else with the newest earlier one, whose refusal says what differs.
  if (ui.report.compareWith === null) {
    const base = runs.find((r) => r.id === id);
    const earlier = runs.filter((r) => r.id !== id && r.status === 'completed' && (!base || r.started_at < base.started_at));
    const like = base ? earlier.find((r) => r.actual_size === base.actual_size && r.window_minutes === base.window_minutes && r.pool_family === base.pool_family) : undefined;
    ui.report.compareWith = (like ?? earlier[0])?.id ?? null;
  }
  if (ui.report.compareWith !== null) await loadCompare(ui.report.compareWith);
}

/** Compares the other run (first) with the shown one (second): changes read as shown − other. */
async function loadCompare(other: number): Promise<void> {
  const base = ui.report.runId;
  if (base === null) return;
  await load((a) => a.assessmentCompare(other, base), (l) => (data.report.compare = l));
}

export const reportActs: Record<string, Act> = {
  cmp(v) {
    const id = Number(v);
    if (!Number.isFinite(id)) return;
    ui.report.compareWith = id;
    void loadCompare(id);
  },
  'report-run'(v) {
    const id = Number(v);
    if (!Number.isFinite(id)) return;
    ui.report.runId = id;
    ui.report.compareWith = null;
    data.report.compare = idle();
    data.report.report = idle();
    void loadReport();
  },
  ann(v) {
    ui.report.annItem = Number(v);
    ui.layer = 'sheet:ann';
    renderLayer(false);
    $(`#layer [data-act="ann"][data-v="${v}"]`)?.focus({ preventScroll: true });
  },
};
