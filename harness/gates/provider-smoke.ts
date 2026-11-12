import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

type ChatMode = "ok" | "auth" | "rate" | "missing" | "malformed" | "error" | "delay";

interface ReceivedCall {
  path: string;
  authorization: string;
  body: unknown;
}

const KEY = `sk-${"P".repeat(32)}`;

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

async function chatResponse(mode: ChatMode, model: string): Promise<Response> {
  switch (mode) {
    case "auth":
      return Response.json({ error: { message: "invalid stub key" } }, { status: 401 });
    case "rate":
      return Response.json({ error: { message: "rate limit reached" } }, { status: 429 });
    case "missing":
      return Response.json({ error: { message: `model ${model} not found` } }, { status: 404 });
    case "malformed":
      return Response.json({ choices: [] }, { status: 200 });
    case "error":
      return Response.json({ error: { message: "stub provider failure" } }, { status: 500 });
    case "delay":
      await Bun.sleep(15000);
      break;
  }
  return Response.json({
    choices: [{ message: { content: "OK" } }],
    usage: { prompt_tokens: 3, completion_tokens: 1 },
  });
}

function sseText(text: string, model: string): Response {
  const base = { id: "chatcmpl-provider-smoke", object: "chat.completion.chunk", created: 1, model };
  const body = [
    { ...base, choices: [{ index: 0, delta: { role: "assistant", content: text }, finish_reason: null }] },
    { ...base, choices: [{ index: 0, delta: {}, finish_reason: "stop" }] },
  ]
    .map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`)
    .join("");
  return new Response(`${body}data: [DONE]\n\n`, { headers: { "content-type": "text/event-stream" } });
}

function sseTool(index: number, kind: string, title: string, body: string, model: string): Response {
  const base = { id: "chatcmpl-provider-smoke", object: "chat.completion.chunk", created: 1, model };
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

function agentSse(body: unknown, model: string): Response {
  const messages = Array.isArray(asRecord(body).messages)
    ? (asRecord(body).messages as { role?: string; content?: unknown }[])
    : [];
  const toolText = messages
    .filter((message) => message.role === "tool")
    .map((message) => String(message.content ?? ""))
    .join("\n");
  if (!/step 1/i.test(toolText)) {
    return sseTool(1, "diagnosis", "Provider smoke step 1", "Mark the polarity cue before comparing options.", model);
  }
  if (!/step 2/i.test(toolText)) {
    return sseTool(2, "tactic", "Provider smoke step 2", "A word match alone is not evidence; check the negation.", model);
  }
  return sseText("Two advisory steps recorded.", model);
}

function providerDoc(id: string, endpoint: string, models: Array<Record<string, unknown>>) {
  return {
    id,
    name: id.toUpperCase(),
    endpoint,
    endpoint_shape: "openai-chat-completions",
    credential_env: `PASSQUEST_${id.toUpperCase()}_API_KEY`,
    models,
  };
}

const customProvider = {
  id: "custom",
  name: "Custom compatible endpoint",
  endpoint: "",
  endpoint_shape: "openai-chat-completions",
  credential_env: "PASSQUEST_CUSTOM_API_KEY",
  custom: true,
  models: [],
};

function writeRegistry(file: string, providers: Array<Record<string, unknown>>): void {
  writeFileSync(file, JSON.stringify({ version: 1, providers }, null, 2));
}

function pageFixture(dir: string): void {
  mkdirSync(path.join(dir, "questions"), { recursive: true });
  writeFileSync(
    path.join(dir, "page-001.txt"),
    "The library closes at 4 p.m. on Saturdays. It opens at 9 a.m. on weekdays.\n",
  );
  writeFileSync(
    path.join(dir, "questions", "q1.json"),
    JSON.stringify({
      id: "q1",
      stem: "When does the library close on Saturdays?",
      options: [
        { label: "A", text: "At 9 a.m." },
        { label: "B", text: "At 4 p.m." },
      ],
      answer: "B",
      evidence: "closes at 4 p.m. on Saturdays",
      reasoning: "The notice states the Saturday closing time.",
      tactics: ["not-except-polarity"],
    }),
  );
}

export const providerSmoke: Gate = {
  id: "provider-smoke",
  title: "provider smoke",
  async run(ctx: GateContext) {
    const binaryName = executable("passquest");
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-provider-smoke-"));
    const binary = path.join(temp, binaryName);
    const checks: string[] = [];
    const seenOutputs: string[] = [];
    let assertions = 0;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };

    const nodeCheck = await run(executable("node"), ["--version"], { cwd: ctx.root });
    if (nodeCheck.code !== 0) {
      rmSync(temp, { recursive: true, force: true });
      return fail([`node is required for the provider smoke gate: ${nodeCheck.stderr || nodeCheck.stdout}`]);
    }

    let alphaChat: ChatMode = "ok";
    let betaChat: ChatMode = "ok";
    let alphaAgent: "delay" | "tools" = "tools";
    const alphaCalls: ReceivedCall[] = [];
    const betaCalls: ReceivedCall[] = [];

    const server = Bun.serve({
      hostname: "127.0.0.1",
      port: 0,
      async fetch(request) {
        const raw = await request.text();
        let body: unknown = {};
        try {
          body = JSON.parse(raw);
        } catch {
          body = {};
        }
        const url = new URL(request.url);
        const call = { path: url.pathname, authorization: request.headers.get("authorization") ?? "", body };
        if (url.pathname.startsWith("/alpha/")) {
          alphaCalls.push(call);
          if (asRecord(body).stream === true) {
            if (alphaAgent === "delay") await Bun.sleep(15000);
            return agentSse(body, "alpha-text");
          }
          return chatResponse(alphaChat, "alpha-text");
        }
        if (url.pathname.startsWith("/beta/")) {
          betaCalls.push(call);
          return chatResponse(betaChat, "beta-text");
        }
        return new Response("not found", { status: 404 });
      },
    });
    const base = `http://127.0.0.1:${server.port}`;

    const deadServer = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("dead") });
    const deadPort = deadServer.port;
    deadServer.stop(true);

    const registry = path.join(temp, "registry.json");
    const alpha = providerDoc("alpha", `${base}/alpha/chat/completions`, [
      { id: "alpha-text", default: true, capabilities: ["text.chat"] },
      { id: "alpha-opt", capabilities: ["text.chat", "search", "speech.synthesis"] },
    ]);
    const beta = providerDoc("beta", `${base}/beta/chat/completions`, [
      { id: "beta-text", default: true, capabilities: ["text.chat"] },
    ]);
    const dead = providerDoc("dead", `http://127.0.0.1:${deadPort}/chat/completions`, [
      { id: "dead-text", default: true, capabilities: ["text.chat"] },
    ]);
    writeRegistry(registry, [alpha, beta, dead, customProvider]);

    const budgetRegistry = path.join(temp, "budget-registry.json");
    writeRegistry(budgetRegistry, [alpha, beta, customProvider]);

    const baseEnv = {
      PASSQUEST_PROVIDER_REGISTRY: registry,
      PASSQUEST_ALPHA_API_KEY: "",
      PASSQUEST_BETA_API_KEY: "",
      PASSQUEST_DEAD_API_KEY: "",
      PASSQUEST_CUSTOM_API_KEY: "",
      PASSQUEST_TUTOR_API_KEY: "",
      PASSQUEST_AGENT_HOST: path.join(ctx.root, "agent", "durable-host.mjs"),
    };
    const call = async (args: string[], env: Record<string, string> = {}) => {
      const result = await run(binary, args, { cwd: ctx.root, env: { ...baseEnv, ...env } });
      seenOutputs.push(result.stdout, result.stderr);
      return result;
    };
    const callJSON = async (db: string, args: string[], env: Record<string, string> = {}) => {
      const result = await call([...args, "--db", db, "--json"], env);
      return { ...result, doc: parse(result.stdout), failure: parse(result.stderr) };
    };
    const copy = (from: string, name: string): string => {
      const target = path.join(temp, `${name}.sqlite`);
      copyFileSync(from, target);
      return target;
    };

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      // 1. Registry drives list and separate axes.
      const db = path.join(temp, "main.sqlite");
      const listNoKey = await callJSON(db, ["model", "list"]);
      if (!check(listNoKey.code === 0, `model list exit = ${listNoKey.code}: ${listNoKey.stderr}`)) return fail(checks);
      const providers = arrayField(listNoKey.doc, "providers");
      const providerByID = (id: string) => providers.find((item) => stringField(item, "id") === id);
      for (const id of ["alpha", "beta", "dead"]) {
        const provider = asRecord(providerByID(id));
        check(stringField(provider, "auth") === "no_credential", `${id} does not print no_credential without a key`);
        check(arrayField(provider, "models").length >= 1, `${id} model rows are missing without a key`);
      }
      const alphaListed = asRecord(providerByID("alpha"));
      const alphaText = arrayField(alphaListed, "models").find((m) => stringField(m, "id") === "alpha-text");
      const alphaCaps = arrayField(asRecord(alphaText), "capabilities");
      const textChat = alphaCaps.find((c) => stringField(c, "id") === "text.chat");
      check(stringField(textChat, "state") === "declared" && asRecord(textChat).verified !== true, "text.chat is not declared,unverified");
      const alphaOpt = asRecord(arrayField(alphaListed, "models").find((m) => stringField(m, "id") === "alpha-opt"));
      for (const id of ["search", "speech.synthesis"]) {
        const capability = asRecord(arrayField(alphaOpt, "capabilities").find((c) => stringField(c, "id") === id));
        check(stringField(capability, "state") === "declared" && asRecord(capability).verified !== true, `${id} is not declared,unverified`);
      }

      // The registry is data: a new entry appears without a code change.
      writeRegistry(registry, [
        alpha,
        beta,
        dead,
        customProvider,
        providerDoc("gamma", `${base}/alpha/chat/completions`, [
          { id: "gamma-text", default: true, capabilities: ["text.chat"] },
        ]),
      ]);
      const listGamma = await callJSON(db, ["model", "list"]);
      check(
        arrayField(listGamma.doc, "providers").some((item) => stringField(item, "id") === "gamma"),
        "an added registry entry did not appear",
      );

      // 2. Switching leaves practice state identical.
      const seeded = await callJSON(db, ["practice", "--answer", "B"]);
      if (!check(seeded.code === 0, `seed practice exit = ${seeded.code}: ${seeded.stderr}`)) return fail(checks);
      const beforeDB = copy(db, "before-switch");
      const afterDB = copy(db, "after-switch");
      const nextBefore = await callJSON(beforeDB, ["next"]);
      const reportBefore = await callJSON(beforeDB, ["report"]);
      const use = await callJSON(afterDB, ["model", "use", "alpha/alpha-text"]);
      if (!check(use.code === 0, `model use exit = ${use.code}: ${use.stderr}`)) return fail(checks);
      check(field(field(use.doc, "active"), "provider") === "alpha", "model use did not report alpha");
      check(field(field(use.doc, "active"), "model") === "alpha-text", "model use did not report alpha-text");
      const nextAfter = await callJSON(afterDB, ["next"]);
      const reportAfter = await callJSON(afterDB, ["report"]);
      const normalized = (raw: unknown): string =>
        JSON.stringify(raw).split(beforeDB).join("<db>").split(afterDB).join("<db>");
      check(normalized(nextBefore.doc) === normalized(nextAfter.doc), "model use changed next output");
      check(normalized(reportBefore.doc) === normalized(reportAfter.doc), "model use changed report output");
      const listActive = await callJSON(afterDB, ["model", "list"]);
      check(field(field(listActive.doc, "active"), "provider") === "alpha", "model list does not show the active provider");

      // 3. Verification is a real call with a timestamp.
      const alphaEnv = { PASSQUEST_ALPHA_API_KEY: KEY };
      alphaChat = "ok";
      const verified = await callJSON(db, ["model", "test", "alpha/alpha-text"], alphaEnv);
      if (!check(verified.code === 0, `verified test exit = ${verified.code}: ${verified.stderr}`)) return fail(checks);
      check(stringField(verified.doc, "result") === "verified", "loopback test did not record verified");
      check(stringField(verified.doc, "checked_at") !== "", "verified test has no timestamp");
      const testList = await callJSON(db, ["model", "list"], alphaEnv);
      const testedAlpha = arrayField(asRecord(arrayField(testList.doc, "providers").find((p) => stringField(p, "id") === "alpha")), "models").find(
        (m) => stringField(m, "id") === "alpha-text",
      );
      const testedChat = arrayField(asRecord(testedAlpha), "capabilities").find((c) => stringField(c, "id") === "text.chat");
      check(stringField(testedChat, "state") === "verified", "list did not render the verified capability");

      const deadEnv = { PASSQUEST_DEAD_API_KEY: KEY };
      const deadTest = await callJSON(db, ["model", "test", "dead/dead-text"], deadEnv);
      check(deadTest.code === 3, `dead endpoint exit = ${deadTest.code}`);
      check(stringField(deadTest.doc, "result") === "failed:endpoint_unreachable", "dead endpoint is not endpoint_unreachable");
      check(stringField(deadTest.doc, "checked_at") !== "", "dead endpoint failure has no timestamp");
      check((await callJSON(db, ["model", "use", "alpha/alpha-text"], alphaEnv)).code === 0, "model use before copies failed");
      const verifiedDB = copy(db, "verified");

      // 4. One transcript per failure type, each with one action.
      const failureDB = copy(db, "failures");
      const reportBaseline = await callJSON(failureDB, ["report"]);
      const failureCases: Array<{ mode: ChatMode | "nokey"; type: string; env: Record<string, string> }> = [
        { mode: "nokey", type: "no_credential", env: { PASSQUEST_ALPHA_API_KEY: "" } },
        { mode: "auth", type: "unauthorized", env: alphaEnv },
        { mode: "rate", type: "rate_limited", env: alphaEnv },
        { mode: "missing", type: "model_missing", env: alphaEnv },
        { mode: "malformed", type: "invalid_response", env: alphaEnv },
        { mode: "error", type: "provider_error", env: alphaEnv },
      ];
      for (const failureCase of failureCases) {
        if (failureCase.mode !== "nokey") alphaChat = failureCase.mode;
        const result = await callJSON(failureDB, ["model", "test", "alpha/alpha-text"], failureCase.env);
        check(result.code !== 0, `${failureCase.type} exit = ${result.code}`);
        check(
          stringField(result.doc, "result") === `failed:${failureCase.type}`,
          `${failureCase.type} result = ${stringField(result.doc, "result")}`,
        );
        check(stringField(field(result.doc, "failure"), "action") !== "", `${failureCase.type} has no next action`);
      }
      const reportAfterFailures = await callJSON(failureDB, ["report"]);
      check(
        JSON.stringify(reportBaseline.doc) === JSON.stringify(reportAfterFailures.doc),
        "a provider failure changed the attempt report",
      );

      // 5. Optional capabilities never block text; the credential never leaks.
      alphaChat = "ok";
      const optionalDB = copy(verifiedDB, "optional");
      const useOptional = await callJSON(optionalDB, ["model", "use", "alpha/alpha-opt"], alphaEnv);
      check(useOptional.code === 0, `model use alpha-opt exit = ${useOptional.code}`);
      const optionalTest = await callJSON(optionalDB, ["model", "test", "alpha/alpha-opt"], alphaEnv);
      check(optionalTest.code === 0 && stringField(optionalTest.doc, "result") === "verified", "alpha-opt text.chat did not verify");
      const optionalAsk = await callJSON(optionalDB, ["tutor", "ask"], alphaEnv);
      check(optionalAsk.code === 0, `tutor ask with unverified search/speech exit = ${optionalAsk.code}: ${optionalAsk.stderr}`);
      const optionalPractice = await callJSON(optionalDB, ["practice", "--answer", "B"]);
      check(optionalPractice.code === 0, `practice with unverified search/speech exit = ${optionalPractice.code}`);
      const optionalList = await callJSON(optionalDB, ["model", "list"], alphaEnv);
      for (const output of [JSON.stringify(optionalList.doc), JSON.stringify(optionalAsk.doc), JSON.stringify(optionalPractice.doc)]) {
        check(!output.includes(KEY), "a model output leaked the credential");
      }

      // 6. Failover: same consent succeeds and prints the line; differing consent refuses.
      const failoverDB = copy(verifiedDB, "failover");
      check(
        (await callJSON(failoverDB, ["model", "test", "beta/beta-text"], { PASSQUEST_BETA_API_KEY: KEY })).code === 0,
        "beta text.chat did not verify",
      );
      alphaChat = "rate";
      betaChat = "ok";
      const beforeBetaCalls = betaCalls.length;
      const failover = await callJSON(failoverDB, ["tutor", "ask"], { ...alphaEnv, PASSQUEST_BETA_API_KEY: KEY });
      if (!check(failover.code === 0, `failover ask exit = ${failover.code}: ${failover.stderr}`)) return fail(checks);
      check(asRecord(field(failover.doc, "failover")).applied === true, "failover was not recorded as applied");
      check(stringField(field(failover.doc, "failover"), "from_provider") === "alpha", "failover from provider is wrong");
      check(stringField(field(failover.doc, "failover"), "to_provider") === "beta", "failover target is wrong");
      check(stringField(field(failover.doc, "call"), "provider") === "beta", "the retried call is not on beta");
      check(stringField(field(failover.doc, "call"), "failover_from") === "alpha", "the retried call lost its failover origin");
      check(betaCalls.length === beforeBetaCalls + 1, "beta was not called exactly once");
      const failoverHuman = await call(["--db", failoverDB, "tutor", "ask"], { ...alphaEnv, PASSQUEST_BETA_API_KEY: KEY });
      check(failoverHuman.stdout.includes("failover: alpha → beta (reason: rate_limited)"), "human failover line is missing");

      const history = await callJSON(failoverDB, ["tutor", "history"]);
      const calls = arrayField(history.doc, "calls");
      check(
        calls.some((c) => stringField(c, "provider") === "alpha" && stringField(c, "status") === "error"),
        "the failed alpha call is not on the ledger",
      );
      check(
        calls.some((c) => stringField(c, "provider") === "beta" && stringField(c, "failover_from") === "alpha"),
        "the failover call is not on the ledger",
      );

      // A private source authorized for alpha only must not move to beta.
      const sourceDir = path.join(temp, "private-source");
      pageFixture(sourceDir);
      const privateDB = copy(failoverDB, "private");
      const imported = await callJSON(privateDB, ["import", "--path", sourceDir]);
      if (!check(imported.code === 0, `import exit = ${imported.code}: ${imported.stderr}`)) return fail(checks);
      const sourceID = stringField(field(imported.doc, "source"), "id");
      if (!sourceID) {
        check(false, "import returned no source id");
        return fail(checks);
      }
      check((await callJSON(privateDB, ["page", "review", sourceID, "1", "--reviewer", "smoke"])).code === 0, "page review failed");
      check(
        (await callJSON(privateDB, ["question", "add", sourceID, "1", "--file", path.join(sourceDir, "questions", "q1.json")])).code === 0,
        "question add failed",
      );
      check(
        (await callJSON(privateDB, ["question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "smoke"])).code === 0,
        "question review failed",
      );
      const privatePractice = await callJSON(privateDB, ["practice", "--source", sourceID, "--page", "1", "--question", "q1", "--answer", "B"]);
      check(privatePractice.code === 0, `private practice exit = ${privatePractice.code}: ${privatePractice.stderr}`);
      const consent = await callJSON(privateDB, ["tutor", "consent", sourceID, "--reviewer", "smoke", "--provider", "alpha"]);
      check(consent.code === 0, `provider-scoped consent exit = ${consent.code}`);
      const beforeRefusedBeta = betaCalls.length;
      alphaChat = "rate";
      const refused = await callJSON(privateDB, ["tutor", "ask", "--attempt", "0"], { ...alphaEnv, PASSQUEST_BETA_API_KEY: KEY });
      check(refused.code === 3, `consent refusal exit = ${refused.code}`);
      const refusal = asRecord(field(refused.failure, "failover"));
      check(refusal.applied === false, "a differing consent scope applied failover");
      check(stringField(refusal, "refusal") === "consent_scope_differs", `refusal = ${stringField(refusal, "refusal")}`);
      check((stringField(refusal, "detail") ?? "").includes(sourceID), "refusal does not name the source");
      check(betaCalls.length === beforeRefusedBeta, "beta was called despite a differing consent scope");

      // 7. Agent run keeps its recorded model across a switch and resume.
      alphaAgent = "delay";
      alphaChat = "ok";
      const agentDB = copy(verifiedDB, "agent");
      const agentStore = path.join(temp, "agent-store.sqlite");
      const paused = await callJSON(agentDB, ["agent", "start", "--timeout-seconds", "1", "--agent-store", agentStore], alphaEnv);
      check(paused.code === 4 && stringField(paused.doc, "state") === "paused", `agent start did not pause (exit ${paused.code})`);
      const runID = stringField(field(paused.doc, "run"), "run_id");
      if (!runID) {
        check(false, "paused run has no id");
        return fail(checks);
      }
      check((await callJSON(agentDB, ["model", "use", "beta/beta-text"], { PASSQUEST_BETA_API_KEY: KEY })).code === 0, "model switch before resume failed");
      alphaAgent = "tools";
      const resumed = await callJSON(
        agentDB,
        ["agent", "resume", runID, "--timeout-seconds", "60", "--agent-store", agentStore],
        { PASSQUEST_ALPHA_API_KEY: KEY, PASSQUEST_BETA_API_KEY: KEY },
      );
      check(resumed.code === 0, `agent resume exit = ${resumed.code}: ${resumed.stderr}`);
      check(stringField(field(resumed.doc, "run"), "model") === "alpha-text", "resume used the switched model");
      check(stringField(field(resumed.doc, "run"), "provider") === "alpha", "resume lost the recorded provider");
      const status = await callJSON(agentDB, ["agent", "status", runID, "--agent-store", agentStore]);
      check(stringField(field(status.doc, "run"), "model") === "alpha-text", "status does not report the recorded model");

      // 8. Per-provider budget: a refusal at each provider's own limit, and the sum.
      const budgetDB = path.join(temp, "budget.sqlite");
      const budgetEnv = { PASSQUEST_PROVIDER_REGISTRY: budgetRegistry, PASSQUEST_ALPHA_API_KEY: KEY, PASSQUEST_BETA_API_KEY: KEY };
      check((await callJSON(budgetDB, ["model", "use", "alpha/alpha-text"], budgetEnv)).code === 0, "budget model use failed");
      check((await callJSON(budgetDB, ["tutor", "config", "--daily-calls", "1"], budgetEnv)).code === 0, "budget config failed");
      alphaChat = "ok";
      betaChat = "ok";
      check((await callJSON(budgetDB, ["model", "test", "alpha/alpha-text"], budgetEnv)).code === 0, "alpha first call failed");
      const alphaRefused = await callJSON(budgetDB, ["model", "test", "alpha/alpha-text"], budgetEnv);
      check(alphaRefused.code === 2, `alpha over-limit exit = ${alphaRefused.code}`);
      check((await callJSON(budgetDB, ["model", "test", "beta/beta-text"], budgetEnv)).code === 0, "beta first call failed");
      const betaRefused = await callJSON(budgetDB, ["model", "test", "beta/beta-text"], budgetEnv);
      check(betaRefused.code === 2, `beta over-limit exit = ${betaRefused.code}`);
      const budget = await callJSON(budgetDB, ["model", "budget"], budgetEnv);
      const budgetDoc = asRecord(field(budget.doc, "budget"));
      check(numberField(budgetDoc, "total_calls") === 2, "budget total calls is not 2");
      check(numberField(budgetDoc, "total_limit") === 2, "budget total limit is not 2");
      check(stringField(budgetDoc, "ceiling") === "2 providers × 1 = 2", `ceiling = ${stringField(budgetDoc, "ceiling")}`);
      const counters = arrayField(budgetDoc, "providers");
      check(counters.some((item) => stringField(item, "provider") === "alpha" && numberField(item, "calls") === 1), "alpha counter missing");
      check(counters.some((item) => stringField(item, "provider") === "beta" && numberField(item, "calls") === 1), "beta counter missing");

      check(!seenOutputs.some((output) => output.includes(KEY)), "a command echoed the credential");
      return pass(`${assertions} assertions`);
    } catch (error) {
      return fail([...checks, `provider smoke threw: ${error instanceof Error ? error.message : String(error)}`]);
    } finally {
      server.stop(true);
      rmSync(temp, { recursive: true, force: true });
    }
  },
};
