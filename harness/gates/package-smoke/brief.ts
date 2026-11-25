/**
 * The package smoke's study-brief case (goal 0016 P3): on an installed copy
 * that carries no `node_modules` — the four hosts as bundles and copies plus
 * the pinned Node runtime, and nothing else — the learner's unfinished brief
 * is resumed from the window's Today sheet and completes against the loopback
 * provider stub, and the installed binary as its own CLI reports the same
 * trail: two committed advisory steps, an untouched attempt report, three
 * provider requests.
 *
 * The paused start is deliberate. The installed binary's own CLI starts the
 * run with a PATH that carries no node and with no PASSQUEST_* override set:
 * the binary passes the directory it resolved for its own packaged files into
 * the CLI's composition root (cmd/passquest-desktop), so the durable host runs
 * on the packaged runtime, and the window half below resolves the same files
 * for itself. The stub never answers that run, so only a pause request or the
 * deadline ends it; the pause request is written once the stub has recorded
 * the run's own first provider request (lib/host-control.ts), which is what
 * makes "the pause found durable work" true by construction — the plan's
 * `--timeout-seconds 1` deadline lost that race under full CPU load, and even
 * a 20-second one held only a ~1.2x margin over the same machine's worst
 * observed slowdown (goal 0016 P3 review, 2026-11-20) — and
 * `--timeout-seconds PAUSE_DEADLINE_SECONDS` stays as the bound under which
 * the packaged runtime and host must reach the provider.
 */
import { randomBytes } from "node:crypto";
import { existsSync } from "node:fs";

import { cleanEnv, cliJson, defaultStubReply, pageScript, parse, STUB_CREDENTIAL_ENV, STUB_MODEL } from "../../lib/desktop-page.ts";
import { PAUSE_DEADLINE_SECONDS, durableStorePath, startPausedRun } from "../../lib/host-control.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { tail } from "../../lib/process.ts";
import { briefAnswer, stubMessages } from "../../lib/sse.ts";
import { launch, seedStudyTrail, textEnv } from "../desktop-smoke/scenario.ts";
import type { Scenario, ScenarioRun } from "../desktop-smoke/scenario.ts";

/**
 * The environment the installed binary gets when this case runs its CLI: the
 * launch environment the window gets (no PASSQUEST_* variable of this shell,
 * the stripped PATH, the site's home), plus the stub registry and credential
 * only. `cliJson` replaces rather than layers the environment, so no
 * PASSQUEST_* variable can leak in: the packaged runtime and hosts are what
 * the binary resolves for itself.
 */
function installedEnv(s: ScenarioRun, keyEnv: Record<string, string>): Record<string, string> {
  return {
    ...textEnv(cleanEnv()),
    ...textEnv(s.shared.launchEnv ?? {}),
    PASSQUEST_PROVIDER_REGISTRY: s.shared.registry,
    ...keyEnv,
  };
}

