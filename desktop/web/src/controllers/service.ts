// The controllers' one path to the core: every StudyApi call goes through `call` (an active
// assessment elsewhere sends the window back to it), reads land in a data slot through `load`, and
// writes run one at a time through `write`. The window never fabricates a result: without a bound
// service every call resolves to `not_connected`.
import { api, NOT_CONNECTED, parseCallError, type StudyApi } from '../api/client';
import type { StudioError } from '../api/types';
import { errorTitle, nextStep, T } from '../core/i18n';
import { render } from '../core/render';
import { go } from '../core/router';
import { data, ui, valueOf, type Loadable } from '../core/state';
import { toast } from '../core/toast';

type Result<T> = { ok: true; value: T } | { ok: false; error: StudioError };

export const ready = <T>(value: T): Loadable<T> => ({ status: 'ready', value });

/**
 * A failure as one toast line in the window's words: the localized title and the localized next step.
 * The core's English message and CLI step are not toast text (failure cards keep them as details).
 */
export const failureLine = (e: StudioError) => {
  const next = nextStep(e.code);
  return next ? `${errorTitle(e.code)} · ${next}` : errorTitle(e.code);
};

export async function call<T>(fn: (a: StudyApi) => Promise<T>): Promise<Result<T>> {
  const a = api();
  if (!a) return { ok: false, error: NOT_CONNECTED };
  try {
    return { ok: true, value: await fn(a) };
  } catch (e) {
    const error = parseCallError(e);
    if (error.code === 'assessment_active' && ui.route !== 'assess') {
      toast(T('测评进行中，先回到测评', 'An assessment is in progress — back to the run'));
      go('assess');
    }
    return { ok: false, error };
  }
}

/** Loads into a slot and re-renders the stage when the route is still the one that asked. */
export async function load<T>(fn: (a: StudyApi) => Promise<T>, set: (l: Loadable<T>) => void, prev?: T): Promise<T | undefined> {
  const route = ui.route;
  set(prev === undefined ? { status: 'loading' } : { status: 'loading', value: prev });
  const r = await call(fn);
  set(r.ok ? ready(r.value) : { status: 'error', error: r.error });
  if (ui.route === route) render(true, false);
  return r.ok ? r.value : undefined;
}

/**
 * True, with a toast, while another write is in flight: the new write is refused (never queued, so
 * nothing runs later against a screen that has moved on). Callers that recover from a failed write
 * check this first, so a refused call is never mistaken for a core failure.
 */
export function busyRefused(): boolean {
  if (!ui.busy) return false;
  toast(T('上一步还在进行，稍后再试', 'Still working on the previous step — try again in a moment'));
  return true;
}

/** A write: one at a time, re-rendering around it. Failures surface as a toast. */
export async function write<T>(fn: (a: StudyApi) => Promise<T>): Promise<T | undefined> {
  if (busyRefused()) return undefined;
  const route = ui.route;
  ui.busy = true;
  render(true, false);
  const r = await call(fn);
  ui.busy = false;
  // The learner moved on while the call ran: the room now shown was rendered busy (disabled).
  if (ui.route !== route) render(true, false);
  if (!r.ok) {
    toast(failureLine(r.error));
    render(true, false);
    return undefined;
  }
  return r.value;
}

/** A write whose result is not needed: resolves true on success. */
export async function writeOk(fn: (a: StudyApi) => Promise<unknown>): Promise<boolean> {
  return (await write(async (a) => {
    await fn(a);
    return true as const;
  })) === true;
}

/** Today carries the budget and the next step every room's writes can move: any of them reloads it. */
export const loadToday = () => load((a) => a.today(), (l) => (data.today = l), valueOf(data.today));
