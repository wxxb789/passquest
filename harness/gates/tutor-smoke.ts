import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

type StubMode = "ok" | "error" | "error_with_key" | "auth" | "malformed" | "rate_limited" | "timeout" | "astral";

interface ReceivedRequest {
  authorization: string;
  path: string;
  body: unknown;
  raw: string;
}

const ASTRAL_TEXT = "🚀".repeat(12000);
const CREDENTIAL_ENV = "PASSQUEST_TUTOR_API_KEY";

interface HostChat {
  model: string;
  messages: { role: string; content: string }[];
  max_tokens: number;
  temperature: number;
  stream: boolean;
}

function hostChat(content: string, maxTokens = 350): HostChat {
  return {
    model: "smoke-model",
    messages: [
      { role: "system", content: "system" },
      { role: "user", content },
    ],
    max_tokens: maxTokens,
    temperature: 0,
    stream: false,
  };
}

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

export const tutorSmoke: Gate = {
  id: "tutor-smoke",
  title: "tutor smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-tutor-smoke-"));
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
      return fail([`node is required for the tutor smoke gate: ${nodeCheck.stderr || nodeCheck.stdout}`]);
    }

    const key = `sk-${"T".repeat(32)}`;
    const baseEnv = { PASSQUEST_TUTOR_API_KEY: key };
    let mode: StubMode = "ok";
    const received: ReceivedRequest[] = [];
    const server = Bun.serve({
      hostname: "127.0.0.1",
      port: 0,
      async fetch(request) {
        const url = new URL(request.url);
        const raw = await request.text();
        let body: unknown = {};
        try {
          body = JSON.parse(raw);
        } catch {
          body = { malformed: true };
        }
        received.push({ authorization: request.headers.get("authorization") ?? "", path: url.pathname, body, raw });
        if (mode === "timeout") {
          await Bun.sleep(3000);
          return Response.json(
            { choices: [{ message: { content: "late response" } }], usage: { prompt_tokens: 1, completion_tokens: 1 } },
            { status: 200 },
          );
        }
        if (mode === "error") {
          return Response.json({ error: { message: "stub provider failure" } }, { status: 500 });
        }
        if (mode === "error_with_key") {
          return Response.json({ error: { message: `provider echoed ${key}` } }, { status: 500 });
        }
        if (mode === "auth") {
          return Response.json({ error: { message: "invalid token" } }, { status: 401 });
        }
        if (mode === "rate_limited") {
          return Response.json({ error: { message: "rate limit reached" } }, { status: 429 });
        }
        if (mode === "malformed") {
          return Response.json({ choices: [] }, { status: 200 });
        }
        if (mode === "astral") {
          return Response.json(
            { choices: [{ message: { content: ASTRAL_TEXT } }], usage: { prompt_tokens: 10, completion_tokens: 5 } },
            { status: 200 },
          );
        }
        return Response.json(
          {
            choices: [{ message: { content: "Stub coaching marker 0005: find the sentence with the closing time." } }],
            usage: { prompt_tokens: 10, completion_tokens: 5 },
          },
          { status: 200 },
        );
      },
    });

    const call = async (args: string[], env: Record<string, string> = {}) => {
      const result = await run(binary, args, { cwd: ctx.root, env: { ...baseEnv, ...env } });
      seenOutputs.push(result.stdout, result.stderr);
      return result;
    };
    const callDB = (args: string[], database: string, env: Record<string, string> = {}) =>
      call([...args, "--db", database], env);

    const hostPath = path.join(ctx.root, "agent", "tutor-host.mjs");
    const endpoint = `http://127.0.0.1:${server.port}/v1/chat/completions`;

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const host = async (request: object): Promise<unknown> => {
        const result = await run(executable("node"), [hostPath], {
          cwd: ctx.root,
          env: baseEnv,
          stdin: JSON.stringify(request),
        });
        return parse(result.stdout);
      };
      const hostDocument = (chat: HostChat, body = JSON.stringify(chat)) => ({
        protocol_version: 1,
        endpoint,
        timeout_ms: 30000,
        max_response_chars: 12000,
        chat,
        body,
      });

      for (const [label, maxTokens, contentChars] of [
        ["max_tokens 4096", 4096, 10],
        ["21001 message characters", 350, 21001],
      ] as const) {
        const chat = hostChat("x".repeat(contentChars), maxTokens);
        const doc = await host(hostDocument(chat));
        check(stringField(doc, "status") === "error", `host accepted ${label}`);
        check(stringField(field(doc, "error"), "code") === "config_error", `host ${label} error code`);
      }
      const matchingChat = hostChat("x".repeat(10));
      const mismatched = await host(
        hostDocument(matchingChat, JSON.stringify({ ...matchingChat, model: "other-model" })),
      );
      check(stringField(mismatched, "status") === "error", "host accepted a body that does not match chat");
      check(stringField(field(mismatched, "error"), "code") === "config_error", "mismatched body error code");

      const db = path.join(temp, "tutor.sqlite");
      const unconfigured = await callDB(["tutor", "config"], db, { PASSQUEST_TUTOR_API_KEY: "" });
      check(unconfigured.code === 0, `human tutor config exit = ${unconfigured.code}`);
      check(unconfigured.stdout.includes("Provider: not configured"), "human config does not report an unconfigured provider");
      check(
        unconfigured.stdout.includes(`Credential: ${CREDENTIAL_ENV} is missing`),
        "human config does not report a missing credential",
      );
      const configResult = await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"],
        db,
      );
      if (!check(configResult.code === 0, `tutor config exit = ${configResult.code}`)) return fail(checks);
      const configHuman = await callDB(["tutor", "config"], db);
      check(
        configHuman.stdout.includes(`Credential: ${CREDENTIAL_ENV} is set`),
        "human config does not report a present credential",
      );
      check(
        configHuman.stdout.includes(`Provider endpoint: ${endpoint}`),
        "human config does not print the saved endpoint",
      );
      const config = parse(configResult.stdout);
      check(field(config, "configured") === true, "tutor config is not configured after saving");
      check(stringField(field(config, "config"), "model") === "smoke-model", "tutor config model");
      check(field(field(config, "credential"), "present") === true, "tutor config does not see the credential");
      check(!configResult.stdout.includes(key), "tutor config output contains the credential");

      const emptyPreview = await callDB(["tutor", "preview", "--json"], db);
      check(emptyPreview.code === 2, `tutor preview without attempts exit = ${emptyPreview.code}`);
      check(stringField(field(parse(emptyPreview.stderr), "error"), "code") === "no_attempts", "tutor preview empty error code");

      const practice = await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], db);
      if (!check(practice.code === 0, `practice exit = ${practice.code}`)) return fail(checks);

      const previewResult = await callDB(["tutor", "preview", "--json"], db);
      if (!check(previewResult.code === 0, `tutor preview exit = ${previewResult.code}`)) return fail(checks);
      const preview = parse(previewResult.stdout);
      check(stringField(preview, "state") === "preview", "tutor preview state");
      check(received.length === 0, "tutor preview contacted the provider");
      check(field(field(preview, "context"), "private_source") === false, "first-party preview is marked private");
      check(field(field(preview, "context"), "source_text_shared") === false, "first-party preview marks source text shared");
      check(!previewResult.stdout.includes(key), "tutor preview output contains the credential");
      const previewUser = userMessage(preview);
      check(previewUser.includes("Westbridge"), "tutor preview does not include the first-party passage");
      check((stringField(preview, "request_sha256") ?? "").length === 64, "tutor preview has no request hash");
      check((numberField(preview, "request_bytes") ?? 0) > 0, "tutor preview has no request size");
      check((stringField(field(preview, "provider"), "endpoint") ?? "") === endpoint, "tutor preview endpoint");

      const missingKey = await callDB(["tutor", "ask", "--json"], db, { PASSQUEST_TUTOR_API_KEY: "" });
      check(missingKey.code === 2, `tutor ask without credential exit = ${missingKey.code}`);
      check(received.length === 0, "tutor ask without credential contacted the provider");
      check(field(field(parse(missingKey.stderr), "error"), "code") === "missing_credential", "missing credential error code");

      const before = await callDB(["report", "--json"], db);
      const askResult = await callDB(["tutor", "ask", "--json"], db);
      if (!check(askResult.code === 0, `tutor ask exit = ${askResult.code}\n${askResult.stderr}`)) return fail(checks);
      const ask = parse(askResult.stdout);
      check(stringField(ask, "state") === "advisory", "tutor ask state");
      check(field(field(ask, "suggestion"), "advisory") === true, "tutor suggestion is not marked advisory");
      check(field(field(ask, "suggestion"), "verified_answer") === false, "tutor suggestion claims a verified answer");
      check((stringField(field(ask, "suggestion"), "text") ?? "").includes("Stub coaching marker 0005"), "tutor suggestion text");
      check(stringField(field(ask, "call"), "status") === "ok", "tutor call status");
      check(numberField(field(ask, "call"), "prompt_tokens") === 10, "tutor call prompt tokens");
      check(numberField(field(ask, "call"), "completion_tokens") === 5, "tutor call completion tokens");
      check(received.length === 1, `provider requests after one ask = ${received.length}`);
      const firstRequest = received[0];
      check(firstRequest.authorization === `Bearer ${key}`, "provider did not receive the bearer credential");
      check(firstRequest.path === "/v1/chat/completions", `provider path = ${firstRequest.path}`);
      check(stringField(firstRequest.body, "model") === "smoke-model", "provider model");
      check(numberField(firstRequest.body, "max_tokens") === 350, "provider max_tokens");
      check(field(firstRequest.body, "temperature") === 0, "provider temperature");
      check(field(firstRequest.body, "stream") === false, "provider stream flag");
      check(arrayField(firstRequest.body, "messages").length === 2, "provider messages");
      check(!JSON.stringify(firstRequest.body).includes(key), "provider request body contains the credential");
      const postedSHA = createHash("sha256").update(firstRequest.raw).digest("hex");
      check(stringField(preview, "request_sha256") === postedSHA, "preview request hash is not the posted body");
      check(stringField(field(ask, "call"), "request_sha256") === postedSHA, "call request hash is not the posted body");
      check(numberField(preview, "request_bytes") === Buffer.byteLength(firstRequest.raw), "preview request size is not the posted body");
      check(
        numberField(field(ask, "call"), "request_bytes") === Buffer.byteLength(firstRequest.raw),
        "call request size is not the posted body",
      );

      const after = await callDB(["report", "--json"], db);
      check(before.stdout === after.stdout, "tutor ask changed the attempt report");
      check(!after.stdout.includes("Stub coaching marker 0005"), "model text leaked into the attempt report");
      const next = await callDB(["next", "--json"], db);
      check(!next.stdout.includes("Stub coaching marker 0005"), "model text leaked into next");
      const history = parse((await callDB(["tutor", "history", "--json"], db)).stdout);
      check(numberField(field(history, "summary"), "calls") === 1, "tutor history did not count the call");
      check(numberField(field(history, "summary"), "ok") === 1, "tutor history did not count a success");
      check(arrayField(history, "calls").length === 1, "tutor history call list");
      check(!(arrayField(history, "calls")[0] && "output_text" in (arrayField(history, "calls")[0] as object)), "tutor history leaks model text");
      const show = parse((await callDB(["tutor", "show", "1", "--json"], db)).stdout);
      check(stringField(show, "state") === "advisory", "tutor show state");
      check((stringField(field(show, "suggestion"), "text") ?? "").includes("Stub coaching marker 0005"), "tutor show text");

      const importResult = await call(
        ["import", "--path", path.join(ctx.root, "internal", "cli", "testdata", "import"), "--json", "--db", db],
      );
      if (!check(importResult.code === 0, `import exit = ${importResult.code}`)) return fail(checks);
      const sourceID = stringField(field(parse(importResult.stdout), "source"), "id");
      if (!check(sourceID !== undefined, "import did not return a source id")) return fail(checks);
      const questionFile = path.join(ctx.root, "internal", "cli", "testdata", "import", "question-q2-tagged.json");
      for (const args of [
        ["question", "add", sourceID!, "1", "--file", questionFile, "--json"],
        ["page", "review", sourceID!, "1", "--reviewer", "smoke", "--json"],
        ["question", "review", sourceID!, "q2", "--answer", "C", "--reviewer", "smoke", "--json"],
        ["practice", "--source", sourceID!, "--page", "1", "--question", "q2", "--answer", "C", "--json"],
      ]) {
        const result = await callDB(args, db);
        check(result.code === 0, `${args.join(" ")} exit = ${result.code}`);
      }

      const privatePreviewResult = await callDB(["tutor", "preview", "--json"], db);
      const privatePreview = parse(privatePreviewResult.stdout);
      check(field(field(privatePreview, "context"), "private_source") === true, "source preview is not private");
      check(field(field(privatePreview, "context"), "consent_granted") === false, "source preview has consent before a grant");
      check(field(field(privatePreview, "context"), "source_text_shared") === false, "withheld source preview marks text shared");
      const privateUser = userMessage(privatePreview);
      for (const secret of [
        "The Westbridge Community Library closes at 4 p.m. on Saturdays.",
        "Which of the following is NOT stated",
        "The library closes at 4 p.m. on Saturdays.",
      ]) {
        check(!privateUser.includes(secret), `withheld source text leaked: ${secret.slice(0, 32)}`);
      }
      check(arrayField(field(privatePreview, "context"), "withheld").includes("passage"), "withheld fields do not name the passage");
      check(received.length === 1, "preview of a private source contacted the provider");

      const consent = await callDB(["tutor", "consent", sourceID!, "--reviewer", "smoke", "--json"], db);
      check(consent.code === 0, `tutor consent exit = ${consent.code}`);
      const authorized = parse((await callDB(["tutor", "preview", "--json"], db)).stdout);
      check(field(field(authorized, "context"), "consent_granted") === true, "consent did not grant access");
      check(field(field(authorized, "context"), "source_text_shared") === true, "authorized preview does not share source text");
      check(userMessage(authorized).includes("The Westbridge Community Library closes at 4 p.m. on Saturdays."), "authorized preview omits the page text");
      const grantedConsentsHuman = await callDB(["tutor", "consents"], db);
      check(
        grantedConsentsHuman.stdout.includes(`  ${sourceID}  import  authorized`),
        "human consents does not label a granted source as authorized",
      );

      const sourceAsk = await callDB(["tutor", "ask", "--json"], db);
      if (!check(sourceAsk.code === 0, `source tutor ask exit = ${sourceAsk.code}`)) return fail(checks);
      const sourceCall = field(parse(sourceAsk.stdout), "call");
      check(field(sourceCall, "source_text_shared") === true, "source ask record does not mark shared text");
      check(received.length === 2, "authorized source ask did not contact the provider");
      check(JSON.stringify(received[1].body).includes("The Westbridge Community Library closes at 4 p.m. on Saturdays."), "provider did not receive authorized page text");
      const sharedHistoryHuman = await callDB(["tutor", "history"], db);
      check(
        sharedHistoryHuman.stdout.includes("source-text-shared"),
        "human history does not label shared private text",
      );

      const revoke = await callDB(["tutor", "revoke", sourceID!, "--reviewer", "smoke", "--json"], db);
      check(revoke.code === 0, `tutor revoke exit = ${revoke.code}`);
      const revoked = parse((await callDB(["tutor", "preview", "--json"], db)).stdout);
      check(field(field(revoked, "context"), "consent_granted") === false, "revoke did not withhold access");
      check(!userMessage(revoked).includes("The Westbridge Community Library closes at 4 p.m."), "revoked source text is still in the prompt");
      const consents = parse((await callDB(["tutor", "consents", "--json"], db)).stdout);
      check(arrayField(consents, "sources").length === 1, "tutor consents did not list the source");
      check(field(arrayField(consents, "sources")[0], "granted") === false, "tutor consents effective state after revoke");
      const revokedConsentsHuman = await callDB(["tutor", "consents"], db);
      check(
        revokedConsentsHuman.stdout.includes(`  ${sourceID}  import  withheld`),
        "human consents does not label a revoked source as withheld",
      );

      const reportBeforeFailure = await callDB(["report", "--json"], db);
      mode = "error";
      const providerFailure = await callDB(["tutor", "ask", "--json"], db);
      check(providerFailure.code === 3, `provider failure exit = ${providerFailure.code}`);
      check(stringField(field(parse(providerFailure.stderr), "error"), "code") === "provider_error", "provider failure code");
      const reportAfterFailure = await callDB(["report", "--json"], db);
      check(reportBeforeFailure.stdout === reportAfterFailure.stdout, "failed provider call changed the attempt report");

      mode = "auth";
      const authFailure = await callDB(["tutor", "ask", "--json"], db);
      check(authFailure.code === 3, `auth failure exit = ${authFailure.code}`);
      check(stringField(field(parse(authFailure.stderr), "error"), "code") === "auth_failed", "auth failure code");

      mode = "malformed";
      const malformed = await callDB(["tutor", "ask", "--json"], db);
      check(malformed.code === 3, `malformed failure exit = ${malformed.code}`);
      check(stringField(field(parse(malformed.stderr), "error"), "code") === "invalid_response", "malformed response code");

      mode = "ok";
      const requestsBeforeNode = received.length;
      const missingNode = await callDB(["tutor", "ask", "--json"], db, {
        PASSQUEST_NODE: path.join(temp, "missing-node"),
      });
      check(missingNode.code === 3, `missing node exit = ${missingNode.code}`);
      check(stringField(field(parse(missingNode.stderr), "error"), "code") === "agent_unavailable", "missing node code");
      check(received.length === requestsBeforeNode, "missing node still contacted the provider");
      const practiceAfterFailure = await callDB(["practice", "--question", "q3", "--answer", "B", "--json"], db);
      check(practiceAfterFailure.code === 0, `practice after tutor failure exit = ${practiceAfterFailure.code}`);

      const mainHistory = parse((await callDB(["tutor", "history", "--json"], db)).stdout);
      check(numberField(field(mainHistory, "summary"), "calls") === 6, "tutor history call total");
      check(numberField(field(mainHistory, "summary"), "ok") === 2, "tutor history success total");
      check(numberField(field(mainHistory, "summary"), "failed") === 4, "tutor history failure total");
      check(numberField(field(mainHistory, "summary"), "prompt_tokens") === 20, "tutor history prompt tokens");
      check(numberField(field(mainHistory, "summary"), "completion_tokens") === 10, "tutor history completion tokens");
      check(arrayField(mainHistory, "calls").length === 6, "tutor history rows");
      check(!seenOutputs.some((output) => output.includes(key)), "credential appeared in command output");

      const failureDB = path.join(temp, "failure.sqlite");
      await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--timeout-seconds", "1", "--json"],
        failureDB,
      );
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], failureDB);
      const failureReport = await callDB(["report", "--json"], failureDB);

      mode = "rate_limited";
      const rateBefore = received.length;
      const rateLimited = await callDB(["tutor", "ask", "--json"], failureDB);
      check(rateLimited.code === 3, `rate limited exit = ${rateLimited.code}`);
      check(stringField(field(parse(rateLimited.stderr), "error"), "code") === "rate_limited", "rate limited code");
      check(received.length === rateBefore + 1, "rate limited request did not reach the provider");
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        "rate limited call changed the attempt report",
      );

      mode = "timeout";
      const timeout = await callDB(["tutor", "ask", "--json"], failureDB);
      check(timeout.code === 3, `timeout exit = ${timeout.code}`);
      check(stringField(field(parse(timeout.stderr), "error"), "code") === "timeout", "timeout code");
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        "timed out call changed the attempt report",
      );

      const closed = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch() { return new Response(""); } });
      const closedPort = closed.port;
      closed.stop(true);
      await callDB(
        ["tutor", "config", "--endpoint", `http://127.0.0.1:${closedPort}/v1/chat/completions`, "--json"],
        failureDB,
      );
      mode = "ok";
      const networkFailure = await callDB(["tutor", "ask", "--json"], failureDB);
      check(networkFailure.code === 3, `network failure exit = ${networkFailure.code}`);
      check(stringField(field(parse(networkFailure.stderr), "error"), "code") === "network_error", "network failure code");
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        "network failure changed the attempt report",
      );
      const failureHistory = parse((await callDB(["tutor", "history", "--json"], failureDB)).stdout);
      check(numberField(field(failureHistory, "summary"), "calls") === 3, "failure history call count");
      check(numberField(field(failureHistory, "summary"), "failed") === 3, "failure history failure count");

      const credentialDB = path.join(temp, "credential.sqlite");
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], credentialDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], credentialDB);

      mode = "error_with_key";
      const echoedError = await callDB(["tutor", "ask", "--json"], credentialDB);
      check(echoedError.code === 3, `credential error exit = ${echoedError.code}`);
      check(stringField(field(parse(echoedError.stderr), "error"), "code") === "provider_error", "credential error code");
      check(!echoedError.stdout.includes(key) && !echoedError.stderr.includes(key), "provider error echoed the credential");
      check(echoedError.stderr.includes("[redacted]"), "provider error was not redacted");

      mode = "ok";
      const fakeHost = path.join(temp, "fake-host.mjs");
      writeFileSync(
        fakeHost,
        'process.stderr.write("host failed with " + (process.env.PASSQUEST_TUTOR_API_KEY || ""));\nprocess.exit(1);\n',
      );
      const hostFailure = await callDB(["tutor", "ask", "--json"], credentialDB, { PASSQUEST_TUTOR_HOST: fakeHost });
      check(hostFailure.code === 3, `host stderr failure exit = ${hostFailure.code}`);
      check(stringField(field(parse(hostFailure.stderr), "error"), "code") === "agent_unavailable", "host stderr failure code");
      check(!hostFailure.stdout.includes(key) && !hostFailure.stderr.includes(key), "host stderr echoed the credential");
      check(hostFailure.stderr.includes("[redacted]"), "host stderr credential was not redacted");
      const credentialBytes = readFileSync(credentialDB);
      check(!credentialBytes.includes(Buffer.from(key)), "credential database bytes contain the credential");

      const budgetDB = path.join(temp, "budget.sqlite");
      await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--daily-calls", "1", "--json"],
        budgetDB,
      );
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], budgetDB);
      const budgetBefore = received.length;
      const budgetFirst = await callDB(["tutor", "ask", "--json"], budgetDB);
      if (!check(budgetFirst.code === 0, `budget first ask exit = ${budgetFirst.code}`)) return fail(checks);
      const budgetSecond = await callDB(["tutor", "ask", "--json"], budgetDB);
      check(budgetSecond.code === 2, `budget second ask exit = ${budgetSecond.code}`);
      check(stringField(field(parse(budgetSecond.stderr), "error"), "code") === "budget_exhausted", "budget error code");
      check(received.length === budgetBefore + 1, "exhausted budget still contacted the provider");
      const budgetHistory = parse((await callDB(["tutor", "history", "--json"], budgetDB)).stdout);
      check(numberField(field(budgetHistory, "summary"), "calls") === 1, "budget history call count");
      check(numberField(field(budgetHistory, "summary"), "daily_limit") === 1, "budget history daily limit");

      const humanDB = path.join(temp, "human.sqlite");
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], humanDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], humanDB);
      const humanAsk = await callDB(["tutor", "ask"], humanDB);
      if (!check(humanAsk.code === 0, `human tutor ask exit = ${humanAsk.code}`)) return fail(checks);
      for (const want of [
        "Pass Quest — advisory AI suggestion (not a verified answer)",
        "--- AI suggestion (advisory) ---",
        "This is model output. It is not a verified answer, grade or exam fact.",
        "Privacy: first-party material only; no private source text is involved",
        "Content: the current passage, question, options, verified key and evidence are included.",
        "Stub coaching marker 0005",
      ]) {
        check(humanAsk.stdout.includes(want), `human advisory output is missing ${want}`);
      }
      check(!humanAsk.stdout.includes(key), "human advisory output contains the credential");
      const humanShow = await callDB(["tutor", "show", "1"], humanDB);
      check(humanShow.stdout.includes("Pass Quest — advisory call #1"), "human show does not identify the call");
      check(
        humanShow.stdout.includes("This stored text is model output, not a verified answer, grade or exam fact."),
        "human show omits the advisory disclaimer",
      );
      const humanHistory = await callDB(["tutor", "history"], humanDB);
      check(humanHistory.stdout.includes("first-party"), "human history does not label first-party calls");

      // The host posts request.body verbatim: a body with &, < and > must
      // arrive byte-identical, so the recorded SHA-256 can match provider logs.
      const specialChat = hostChat("Marks & Spencer <tag> & more");
      const specialBody = JSON.stringify(specialChat);
      check(
        field(await host(hostDocument(specialChat, specialBody)), "status") === "ok",
        "host rejected a valid special-character body",
      );
      check(received.at(-1)?.raw === specialBody, "host did not post the request body verbatim");

      // 15,000 astral code points are 30,000 UTF-16 units; the documented
      // 20,000-character bound counts code points.
      const astralChat = hostChat("🚀".repeat(15000));
      check(
        field(await host(hostDocument(astralChat)), "status") === "ok",
        "host rejected a 15000-code-point astral prompt",
      );

      const boundsDB = path.join(temp, "bounds.sqlite");
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], boundsDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], boundsDB);
      mode = "astral";
      const astral = await callDB(["tutor", "ask", "--json"], boundsDB);
      mode = "ok";
      if (!check(astral.code === 0, `astral response exit = ${astral.code}`)) return fail(checks);
      check(
        numberField(field(parse(astral.stdout), "call"), "output_chars") === 12000,
        "a 12000-code-point astral response was rejected or clipped",
      );

      const mainBytes = readFileSync(db);
      check(!mainBytes.includes(Buffer.from(key)), "main database bytes contain the credential");
      const budgetBytes = readFileSync(budgetDB);
      check(!budgetBytes.includes(Buffer.from(key)), "budget database bytes contain the credential");
      check(!seenOutputs.some((output) => output.includes(key)), "credential appeared in captured output");
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      server.stop(true);
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);
  },
};

function userMessage(document: unknown): string {
  const messages = arrayField(field(document, "chat"), "messages");
  const user = messages.find((message) => stringField(message, "role") === "user");
  return stringField(user, "content") ?? "";
}