export const brief: Scenario = {
  id: "brief",
  async run(s) {
    const { provider } = s.shared;
    const key = `sk-smoke-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    // siteOf in the package smoke — this case's only caller — names both from
    // the installed tree, whose copy always carries the runtime and
    // agent/durable-host.mjs. A copy that lost either is a broken package, and
    // the case names the file instead of leaving it to a resolution error that
    // would list every candidate directory.
    const runtime = s.shared.runtime ?? "";
    const durableHost = s.shared.durableHost ?? "";
    const absent = !existsSync(runtime) ? "no Node runtime" : !existsSync(durableHost) ? "no agent/durable-host.mjs" : "";
    if (absent !== "") {
      s.check(false, `brief: the installed app carries ${absent}, so its own CLI cannot run the brief`);
      return;
    }
    const env = installedEnv(s, keyEnv);
    const installed = (args: string[]): Promise<{ code: number; stdout: string; stderr: string; doc: unknown }> =>
      cliJson(s.shared.binary, s.database, s.shared.cwd ?? s.shared.ctx.root, args, env, { replaceEnv: true });

    try {
      // The same trail the coach case seeds: recorded attempts make the brief's
      // packet non-empty.
      if (!(await seedStudyTrail(s, keyEnv))) return;

      // 1. The installed copy pauses a brief of its own: its runtime and host run
      // the packaged bundle, and the paused run carries the durable work Today
      // will resume.
      // A held answer: only a stop or the deadline can end this request, so the
      // pause below cannot race an answer, however slow the machine is. The
      // pause is requested once the stub has this run's request; the deadline is
      // a bound, not the trigger (lib/host-control.ts).
      provider.respond = () =>
        new Response(new ReadableStream({ start() {} }), { headers: { "content-type": "text/event-stream" } });
      const requestsBefore = provider.received.length;
      const paused = await startPausedRun(
        () => installed(["agent", "start", "--timeout-seconds", String(PAUSE_DEADLINE_SECONDS)]),
        provider.received,
        durableStorePath(s.database),
      );
      const pausedDoc = paused.doc as Record<string, unknown>;
      if (
        !s.check(
          paused.code === 4 && stringField(pausedDoc, "state") === "paused",
          `brief: the installed run did not pause (exit ${paused.code}: ${tail(paused.stderr, 4)})`,
        )
      ) {
        return;
      }
      const runID = stringField(field(pausedDoc, "run"), "run_id") ?? "";
      s.check(/^run-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}$/.test(runID), `brief: the installed run reported id ${runID}`);
      const pausedStatus = await installed(["agent", "status", runID]);
      const pausedStatusDoc = pausedStatus.doc as Record<string, unknown>;
      s.check(
        stringField(pausedStatusDoc, "state") === "paused",
        `brief: the paused run's status is ${String(stringField(pausedStatusDoc, "state"))}`,
      );
      // The resume's precondition, and the one a pause that lands before the
      // durable submit breaks: Today offers Resume only for a paused run with
      // work outstanding, so the case stops here rather than launching the
      // window at a run it could not resume.
      if (
        !s.check(
          (numberField(field(field(pausedStatusDoc, "durable"), "snapshot"), "pending_tasks") ?? 0) > 0,
          "brief: the paused run left no pending durable work for the window to resume",
        )
      ) {
        return;
      }
      s.check(
        provider.received.length - requestsBefore === 1,
        `brief: the installed run made ${provider.received.length - requestsBefore} provider request(s) before its pause, want 1`,
      );
      const reportBefore = await installed(["report"]);
      if (!s.check(reportBefore.code === 0, `brief: report exit = ${reportBefore.code}: ${tail(reportBefore.stderr, 4)}`)) return;

      // 2. The window resumes it with no pin at all: the app finds the packaged
      // runtime and the bundled host by itself, and the stub answers step 1,
      // step 2 and the final text.
      const label = runID.slice(-8);
      provider.respond = (request) => briefAnswer(stubMessages(parse(request.raw)), label, STUB_MODEL);
      const resumeFrom = provider.received.length;
      const launched = await launch(s, {
        label: "brief",
        env: keyEnv,
        script: pageScript(`
await today();
const satellite = () => {
  const button = q('[data-act="sheet"][data-v="brief"]');
  return { flag: button?.querySelector('.flag')?.className ?? null, line: (button?.querySelector('.vl')?.textContent ?? '').trim() };
};
const sheetState = () => (q('#layer .sheet .sh-b .kv b')?.textContent ?? '').trim() || null;
const briefSatellite = satellite();
// The sheet of the paused run: the state row, Resume and Cancel.
await click('[data-act="sheet"][data-v="brief"]');
const resume = q('#layer [data-act="brief-resume"]');
const before = {
  state: sheetState(),
  resume: resume !== null,
  resumeDisabled: resume?.disabled === true,
  cancel: q('#layer [data-act="brief-cancel"]') !== null,
};
const events = [];
const off = window.mygo.on("brief:changed", (payload) => events.push(payload));
await click('[data-act="brief-resume"]');
const finished = await until("the brief to finish", () => events.find((event) => event && event.phase === "finished"), 45000);
off();
// Today reloads from brief:changed; a finished brief is no unfinished brief,
// so the satellite's line and the sheet's actions both change.
await until("Today to reload after the brief", () => satellite().line !== briefSatellite.line, 20000);
const afterSatellite = satellite();
await click('[data-act="sheet"][data-v="brief"]');
const after = {
  state: sheetState(),
  resume: q('#layer [data-act="brief-resume"]') !== null,
  cancel: q('#layer [data-act="brief-cancel"]') !== null,
};
return {
  satellite: { before: briefSatellite, after: afterSatellite },
  before,
  finished: { phase: finished.phase, error: finished.error ?? null, status: finished.status ?? null },
  after,
  phases: events.map((event) => event.phase),
};
`),
      });
      if (!launched) return;
      const value = launched.value;

      // 3. The window's resume: exactly three requests on the one path, the two
      // steps the stub asked for threaded back as tool results, the event's own
      // status, and a Today sheet that no longer offers the run.
      const resumed = provider.received.slice(resumeFrom);
      s.check(resumed.length === 3, `brief: the window's resume made ${resumed.length} provider request(s), want 3`);
      s.check(
        resumed.every((request) => request.path === "/v1/chat/completions"),
        `brief: the resume's paths are ${resumed.map((request) => request.path).join(", ")}`,
      );
      s.check((resumed[1]?.raw ?? "").includes("Recorded step 1"), "brief: the second request carries no result of the first step's tool call");
      s.check((resumed[2]?.raw ?? "").includes("Recorded step 2"), "brief: the third request carries no result of the second step's tool call");
      const before = field(value, "before");
      s.check(field(before, "resume") === true, "brief: Today's brief sheet offers no Resume for the paused run");
      s.check(field(before, "resumeDisabled") === false, "brief: the sheet disables Resume, so the window reports the packaged runtime unavailable");
      s.check(field(before, "cancel") === true, "brief: the paused brief sheet offers no Cancel");
      s.check(
        (stringField(before, "state") ?? "").includes("0 / 3"),
        `brief: the sheet shows ${JSON.stringify(stringField(before, "state"))}, not the paused run at step 0 of 3`,
      );
      const satellite = field(value, "satellite");
      s.check(
        (stringField(field(satellite, "before"), "flag") ?? "").includes("warm"),
        "brief: Today's brief satellite was not flagged as a paused run",
      );
      const finished = field(value, "finished");
      s.check(field(finished, "error") === null, `brief: brief:changed reported an error: ${JSON.stringify(field(finished, "error"))}`);
      const finishedStatus = field(finished, "status");
      s.check(
        stringField(finishedStatus, "state") === "completed",
        `brief: brief:changed finished in state ${String(stringField(finishedStatus, "state"))}`,
      );
      s.check(arrayField(finishedStatus, "steps").length === 2, "brief: the finished brief committed no two steps");
      s.check(field(finishedStatus, "live") === false, "brief: the finished brief still reports a live host");
      s.check((stringField(finishedStatus, "answer") ?? "").length > 0, "brief: the finished brief carried no answer");
      s.check(
        arrayField(value, "phases").join(",") === "started,finished",
        `brief: brief:changed phases are ${arrayField(value, "phases").join(",")}`,
      );
      s.check(field(field(satellite, "after"), "flag") === null, "brief: Today still flags a paused brief after the run finished");
      const after = field(value, "after");
      s.check(field(after, "state") === null, `brief: the finished brief still shows a state row (${JSON.stringify(field(after, "state"))})`);
      s.check(field(after, "resume") === false && field(after, "cancel") === false, "brief: the finished brief still offers Resume or Cancel");

      // 4. The installed binary reads the trail back: the same run, completed,
      // two advisory steps, no pending work, and an attempt report the brief
      // never touched.
      const statusAfter = await installed(["agent", "status", runID]);
      const afterDoc = statusAfter.doc as Record<string, unknown>;
      s.check(stringField(afterDoc, "state") === "completed", `brief: status after the resume is ${String(stringField(afterDoc, "state"))}`);
      s.check(stringField(afterDoc, "latest_event") === "completed", "brief: the run's latest event is not completed");
      s.check(numberField(field(afterDoc, "counts"), "steps") === 2, "brief: the run did not commit two steps");
      const snapshot = field(field(afterDoc, "durable"), "snapshot");
      s.check(numberField(snapshot, "pending_tasks") === 0, "brief: the completed run still has pending durable work");
      s.check(numberField(snapshot, "unsettled_submissions") === 0, "brief: the completed run has unsettled submissions");
      const recorded = arrayField(afterDoc, "steps");
      s.check(
        recorded.map((step) => stringField(step, "kind")).join(",") === "diagnosis,tactic",
        `brief: the recorded steps are ${recorded.map((step) => String(stringField(step, "kind"))).join(",")}, want diagnosis,tactic`,
      );
      s.check(
        recorded.every((step) => field(step, "advisory") === true && (stringField(step, "title") ?? "").length > 0),
        "brief: a recorded step is not advisory or has no title",
      );
      const reportAfter = await installed(["report"]);
      s.check(reportAfter.stdout === reportBefore.stdout, "brief: the resume changed report --json");
    } finally {
      provider.respond = defaultStubReply;
    }
  },
};
