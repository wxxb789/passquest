import { existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

type StubMode = "tools" | "delay" | "auth" | "malformed" | "text";

interface ReceivedRequest {
  path: string;
  body: unknown;
}

const KEY = "test-key-not-real";
const MODEL = "smoke-model";

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function sseText(text: string): Response {
  const base = { id: "chatcmpl-smoke", object: "chat.completion.chunk", created: 1, model: MODEL };
  const body = [
    { ...base, choices: [{ index: 0, delta: { role: "assistant", content: text }, finish_reason: null }] },
    { ...base, choices: [{ index: 0, delta: {}, finish_reason: "stop" }] },
  ]
    .map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`)
    .join("");
  return new Response(`${body}data: [DONE]\n\n`, { headers: { "content-type": "text/event-stream" } });
}

function sseTool(index: number, kind: string, title: string, body: string): Response {
  const base = { id: "chatcmpl-smoke", object: "chat.completion.chunk", created: 1, model: MODEL };
  const call = {
    index: 0,
    id: `call_${index}`,
    type: "function",
    function: { name: "record_study_step", arguments: JSON.stringify({ index, kind, title, body }) },
  };
  const chunks = [
    { ...base, choices: [{ index: 0, delta: { role: "assistant", tool_calls: [call] }, finish_reason: null }] },
    { ...base, choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }] },
  ];
  return new Response(`${chunks.map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`).join("")}data: [DONE]\n\n`, {
    headers: { "content-type": "text/event-stream" },
  });
}

export const agentSmoke: Gate = {
  id: "agent-smoke",
  title: "agent smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-agent-smoke-"));
    const binary = path.join(temp, binaryName);
    const checks: string[] = [];
    const seenOutputs: string[] = [];
    let assertions = 0;
    let permissionChecks = 0;
    let permissionsSkipped = false;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };

    const nodeCheck = await run(executable("node"), ["--version"], { cwd: ctx.root });
    if (nodeCheck.code !== 0) {
      rmSync(temp, { recursive: true, force: true });
      return fail([`node is required for the agent smoke gate: ${nodeCheck.stderr || nodeCheck.stdout}`]);
    }

    let mode: StubMode = "tools";
    let slowTools = false;
    const received: ReceivedRequest[] = [];
    const server = Bun.serve({
      hostname: "127.0.0.1",
      port: 0,
      async fetch(request) {
        const raw = await request.text();
        let body: unknown = {};
        try {
          body = JSON.parse(raw);
        } catch {
          body = { malformed: true };
        }
        received.push({ path: new URL(request.url).pathname, body });
        if (mode === "auth") {
          return Response.json({ error: { message: "invalid stub key" } }, { status: 401 });
        }
        if (mode === "malformed") {
          return new Response("data: {bad json}\n\n", { headers: { "content-type": "text/event-stream" } });
        }
        if (mode === "delay") {
          await Bun.sleep(15000);
          return sseText("late stub answer");
        }
        if (mode === "text") {
          return sseText("plain stub answer");
        }
        const messages = Array.isArray((body as { messages?: unknown }).messages)
          ? ((body as { messages: { role?: string; content?: unknown }[] }).messages)
          : [];
        if (slowTools) await Bun.sleep(300);
        const promptText = messages.map((message) => String(message.content ?? "")).join("\n");
        const runMatch = promptText.match(/run-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}/);
        const label = runMatch ? runMatch[0].slice(-8) : "unknown";
        const toolText = messages
          .filter((message) => message.role === "tool")
          .map((message) => String(message.content ?? ""))
          .join("\n");
        if (!/step 1/i.test(toolText)) {
          return sseTool(1, "diagnosis", `Step 1 diagnosis ${label}`, "The recorded outcome suggests marking the negation before comparing options.");
        }
        if (!/step 2/i.test(toolText)) {
          return sseTool(2, "tactic", `Step 2 tactic ${label}`, "Mark NOT or EXCEPT first; a word match alone is not evidence.");
        }
        return sseText("Two advisory steps recorded; practise the focused tactic next.");
      },
    });

    const endpoint = `http://127.0.0.1:${server.port}/v1/chat/completions`;
    const hostPath = path.join(ctx.root, "agent", "durable-host.mjs");
    const baseEnv = {
      PASSQUEST_TUTOR_API_KEY: KEY,
      PASSQUEST_AGENT_HOST: hostPath,
    };
    const call = async (args: string[], env: Record<string, string> = {}) => {
      const result = await run(binary, args, { cwd: ctx.root, env: { ...baseEnv, ...env } });
      seenOutputs.push(result.stdout, result.stderr);
      return result;
    };
    const configure = async (db: string) => {
      const result = await call([
        "tutor", "config", "--endpoint", endpoint, "--model", MODEL,
        "--max-output-tokens", "600", "--db", db,
      ]);
      check(result.code === 0, `tutor config failed: ${result.stderr}`);
    };
    const documentOf = (result: { code: number; stdout: string; stderr: string }) =>
      parse(result.code === 0 || result.code === 4 ? result.stdout : result.stderr) as Record<string, unknown>;
    const checkFileExists = (file: string, label: string): boolean => {
      assertions++;
      if (existsSync(file)) return true;
      checks.push(`${label} does not exist`);
      return false;
    };
    const checkStoreMode = (file: string, label: string) => {
      if (!checkFileExists(file, label)) return;
      if (process.platform === "win32") {
        permissionsSkipped = true;
        return;
      }
      permissionChecks++;
      const mode = statSync(file).mode & 0o777;
      check(mode === 0o600, `${label} mode is ${mode.toString(8)}, want 600`);
    };
    const noSecret = (file: string, label: string) => {
      if (!checkFileExists(file, label)) return;
      const bytes = readFileSync(file);
      check(!bytes.includes(Buffer.from(KEY)), `${label} contains the provider credential`);
    };

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      // 1. Fresh run: complete, commit two steps, keep attempts/report intact,
      // and expose the durable history.
      {
        const db = path.join(temp, "app1.sqlite");
        const store = path.join(temp, "agent1.sqlite");
        await configure(db);
        const before = await call(["report", "--db", db, "--json"]);
        mode = "tools";
        const requestsBefore = received.length;
        const started = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        const startDoc = documentOf(started);
        check(started.code === 0, `start exit ${started.code}: ${started.stderr}`);
        check(received.length - requestsBefore === 3, `start made ${received.length - requestsBefore} provider requests, want 3`);
        check(stringField(startDoc, "state") === "completed", "start state is not completed");
        check(arrayField(startDoc, "steps").length === 2, "start did not commit two steps");
        const steps = arrayField(startDoc, "steps");
        check(stringField(steps[0], "kind") === "diagnosis", "first step kind is not diagnosis");
        check(stringField(steps[1], "kind") === "tactic", "second step kind is not tactic");
        check((numberField(field(startDoc, "durable"), "conversation_id") ?? 0) > 0, "no Pi conversation id");
        const after = await call(["report", "--db", db, "--json"]);
        check(before.stdout === after.stdout, "report changed after an agent run");
        const runID = stringField(field(startDoc, "run"), "run_id") ?? "";
        check(/^run-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}$/.test(runID), `run id shape ${runID}`);

        const status = await call(["agent", "status", runID, "--db", db, "--json"]);
        const statusDoc = documentOf(status);
        check(status.code === 0, `status exit ${status.code}: ${status.stderr}`);
        check(stringField(statusDoc, "state") === "completed", "status is not completed");
        check(stringField(statusDoc, "latest_event") === "completed", "status latest event is not completed");
        check(numberField(field(statusDoc, "counts"), "steps") === 2, "status step count is not 2");
        check(field(statusDoc, "durable") !== undefined, "status has no durable view");

        const show = await call(["agent", "show", runID, "--db", db, "--json"]);
        const showDoc = documentOf(show);
        const effects = field(showDoc, "effects");
        check(stringField(showDoc, "state") === "run", "show state is not run");
        check(numberField(effects, "steps") === 2, "show effects steps is not 2");
        check(numberField(effects, "attempts") === 2, "show effects attempts is not 2");
        check(numberField(effects, "duplicates") === 0, "show effects duplicates is not 0");

        const transcript = await call(["agent", "transcript", runID, "--agent-store", store, "--db", db, "--json"]);
        const transcriptDoc = documentOf(transcript);
        const entries = arrayField(transcriptDoc, "entries");
        const text = entries.map((entry) => stringField(entry, "text") ?? "").join("\n");
        check(transcript.code === 0, `transcript exit ${transcript.code}: ${transcript.stderr}`);
        check(Boolean(field(transcriptDoc, "found")), "transcript did not find the run");
        check(entries.some((entry) => stringField(entry, "kind") === "app.agent-task"), "transcript lost the task entry");
        check(text.includes("Recorded step 1") && text.includes("Recorded step 2"), "transcript lost the tool results");
        check(text.includes("Two advisory steps recorded"), "transcript lost the final answer");
        check(!text.includes("data/baselines") && !text.includes("data/private"), "transcript contains a private path");
        checkStoreMode(store, "Pi store");
        noSecret(store, "Pi store");
        noSecret(db, "application database");

        const copy = path.join(temp, "agent1-copy.sqlite");
        const missingDB = path.join(temp, "missing", "app.sqlite");
        if (existsSync(store)) await Bun.write(copy, readFileSync(store));
        const copied = await call(["agent", "transcript", runID, "--agent-store", copy, "--db", missingDB, "--json"]);
        const copiedDoc = documentOf(copied);
        check(copied.code === 0, `copied transcript exit ${copied.code}: ${copied.stderr}`);
        check(Boolean(field(copiedDoc, "found")), "copied transcript did not find the run");
        check(arrayField(copiedDoc, "entries").length === entries.length, "copied transcript entry count differs");
        check(!existsSync(missingDB), "transcript created the application database");
      }

      // 2. Timeout pauses resumable work; a second start is refused; resume
      // completes with the same run and a coherent transcript.
      {
        const db = path.join(temp, "app2.sqlite");
        const store = path.join(temp, "agent2.sqlite");
        await configure(db);
        mode = "delay";
        const paused = await call(["agent", "start", "--db", db, "--agent-store", store, "--timeout-seconds", "1", "--json"]);
        const pausedDoc = documentOf(paused);
        check(paused.code === 4, `paused start exit ${paused.code}: ${paused.stderr}`);
        check(stringField(pausedDoc, "state") === "paused", "timed-out start is not paused");
        const runID = stringField(field(pausedDoc, "run"), "run_id") ?? "";
        const statusPaused = await call(["agent", "status", runID, "--db", db, "--json"]);
        check(stringField(documentOf(statusPaused), "state") === "paused", "status did not report paused");
        const busy = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        check(busy.code === 2, `second start while paused exit ${busy.code}`);
        check(stringField(field(documentOf(busy), "error"), "code") === "agent_busy", "second start is not agent_busy");
        mode = "tools";
        const resumed = await call(["agent", "resume", runID, "--db", db, "--timeout-seconds", "60", "--json"]);
        const resumedDoc = documentOf(resumed);
        check(resumed.code === 0, `resume exit ${resumed.code}: ${resumed.stderr}`);
        check(stringField(resumedDoc, "state") === "completed", "resume did not complete");
        check(stringField(field(resumedDoc, "run"), "run_id") === runID, "resume changed the run id");
        check(arrayField(resumedDoc, "steps").length === 2, "resume did not commit two steps");
        const transcript = await call(["agent", "transcript", runID, "--agent-store", store, "--db", db, "--json"]);
        const transcriptDoc = documentOf(transcript);
        const entries = arrayField(transcriptDoc, "entries");
        check(entries.some((entry) => stringField(entry, "kind") === "app.agent-task"), "resumed transcript lost the task");
        check(
          entries.filter((entry) => stringField(entry, "kind") === "pi.tool-result").length === 2,
          "resumed transcript does not show two tool results",
        );
      }

      // 3. A crash after the application commit reruns the same tool call. The
      // duplicate is recorded, the first text is kept, and no attempt is added.
      {
        const db = path.join(temp, "app3.sqlite");
        const store = path.join(temp, "agent3.sqlite");
        await configure(db);
        mode = "tools";
        const before = await call(["report", "--db", db, "--json"]);
        const crashed = await call(
          ["agent", "start", "--db", db, "--agent-store", store, "--json"],
          { PASSQUEST_AGENT_FAULT: "kill-after-step" },
        );
        const crashedDoc = documentOf(crashed);
        check(crashed.code === 4, `fault start exit ${crashed.code}: ${crashed.stderr}`);
        check(stringField(crashedDoc, "state") === "paused", "fault start is not paused");
        const runID = stringField(field(crashedDoc, "run"), "run_id") ?? "";
        const beforeResume = documentOf(await call(["agent", "show", runID, "--db", db, "--json"]));
        check(numberField(field(beforeResume, "effects"), "steps") === 1, "fault did not leave one committed step");
        check(numberField(field(beforeResume, "effects"), "attempts") === 1, "fault attempt ledger is not 1");
        const resumed = await call(["agent", "resume", runID, "--db", db, "--timeout-seconds", "60", "--json"]);
        check(resumed.code === 0, `fault resume exit ${resumed.code}: ${resumed.stderr}`);
        const afterResume = documentOf(await call(["agent", "show", runID, "--db", db, "--json"]));
        const effects = field(afterResume, "effects");
        check(numberField(effects, "steps") === 2, "reconciliation steps is not 2");
        check(numberField(effects, "attempts") === 3, "reconciliation attempts is not 3");
        check(numberField(effects, "duplicates") === 1, "reconciliation duplicates is not 1");
        check(numberField(effects, "reconciled_events") === 1, "reconciled event count is not 1");
        const after = await call(["report", "--db", db, "--json"]);
        check(before.stdout === after.stdout, "report changed after a retried effect");
        const steps = arrayField(afterResume, "steps");
        check(steps.length === 2, "reconciliation step rows are not 2");
        check(
          steps.every((step) => (stringField(step, "title") ?? "").length > 0 && Boolean(field(step, "advisory"))),
          "a step is not advisory",
        );
      }

      // 4. Cancel a running host from a second process; a resume beside the
      // live host is refused; the start process reports cancelled and resume
      // refuses afterwards.
      {
        const db = path.join(temp, "app4.sqlite");
        const store = path.join(temp, "agent4.sqlite");
        await configure(db);
        mode = "delay";
        const startPromise = call([
          "agent", "start", "--db", db, "--agent-store", store, "--timeout-seconds", "60", "--json",
        ]);
        let runID = "";
        let running = false;
        for (let attempt = 0; attempt < 80; attempt++) {
          await Bun.sleep(100);
          const runs = await call(["agent", "runs", "--db", db, "--json"]);
          const list = arrayField(documentOf(runs), "runs");
          if (list.length === 0) continue;
          runID = stringField(list[0], "run_id") ?? "";
          const status = await call(["agent", "status", runID, "--db", db, "--json"]);
          if (stringField(documentOf(status), "state") === "running") {
            running = true;
            check((numberField(field(documentOf(status), "durable"), "heartbeat_pid") ?? 0) > 0, "running status has no host pid");
            check(Boolean(field(field(documentOf(status), "durable"), "heartbeat_fresh")), "running status heartbeat is not fresh");
            checkStoreMode(`${store}.host`, "host heartbeat");
            break;
          }
        }
        check(running, "run never reported running");
        const busyResume = await call(["agent", "resume", runID, "--db", db, "--timeout-seconds", "5", "--json"]);
        check(busyResume.code === 2, `resume beside a live host exit ${busyResume.code}: ${busyResume.stderr}`);
        check(
          stringField(field(documentOf(busyResume), "error"), "code") === "run_running",
          "resume beside a live host is not run_running",
        );
        const stillRunning = documentOf(await call(["agent", "status", runID, "--db", db, "--json"]));
        check(stringField(stillRunning, "state") === "running", "resume beside a live host changed the state");
        const cancelled = await call(["agent", "cancel", runID, "--db", db, "--json"]);
        const cancelDoc = documentOf(cancelled);
        check(cancelled.code === 0, `cancel exit ${cancelled.code}: ${cancelled.stderr}`);
        check(Boolean(field(cancelDoc, "confirmed")), "cancel was not confirmed");
        const started = await startPromise;
        const startDoc = documentOf(started);
        check(started.code === 0, `cancelled start exit ${started.code}: ${started.stderr}`);
        check(stringField(startDoc, "state") === "cancelled", "cancelled start state is not cancelled");
        const status = documentOf(await call(["agent", "status", runID, "--db", db, "--json"]));
        check(stringField(status, "state") === "cancelled", "status after cancel is not cancelled");
        const resumed = await call(["agent", "resume", runID, "--db", db, "--json"]);
        check(resumed.code === 2, `resume after cancel exit ${resumed.code}`);
        check(!existsSync(`${store}.host`), "heartbeat file survived cancel");
      }

      // 5. Provider auth failure needs action; a fixed stub then resumes to a
      // completed run.
      {
        const db = path.join(temp, "app5.sqlite");
        const store = path.join(temp, "agent5.sqlite");
        await configure(db);
        mode = "auth";
        const failed = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        const failure = documentOf(failed);
        check(failed.code === 3, `auth start exit ${failed.code}`);
        check(stringField(field(failure, "error"), "code") === "auth_failed", `auth code ${stringField(field(failure, "error"), "code")}`);
        check(stringField(field(failure, "status"), "state") === "needs_action", "auth state is not needs_action");
        const runID = stringField(field(failure, "run"), "run_id") ?? "";
        const status = documentOf(await call(["agent", "status", runID, "--db", db, "--json"]));
        check(stringField(status, "state") === "needs_action", "status after auth failure is not needs_action");
        mode = "tools";
        const resumed = await call(["agent", "resume", runID, "--db", db, "--timeout-seconds", "60", "--json"]);
        check(resumed.code === 0, `resume after auth failure exit ${resumed.code}: ${resumed.stderr}`);
        check(arrayField(documentOf(resumed), "steps").length === 2, "resume after auth did not commit two steps");
      }

      // 6. A malformed provider response is a terminal failure, not a pause.
      {
        const db = path.join(temp, "app6.sqlite");
        const store = path.join(temp, "agent6.sqlite");
        await configure(db);
        mode = "malformed";
        const failed = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        const failure = documentOf(failed);
        check(failed.code === 3, `malformed start exit ${failed.code}`);
        check(stringField(field(failure, "error"), "code") === "invalid_response", `malformed code ${stringField(field(failure, "error"), "code")}`);
        const runID = stringField(field(failure, "run"), "run_id") ?? "";
        const status = documentOf(await call(["agent", "status", runID, "--db", db, "--json"]));
        check(stringField(status, "state") === "failed", "status after malformed response is not failed");
      }

      // 7. The optional host is absent: the agent says so and practice still
      // records and grades.
      {
        const db = path.join(temp, "app7.sqlite");
        const store = path.join(temp, "agent7.sqlite");
        await configure(db);
        const missingNode = path.join(temp, "no-node-here");
        const started = await call(
          ["agent", "start", "--db", db, "--agent-store", store, "--json"],
          { PASSQUEST_NODE: missingNode },
        );
        const failure = documentOf(started);
        check(started.code === 3, `no-node start exit ${started.code}`);
        check(stringField(field(failure, "error"), "code") === "agent_unavailable", "no-node code is not agent_unavailable");
        const runID = stringField(field(failure, "run"), "run_id") ?? "";
        const status = await call(["agent", "status", runID, "--db", db, "--json"], { PASSQUEST_NODE: missingNode });
        const statusDoc = documentOf(status);
        check(status.code === 0, `no-node status exit ${status.code}`);
        check(stringField(statusDoc, "state") === "needs_action", "no-node status is not needs_action");
        check(!field(field(statusDoc, "durable"), "available"), "no-node status claims a durable snapshot");
        const practice = await call(["practice", "--answer", "A", "--db", db, "--json"], { PASSQUEST_NODE: missingNode });
        check(practice.code === 0, `practice without node exit ${practice.code}: ${practice.stderr}`);
      }

      // 8. A completed host answer with no committed step is the documented
      // zero-step failure, not a completed run.
      {
        const db = path.join(temp, "app8.sqlite");
        const store = path.join(temp, "agent8.sqlite");
        await configure(db);
        mode = "text";
        const failed = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        const failure = documentOf(failed);
        check(failed.code === 3, `no-step start exit ${failed.code}: ${failed.stderr}`);
        check(
          stringField(field(failure, "error"), "code") === "no_steps",
          `no-step code ${stringField(field(failure, "error"), "code")}`,
        );
        check(stringField(field(failure, "status"), "state") === "failed", "no-step state is not failed");
        const runID = stringField(field(failure, "run"), "run_id") ?? "";
        const status = documentOf(await call(["agent", "status", runID, "--db", db, "--json"]));
        check(stringField(status, "state") === "failed", "status after a no-step answer is not failed");
        const resumed = await call(["agent", "resume", runID, "--db", db, "--json"]);
        check(resumed.code === 2, `resume after a no-step answer exit ${resumed.code}`);
      }

      // 9. Durable counts belong to the requested conversation: a completed
      // run stays completed while a later run on the same store is paused.
      {
        const db = path.join(temp, "app9.sqlite");
        const store = path.join(temp, "agent9.sqlite");
        await configure(db);
        mode = "tools";
        const first = documentOf(await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]));
        const firstID = stringField(field(first, "run"), "run_id") ?? "";
        check(stringField(first, "state") === "completed", "conversation-scoped status: first run did not complete");
        mode = "delay";
        const second = await call(["agent", "start", "--db", db, "--agent-store", store, "--timeout-seconds", "1", "--json"]);
        const secondDoc = documentOf(second);
        const secondID = stringField(field(secondDoc, "run"), "run_id") ?? "";
        check(
          second.code === 4 && stringField(secondDoc, "state") === "paused",
          `conversation-scoped status: second run did not pause (exit ${second.code}: ${second.stderr})`,
        );
        const secondStatus = documentOf(await call(["agent", "status", secondID, "--db", db, "--json"]));
        const secondSnapshot = field(field(secondStatus, "durable"), "snapshot");
        check(stringField(secondStatus, "state") === "paused", "later run's status is not paused");
        check(
          (numberField(secondSnapshot, "pending_tasks") ?? 0) > 0,
          "later run has no pending durable work to contaminate the first",
        );
        const firstStatus = documentOf(await call(["agent", "status", firstID, "--db", db, "--json"]));
        const firstSnapshot = field(field(firstStatus, "durable"), "snapshot");
        check(
          stringField(firstStatus, "state") === "completed",
          "completed run flipped state while another conversation was paused",
        );
        check(
          numberField(firstSnapshot, "pending_tasks") === 0,
          "completed run counted another conversation's pending task",
        );
        check(
          numberField(firstSnapshot, "unsettled_submissions") === 0,
          "completed run counted another conversation's unsettled submission",
        );
      }

      // 10. Work recovered from a cancelled run stays bound to that run: a new
      // run on the same store must not commit the old call under its own id.
      {
        const db = path.join(temp, "app10.sqlite");
        const store = path.join(temp, "agent10.sqlite");
        await configure(db);
        mode = "tools";
        const crashed = await call(
          ["agent", "start", "--db", db, "--agent-store", store, "--json"],
          { PASSQUEST_AGENT_FAULT: "kill-after-step" },
        );
        const crashedDoc = documentOf(crashed);
        const cancelledID = stringField(field(crashedDoc, "run"), "run_id") ?? "";
        check(
          crashed.code === 4 && stringField(crashedDoc, "state") === "paused",
          `recovered-work isolation: fault start did not pause (exit ${crashed.code}: ${crashed.stderr})`,
        );
        const cancelledRun = documentOf(await call(["agent", "show", cancelledID, "--db", db, "--json"]));
        const cancelledSteps = arrayField(cancelledRun, "steps");
        const cancelledTitle = stringField(cancelledSteps[0], "title") ?? "";
        check(
          cancelledTitle.includes(cancelledID.slice(-8)),
          "recovered-work isolation: first run did not record its own step",
        );
        const cancelled = await call(["agent", "cancel", cancelledID, "--db", db, "--json"]);
        check(
          cancelled.code === 0,
          `recovered-work isolation: cancel exit ${cancelled.code}: ${cancelled.stderr}`,
        );
        slowTools = true;
        const started = await call(["agent", "start", "--db", db, "--agent-store", store, "--json"]);
        slowTools = false;
        const startedDoc = documentOf(started);
        const startedID = stringField(field(startedDoc, "run"), "run_id") ?? "";
        check(
          started.code === 0 && stringField(startedDoc, "state") === "completed",
          `recovered-work isolation: second start exit ${started.code}: ${started.stderr}`,
        );
        const startedSteps = arrayField(startedDoc, "steps");
        check(startedSteps.length === 2, "recovered-work isolation: second run did not commit two steps");
        const firstTitle = stringField(startedSteps[0], "title") ?? "";
        check(
          firstTitle.includes(startedID.slice(-8)),
          `recovered-work isolation: step 1 title ${JSON.stringify(firstTitle)} came from the cancelled run`,
        );
        check(
          !startedSteps.some((step) => (stringField(step, "title") ?? "").includes(cancelledID.slice(-8))),
          "recovered-work isolation: cancelled run text leaked into the new run",
        );
      }

      // 11. The host applies Go's loopback exception to a bracketed IPv6
      // endpoint before contacting a provider.
      {
        const store = path.join(temp, "agent11.sqlite");
        const db = path.join(temp, "app11.sqlite");
        writeFileSync(store, "");
        const probe = await run(executable("node"), [hostPath], {
          cwd: ctx.root,
          stdin: JSON.stringify({
            protocol_version: 1,
            mode: "start",
            run_id: "run-20261110T100000Z-55555555",
            task_kind: "study-brief",
            prompt_version: "agent-study-brief-v1",
            store_path: store,
            heartbeat_path: `${store}.host`,
            host_token: "5".repeat(32),
            binary_path: binary,
            db_path: db,
            credential_env: "PASSQUEST_AGENT_SMOKE_UNSET",
            model: {
              endpoint: "http://[::1]:8080/v1/chat/completions",
              model: MODEL,
              timeout_ms: 30000,
              max_output_tokens: 600,
              max_steps: 3,
              max_turns: 8,
            },
            prompt: "config probe",
            deadline_ms: 60000,
          }),
          env: { PASSQUEST_AGENT_SMOKE_UNSET: "" },
        });
        const probeDoc = parse(probe.stdout) as Record<string, unknown>;
        check(probe.code === 0, `bracketed endpoint exit ${probe.code}: ${probe.stderr}`);
        check(
          stringField(probeDoc, "state") === "needs_action",
          `bracketed endpoint state ${stringField(probeDoc, "state")}`,
        );
        check(
          stringField(field(probeDoc, "error"), "code") === "missing_credential",
          `bracketed endpoint code ${stringField(field(probeDoc, "error"), "code")}`,
        );
      }

      check(!seenOutputs.join("\n").includes(KEY), "agent output contains the provider credential");
      if (checks.length > 0) return fail(checks);
      const permissionNote = permissionsSkipped
        ? "permissions skipped on Windows"
        : `${permissionChecks} permission checks`;
      return pass(`${assertions} assertions, 11 scenarios, ${received.length} provider requests, ${permissionNote}`);
    } finally {
      server.stop(true);
      rmSync(temp, { recursive: true, force: true });
    }
  },
};
