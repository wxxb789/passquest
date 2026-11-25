import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { chatReply, responsesReply } from "../lib/desktop-page.ts";
import { buildCLI } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

type StubMode = "ok" | "error" | "error_with_key" | "auth" | "malformed" | "rate_limited" | "timeout" | "astral" | "wrong_format" | "incomplete";

interface ReceivedRequest {
  authorization: string;
  path: string;
  body: unknown;
  raw: string;
}

const ASTRAL_TEXT = "🚀".repeat(12000);
const CREDENTIAL_ENV = "PASSQUEST_TUTOR_API_KEY";
const COACHING_TEXT = "Stub coaching marker 0005: find the sentence with the closing time.";
const REFINEMENT_TEXT = '{"front":"refined front","back":"refined meaning\\n\\nExample: refined example"}';

interface HostChat {
  model: string;
  messages: { role: string; content: string }[];
  max_tokens: number;
  temperature: number;
  stream: boolean;
}

interface HostResponses {
  model: string;
  instructions: string;
  input: { role: string; content: { type: string; text: string }[] }[];
  max_output_tokens: number;
  temperature: number;
  stream: boolean;
  store: boolean;
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

function hostResponses(content: string, maxTokens = 350): HostResponses {
  return {
    model: "smoke-model",
    instructions: "system",
    input: [{ role: "user", content: [{ type: "input_text", text: content }] }],
    max_output_tokens: maxTokens,
    temperature: 0,
    stream: false,
    store: false,
  };
}

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

/** The wire document's own text, whichever format the request carries. */
function wireText(body: unknown): string {
  const document = body && typeof body === "object" ? (body as Record<string, unknown>) : {};
  if (typeof document.instructions === "string") return `${document.instructions}\n${JSON.stringify(document.input ?? [])}`;
  return JSON.stringify(document.messages ?? []);
}

interface WirePass {
  label: string;
  suffix: string;
  /** The endpoint the pass configures, filled once the stub's port is known. */
  endpoint: string;
}

export const tutorSmoke: Gate = {
  id: "tutor-smoke",
  title: "tutor smoke",
  async run(ctx: GateContext) {
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-tutor-smoke-"));
    let binary = "";
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
        const wantsResponses = url.pathname.endsWith("/responses");
        // The refinement path's own ask: the stub answers the strict JSON the
        // refinement parser needs, in the request format's own response shape.
        if (mode === "ok" && wireText(body).includes("flashcard")) {
          return wantsResponses ? responsesReply(REFINEMENT_TEXT) : chatReply(REFINEMENT_TEXT);
        }
        if (mode === "timeout") {
          await Bun.sleep(3000);
          return wantsResponses ? responsesReply("late response") : chatReply("late response");
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
          return Response.json(wantsResponses ? { status: "completed", output: [] } : { choices: [] }, { status: 200 });
        }
        if (mode === "astral") {
          return wantsResponses ? responsesReply(ASTRAL_TEXT) : chatReply(ASTRAL_TEXT);
        }
        if (mode === "wrong_format") return wantsResponses ? chatReply(COACHING_TEXT) : responsesReply(COACHING_TEXT);
        if (mode === "incomplete") {
          return Response.json({
            status: "incomplete",
            incomplete_details: { reason: "max_output_tokens" },
            output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "cut off" }] }],
            usage: { input_tokens: 10, output_tokens: 5 },
          });
        }
        return wantsResponses ? responsesReply(COACHING_TEXT) : chatReply(COACHING_TEXT);
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
    const host = async (request: object): Promise<unknown> => {
      const result = await run(executable("node"), [hostPath], {
        cwd: ctx.root,
        env: baseEnv,
        stdin: JSON.stringify(request),
      });
      return parse(result.stdout);
    };
    const hostDocument = (wire: WirePass, document: HostChat | HostResponses, body = JSON.stringify(document)) => ({
      protocol_version: 1,
      endpoint: wire.endpoint,
      timeout_ms: 30000,
      max_response_chars: 12000,
      [wire.suffix.endsWith("/responses") ? "responses" : "chat"]: document,
      body,
    });
    const passes: WirePass[] = [
      { label: "chat", suffix: "/v1/chat/completions", endpoint: "" },
      { label: "responses", suffix: "/v1/responses", endpoint: "" },
    ];
    for (const wire of passes) wire.endpoint = `http://127.0.0.1:${server.port}${wire.suffix}`;

    try {
      const build = await buildCLI(ctx);
      if ("failure" in build) {
        return fail(["go build ./cmd/passquest failed", build.failure]);
      }
      binary = build.binary;

      for (const [label, maxTokens, contentChars] of [
        ["max_tokens 4096", 4096, 10],
        ["21001 message characters", 350, 21001],
      ] as const) {
        const chat = hostChat("x".repeat(contentChars), maxTokens);
        const doc = await host(hostDocument(passes[0], chat));
        check(stringField(doc, "status") === "error", `host accepted ${label}`);
        check(stringField(field(doc, "error"), "code") === "config_error", `host ${label} error code`);
      }
      const matchingChat = hostChat("x".repeat(10));
      const mismatched = await host(
        hostDocument(passes[0], matchingChat, JSON.stringify({ ...matchingChat, model: "other-model" })),
      );
      check(stringField(mismatched, "status") === "error", "host accepted a body that does not match chat");
      check(stringField(field(mismatched, "error"), "code") === "config_error", "mismatched body error code");

      const matchingResponses = hostResponses("x".repeat(10));
      const storedResponses = { ...matchingResponses, store: true };
      const stored = await host(hostDocument(passes[1], storedResponses));
      check(stringField(stored, "status") === "error", "host accepted a responses body with store true");
      check(stringField(field(stored, "error"), "code") === "config_error", "store true error code");
      check(
        (stringField(field(stored, "error"), "message") ?? "").includes("store"),
        "store true error does not name the field",
      );
      const crossedDocument = { ...matchingChat };
      const crossed = await host({
        protocol_version: 1,
        endpoint: passes[1].endpoint,
        timeout_ms: 30000,
        max_response_chars: 12000,
        chat: crossedDocument,
        body: JSON.stringify(crossedDocument),
      });
      check(stringField(crossed, "status") === "error", "host accepted a chat document for a responses endpoint");
      check(
        (stringField(field(crossed, "error"), "message") ?? "").includes("/responses"),
        "cross-format refusal does not name the endpoint's form",
      );

      for (const wire of passes) await runWirePass(wire);
      mode = "ok";

      check(!seenOutputs.some((output) => output.includes(key)), "credential appeared in command output");
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      server.stop(true);
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);

    /** Everything one wire format must prove, over its own databases and endpoint. */
    async function runWirePass(wire: WirePass) {
      const label = wire.label;
      const endpoint = wire.endpoint;
      const isResponses = wire.suffix.endsWith("/responses");
      /** The wire document's learner-visible request text, per format. */
      const previewUserText = (document: unknown): string => {
        if (isResponses) {
          const responses = field(document, "responses");
          const input = arrayField(responses, "input");
          const parts = arrayField(input[0], "content");
          return stringField(parts[0], "text") ?? "";
        }
        const messages = arrayField(field(document, "chat"), "messages");
        const user = messages.find((message) => stringField(message, "role") === "user");
        return stringField(user, "content") ?? "";
      };

      const unconfigured = await callDB(["tutor", "config"], path.join(temp, `${label}-config.sqlite`), {
        PASSQUEST_TUTOR_API_KEY: "",
      });
      check(unconfigured.code === 0, `${label}: human tutor config exit = ${unconfigured.code}`);
      check(
        unconfigured.stdout.includes("Provider: not configured"),
        `${label}: human config does not report an unconfigured provider`,
      );
      check(
        unconfigured.stdout.includes(`Credential: ${CREDENTIAL_ENV} is missing`),
        `${label}: human config does not report a missing credential`,
      );

      const db = path.join(temp, `${label}.sqlite`);
      const configResult = await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"],
        db,
      );
      if (!check(configResult.code === 0, `${label}: tutor config exit = ${configResult.code}`)) return;
      const configHuman = await callDB(["tutor", "config"], db);
      check(
        configHuman.stdout.includes(`Credential: ${CREDENTIAL_ENV} is set`),
        `${label}: human config does not report a present credential`,
      );
      check(
        configHuman.stdout.includes(`Provider endpoint: ${endpoint}`),
        `${label}: human config does not print the saved endpoint`,
      );
      const config = parse(configResult.stdout);
      check(field(config, "configured") === true, `${label}: tutor config is not configured after saving`);
      check(stringField(field(config, "config"), "model") === "smoke-model", `${label}: tutor config model`);
      check(field(field(config, "credential"), "present") === true, `${label}: tutor config does not see the credential`);
      check(!configResult.stdout.includes(key), `${label}: tutor config output contains the credential`);

      const emptyPreview = await callDB(["tutor", "preview", "--json"], db);
      check(emptyPreview.code === 2, `${label}: tutor preview without attempts exit = ${emptyPreview.code}`);
      check(
        stringField(field(parse(emptyPreview.stderr), "error"), "code") === "no_attempts",
        `${label}: tutor preview empty error code`,
      );

      const practice = await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], db);
      if (!check(practice.code === 0, `${label}: practice exit = ${practice.code}`)) return;

      const beforePreview = received.length;
      const previewResult = await callDB(["tutor", "preview", "--json"], db);
      if (!check(previewResult.code === 0, `${label}: tutor preview exit = ${previewResult.code}`)) return;
      const preview = parse(previewResult.stdout);
      check(stringField(preview, "state") === "preview", `${label}: tutor preview state`);
      check(received.length === beforePreview, `${label}: tutor preview contacted the provider`);
      check(field(field(preview, "context"), "private_source") === false, `${label}: first-party preview is marked private`);
      check(
        field(field(preview, "context"), "source_text_shared") === false,
        `${label}: first-party preview marks source text shared`,
      );
      check(!previewResult.stdout.includes(key), `${label}: tutor preview output contains the credential`);
      const previewUser = previewUserText(preview);
      check(previewUser.includes("Westbridge"), `${label}: tutor preview does not include the first-party passage`);
      check((stringField(preview, "request_sha256") ?? "").length === 64, `${label}: tutor preview has no request hash`);
      check((numberField(preview, "request_bytes") ?? 0) > 0, `${label}: tutor preview has no request size`);
      check((stringField(field(preview, "provider"), "endpoint") ?? "") === endpoint, `${label}: tutor preview endpoint`);
      check(field(preview, "chat") === undefined || !isResponses, `${label}: responses preview carries a chat document`);
      check(field(preview, "responses") === undefined || isResponses, `${label}: chat preview carries a responses document`);
      if (isResponses) {
        const responses = field(preview, "responses");
        check((stringField(responses, "instructions") ?? "").includes("text study coach"), `${label}: preview has no instructions`);
        check(numberField(responses, "max_output_tokens") === 350, `${label}: preview max_output_tokens`);
        check(field(responses, "store") === false, `${label}: preview store is not false`);
        check(field(responses, "stream") === false && field(responses, "temperature") === 0, `${label}: preview stream/temperature`);
        check(arrayField(responses, "input").length === 1, `${label}: preview input is not one item`);
      } else {
        check(arrayField(field(preview, "chat"), "messages").length === 2, `${label}: preview messages`);
      }

      const missingKey = await callDB(["tutor", "ask", "--json"], db, { PASSQUEST_TUTOR_API_KEY: "" });
      check(missingKey.code === 2, `${label}: tutor ask without credential exit = ${missingKey.code}`);
      check(received.length === beforePreview, `${label}: tutor ask without credential contacted the provider`);
      check(
        field(field(parse(missingKey.stderr), "error"), "code") === "missing_credential",
        `${label}: missing credential error code`,
      );

      const before = await callDB(["report", "--json"], db);
      const askResult = await callDB(["tutor", "ask", "--json"], db);
      if (!check(askResult.code === 0, `${label}: tutor ask exit = ${askResult.code}\n${askResult.stderr}`)) return;
      const ask = parse(askResult.stdout);
      check(stringField(ask, "state") === "advisory", `${label}: tutor ask state`);
      check(field(field(ask, "suggestion"), "advisory") === true, `${label}: tutor suggestion is not marked advisory`);
      check(field(field(ask, "suggestion"), "verified_answer") === false, `${label}: tutor suggestion claims a verified answer`);
      check(
        (stringField(field(ask, "suggestion"), "text") ?? "").includes("Stub coaching marker 0005"),
        `${label}: tutor suggestion text`,
      );
      check(stringField(field(ask, "call"), "status") === "ok", `${label}: tutor call status`);
      check(numberField(field(ask, "call"), "prompt_tokens") === 10, `${label}: tutor call prompt tokens`);
      check(numberField(field(ask, "call"), "completion_tokens") === 5, `${label}: tutor call completion tokens`);
      check(received.length === beforePreview + 1, `${label}: provider requests after one ask = ${received.length - beforePreview}`);
      const firstRequest = received[received.length - 1];
      check(firstRequest.authorization === `Bearer ${key}`, `${label}: provider did not receive the bearer credential`);
      check(firstRequest.path === wire.suffix, `${label}: provider path = ${firstRequest.path}`);
      check(stringField(firstRequest.body, "model") === "smoke-model", `${label}: provider model`);
      if (isResponses) {
        check(stringField(firstRequest.body, "instructions") !== undefined, `${label}: provider request has no instructions`);
        check(numberField(firstRequest.body, "max_output_tokens") === 350, `${label}: provider max_output_tokens`);
        check(field(firstRequest.body, "store") === false, `${label}: provider store is not false`);
        check(field(firstRequest.body, "temperature") === 0, `${label}: provider temperature`);
        check(field(firstRequest.body, "stream") === false, `${label}: provider stream flag`);
        check(arrayField(firstRequest.body, "input").length === 1, `${label}: provider input items`);
        check(field(firstRequest.body, "messages") === undefined, `${label}: provider request carries chat messages`);
      } else {
        check(numberField(firstRequest.body, "max_tokens") === 350, `${label}: provider max_tokens`);
        check(field(firstRequest.body, "temperature") === 0, `${label}: provider temperature`);
        check(field(firstRequest.body, "stream") === false, `${label}: provider stream flag`);
        check(arrayField(firstRequest.body, "messages").length === 2, `${label}: provider messages`);
        check(field(firstRequest.body, "instructions") === undefined, `${label}: provider request carries instructions`);
      }
      check(!JSON.stringify(firstRequest.body).includes(key), `${label}: provider request body contains the credential`);
      const postedSHA = createHash("sha256").update(firstRequest.raw).digest("hex");
      check(stringField(preview, "request_sha256") === postedSHA, `${label}: preview request hash is not the posted body`);
      check(stringField(field(ask, "call"), "request_sha256") === postedSHA, `${label}: call request hash is not the posted body`);
      check(
        numberField(preview, "request_bytes") === Buffer.byteLength(firstRequest.raw),
        `${label}: preview request size is not the posted body`,
      );
      check(
        numberField(field(ask, "call"), "request_bytes") === Buffer.byteLength(firstRequest.raw),
        `${label}: call request size is not the posted body`,
      );

      const after = await callDB(["report", "--json"], db);
      check(before.stdout === after.stdout, `${label}: tutor ask changed the attempt report`);
      check(!after.stdout.includes("Stub coaching marker 0005"), `${label}: model text leaked into the attempt report`);
      const next = await callDB(["next", "--json"], db);
      check(!next.stdout.includes("Stub coaching marker 0005"), `${label}: model text leaked into next`);
      const history = parse((await callDB(["tutor", "history", "--json"], db)).stdout);
      check(numberField(field(history, "summary"), "calls") === 1, `${label}: tutor history did not count the call`);
      check(numberField(field(history, "summary"), "ok") === 1, `${label}: tutor history did not count a success`);
      check(arrayField(history, "calls").length === 1, `${label}: tutor history call list`);
      check(
        !(arrayField(history, "calls")[0] && "output_text" in (arrayField(history, "calls")[0] as object)),
        `${label}: tutor history leaks model text`,
      );
      const show = parse((await callDB(["tutor", "show", "1", "--json"], db)).stdout);
      check(stringField(show, "state") === "advisory", `${label}: tutor show state`);
      check((stringField(field(show, "suggestion"), "text") ?? "").includes("Stub coaching marker 0005"), `${label}: tutor show text`);

      const importResult = await call([
        "import",
        "--path",
        path.join(ctx.root, "internal", "cli", "testdata", "import"),
        "--json",
        "--db",
        db,
      ]);
      seenOutputs.push(importResult.stdout, importResult.stderr);
      if (!check(importResult.code === 0, `${label}: import exit = ${importResult.code}`)) return;
      const sourceID = stringField(field(parse(importResult.stdout), "source"), "id");
      if (!check(sourceID !== undefined, `${label}: import did not return a source id`)) return;
      const questionFile = path.join(ctx.root, "internal", "cli", "testdata", "import", "question-q2-tagged.json");
      for (const args of [
        ["question", "add", sourceID!, "1", "--file", questionFile, "--json"],
        ["page", "review", sourceID!, "1", "--reviewer", "smoke", "--json"],
        ["question", "review", sourceID!, "q2", "--answer", "C", "--reviewer", "smoke", "--json"],
        ["practice", "--source", sourceID!, "--page", "1", "--question", "q2", "--answer", "C", "--json"],
      ]) {
        const result = await callDB(args, db);
        check(result.code === 0, `${label}: ${args.join(" ")} exit = ${result.code}`);
      }

      const privatePreviewResult = await callDB(["tutor", "preview", "--json"], db);
      const privatePreview = parse(privatePreviewResult.stdout);
      check(field(field(privatePreview, "context"), "private_source") === true, `${label}: source preview is not private`);
      check(
        field(field(privatePreview, "context"), "consent_granted") === false,
        `${label}: source preview has consent before a grant`,
      );
      check(
        field(field(privatePreview, "context"), "source_text_shared") === false,
        `${label}: withheld source preview marks text shared`,
      );
      const privateUser = previewUserText(privatePreview);
      for (const secret of [
        "The Westbridge Community Library closes at 4 p.m. on Saturdays.",
        "Which of the following is NOT stated",
        "The library closes at 4 p.m. on Saturdays.",
      ]) {
        check(!privateUser.includes(secret), `${label}: withheld source text leaked: ${secret.slice(0, 32)}`);
      }
      check(
        arrayField(field(privatePreview, "context"), "withheld").includes("passage"),
        `${label}: withheld fields do not name the passage`,
      );
      check(received.length === beforePreview + 1, `${label}: preview of a private source contacted the provider`);

      const consent = await callDB(["tutor", "consent", sourceID!, "--reviewer", "smoke", "--json"], db);
      check(consent.code === 0, `${label}: tutor consent exit = ${consent.code}`);
      const authorized = parse((await callDB(["tutor", "preview", "--json"], db)).stdout);
      check(field(field(authorized, "context"), "consent_granted") === true, `${label}: consent did not grant access`);
      check(
        field(field(authorized, "context"), "source_text_shared") === true,
        `${label}: authorized preview does not share source text`,
      );
      check(
        previewUserText(authorized).includes("The Westbridge Community Library closes at 4 p.m. on Saturdays."),
        `${label}: authorized preview omits the page text`,
      );
      const grantedConsentsHuman = await callDB(["tutor", "consents"], db);
      check(
        grantedConsentsHuman.stdout.includes(`  ${sourceID}  import  authorized`),
        `${label}: human consents does not label a granted source as authorized`,
      );

      const sourceAsk = await callDB(["tutor", "ask", "--json"], db);
      if (!check(sourceAsk.code === 0, `${label}: source tutor ask exit = ${sourceAsk.code}`)) return;
      const sourceCall = field(parse(sourceAsk.stdout), "call");
      check(field(sourceCall, "source_text_shared") === true, `${label}: source ask record does not mark shared text`);
      check(received.length === beforePreview + 2, `${label}: authorized source ask did not contact the provider`);
      check(
        JSON.stringify(received[received.length - 1].body).includes("The Westbridge Community Library closes at 4 p.m. on Saturdays."),
        `${label}: provider did not receive authorized page text`,
      );
      const sharedHistoryHuman = await callDB(["tutor", "history"], db);
      check(
        sharedHistoryHuman.stdout.includes("source-text-shared"),
        `${label}: human history does not label shared private text`,
      );

      // The refinement path builds its own wire document; the recorded consent
      // for this source and provider is in force here, before the revoke.
      const wrongPractice = await callDB(
        ["practice", "--source", sourceID!, "--page", "1", "--question", "q2", "--answer", "A", "--json"],
        db,
      );
      check(wrongPractice.code === 0, `${label}: wrong practice exit = ${wrongPractice.code}`);
      const attemptID = numberField(field(parse(wrongPractice.stdout), "attempt"), "id") ?? 0;
      const refineResult = await callDB(["note", "propose", "--attempt", String(attemptID), "--refine", "--json"], db);
      if (!check(refineResult.code === 0, `${label}: note propose --refine exit = ${refineResult.code}\n${refineResult.stderr}`)) return;
      const refine = parse(refineResult.stdout);
      const refinement = field(refine, "refinement");
      check(stringField(refinement, "model") === "smoke-model", `${label}: refinement model`);
      check(field(refinement, "advisory") === true, `${label}: refinement is not marked advisory`);
      check(field(field(refine, "draft"), "origin") === "refined", `${label}: refinement did not refine the draft`);
      check(received.length === beforePreview + 3, `${label}: refinement did not contact the provider`);
      const refineRequest = received[received.length - 1];
      check(refineRequest.path === wire.suffix, `${label}: refinement path = ${refineRequest.path}`);
      check(
        stringField(refinement, "request_sha256") === createHash("sha256").update(refineRequest.raw).digest("hex"),
        `${label}: refinement hash is not the posted body`,
      );
      if (isResponses) {
        check(stringField(refineRequest.body, "instructions") !== undefined, `${label}: refinement has no instructions`);
        check(arrayField(refineRequest.body, "input").length === 1, `${label}: refinement input items`);
        check(numberField(refineRequest.body, "max_output_tokens") === 350, `${label}: refinement max_output_tokens`);
        check(field(refineRequest.body, "store") === false, `${label}: refinement store is not false`);
        check(field(refineRequest.body, "messages") === undefined, `${label}: refinement carries chat messages`);
      } else {
        check(arrayField(refineRequest.body, "messages").length === 2, `${label}: refinement messages`);
        check(field(refineRequest.body, "instructions") === undefined, `${label}: refinement carries instructions`);
      }

      const revoke = await callDB(["tutor", "revoke", sourceID!, "--reviewer", "smoke", "--json"], db);
      check(revoke.code === 0, `${label}: tutor revoke exit = ${revoke.code}`);
      const revoked = parse((await callDB(["tutor", "preview", "--json"], db)).stdout);
      check(field(field(revoked, "context"), "consent_granted") === false, `${label}: revoke did not withhold access`);
      check(
        !previewUserText(revoked).includes("The Westbridge Community Library closes at 4 p.m."),
        `${label}: revoked source text is still in the prompt`,
      );
      const consents = parse((await callDB(["tutor", "consents", "--json"], db)).stdout);
      check(arrayField(consents, "sources").length === 1, `${label}: tutor consents did not list the source`);
      check(field(arrayField(consents, "sources")[0], "granted") === false, `${label}: tutor consents effective state after revoke`);
      const revokedConsentsHuman = await callDB(["tutor", "consents"], db);
      check(
        revokedConsentsHuman.stdout.includes(`  ${sourceID}  import  withheld`),
        `${label}: human consents does not label a revoked source as withheld`,
      );

      // A response in the other format's shape is a failure, both directions:
      // the format the path selects is the only one the host accepts.
      const wrongFormatCalls = received.length;
      mode = "wrong_format";
      const wrongFormat = await callDB(["tutor", "ask", "--json"], db);
      check(wrongFormat.code === 3, `${label}: wrong-format answer exit = ${wrongFormat.code}`);
      check(
        stringField(field(parse(wrongFormat.stderr), "error"), "code") === "invalid_response",
        `${label}: wrong-format answer code`,
      );
      check(received.length === wrongFormatCalls + 1, `${label}: wrong-format answer did not reach the provider`);

      const reportBeforeFailure = await callDB(["report", "--json"], db);
      mode = "error";
      const providerFailure = await callDB(["tutor", "ask", "--json"], db);
      check(providerFailure.code === 3, `${label}: provider failure exit = ${providerFailure.code}`);
      check(
        stringField(field(parse(providerFailure.stderr), "error"), "code") === "provider_error",
        `${label}: provider failure code`,
      );
      const reportAfterFailure = await callDB(["report", "--json"], db);
      check(reportBeforeFailure.stdout === reportAfterFailure.stdout, `${label}: failed provider call changed the attempt report`);

      mode = "auth";
      const authFailure = await callDB(["tutor", "ask", "--json"], db);
      check(authFailure.code === 3, `${label}: auth failure exit = ${authFailure.code}`);
      check(stringField(field(parse(authFailure.stderr), "error"), "code") === "auth_failed", `${label}: auth failure code`);

      mode = "malformed";
      const malformed = await callDB(["tutor", "ask", "--json"], db);
      check(malformed.code === 3, `${label}: malformed failure exit = ${malformed.code}`);
      check(
        stringField(field(parse(malformed.stderr), "error"), "code") === "invalid_response",
        `${label}: malformed response code`,
      );

      if (isResponses) {
        const incompleteCalls = received.length;
        mode = "incomplete";
        const incomplete = await callDB(["tutor", "ask", "--json"], db);
        check(incomplete.code === 3, `${label}: incomplete answer exit = ${incomplete.code}`);
        const incompleteError = field(parse(incomplete.stderr), "error");
        check(stringField(incompleteError, "code") === "invalid_response", `${label}: incomplete answer code`);
        check(
          (stringField(incompleteError, "message") ?? "").includes("max_output_tokens"),
          `${label}: incomplete answer does not name the reason: ${stringField(incompleteError, "message")}`,
        );
        check(received.length === incompleteCalls + 1, `${label}: incomplete answer did not reach the provider`);
      }

      mode = "ok";
      const requestsBeforeNode = received.length;
      const missingNode = await callDB(["tutor", "ask", "--json"], db, {
        PASSQUEST_NODE: path.join(temp, "missing-node"),
      });
      check(missingNode.code === 3, `${label}: missing node exit = ${missingNode.code}`);
      check(stringField(field(parse(missingNode.stderr), "error"), "code") === "agent_unavailable", `${label}: missing node code`);
      check(received.length === requestsBeforeNode, `${label}: missing node still contacted the provider`);
      const practiceAfterFailure = await callDB(["practice", "--question", "q3", "--answer", "B", "--json"], db);
      check(practiceAfterFailure.code === 0, `${label}: practice after tutor failure exit = ${practiceAfterFailure.code}`);

      // Recorded failures: the wrong-format, provider-error, auth, malformed and
      // missing-node asks every format proves, plus the incomplete answer the
      // responses format adds. The refinement records its own row, not a tutor
      // call, so the ledger holds the two successful asks beside them.
      const expectedCalls = isResponses ? 8 : 7;
      const expectedFailures = isResponses ? 6 : 5;
      const mainHistory = parse((await callDB(["tutor", "history", "--json"], db)).stdout);
      check(numberField(field(mainHistory, "summary"), "calls") === expectedCalls, `${label}: tutor history call total`);
      check(numberField(field(mainHistory, "summary"), "ok") === 2, `${label}: tutor history success total`);
      check(numberField(field(mainHistory, "summary"), "failed") === expectedFailures, `${label}: tutor history failure total`);
      check(numberField(field(mainHistory, "summary"), "prompt_tokens") === 20, `${label}: tutor history prompt tokens`);
      check(numberField(field(mainHistory, "summary"), "completion_tokens") === 10, `${label}: tutor history completion tokens`);
      check(arrayField(mainHistory, "calls").length === expectedCalls, `${label}: tutor history rows`);

      const failureDB = path.join(temp, `${label}-failure.sqlite`);
      await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--timeout-seconds", "1", "--json"],
        failureDB,
      );
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], failureDB);
      const failureReport = await callDB(["report", "--json"], failureDB);

      mode = "rate_limited";
      const rateBefore = received.length;
      const rateLimited = await callDB(["tutor", "ask", "--json"], failureDB);
      check(rateLimited.code === 3, `${label}: rate limited exit = ${rateLimited.code}`);
      check(stringField(field(parse(rateLimited.stderr), "error"), "code") === "rate_limited", `${label}: rate limited code`);
      check(received.length === rateBefore + 1, `${label}: rate limited request did not reach the provider`);
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        `${label}: rate limited call changed the attempt report`,
      );

      mode = "timeout";
      const timeout = await callDB(["tutor", "ask", "--json"], failureDB);
      check(timeout.code === 3, `${label}: timeout exit = ${timeout.code}`);
      check(stringField(field(parse(timeout.stderr), "error"), "code") === "timeout", `${label}: timeout code`);
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        `${label}: timed out call changed the attempt report`,
      );

      const closed = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch() { return new Response(""); } });
      const closedPort = closed.port;
      closed.stop(true);
      await callDB(["tutor", "config", "--endpoint", `http://127.0.0.1:${closedPort}${wire.suffix}`, "--json"], failureDB);
      mode = "ok";
      const networkFailure = await callDB(["tutor", "ask", "--json"], failureDB);
      check(networkFailure.code === 3, `${label}: network failure exit = ${networkFailure.code}`);
      check(stringField(field(parse(networkFailure.stderr), "error"), "code") === "network_error", `${label}: network failure code`);
      check(
        (await callDB(["report", "--json"], failureDB)).stdout === failureReport.stdout,
        `${label}: network failure changed the attempt report`,
      );
      const failureHistory = parse((await callDB(["tutor", "history", "--json"], failureDB)).stdout);
      check(numberField(field(failureHistory, "summary"), "calls") === 3, `${label}: failure history call count`);
      check(numberField(field(failureHistory, "summary"), "failed") === 3, `${label}: failure history failure count`);

      const credentialDB = path.join(temp, `${label}-credential.sqlite`);
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], credentialDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], credentialDB);

      mode = "error_with_key";
      const echoedError = await callDB(["tutor", "ask", "--json"], credentialDB);
      check(echoedError.code === 3, `${label}: credential error exit = ${echoedError.code}`);
      check(stringField(field(parse(echoedError.stderr), "error"), "code") === "provider_error", `${label}: credential error code`);
      check(!echoedError.stdout.includes(key) && !echoedError.stderr.includes(key), `${label}: provider error echoed the credential`);
      check(echoedError.stderr.includes("[redacted]"), `${label}: provider error was not redacted`);

      mode = "ok";
      const fakeHost = path.join(temp, `${label}-fake-host.mjs`);
      writeFileSync(
        fakeHost,
        'process.stderr.write("host failed with " + (process.env.PASSQUEST_TUTOR_API_KEY || ""));\nprocess.exit(1);\n',
      );
      const hostFailure = await callDB(["tutor", "ask", "--json"], credentialDB, { PASSQUEST_TUTOR_HOST: fakeHost });
      check(hostFailure.code === 3, `${label}: host stderr failure exit = ${hostFailure.code}`);
      check(stringField(field(parse(hostFailure.stderr), "error"), "code") === "agent_unavailable", `${label}: host stderr failure code`);
      check(!hostFailure.stdout.includes(key) && !hostFailure.stderr.includes(key), `${label}: host stderr echoed the credential`);
      check(hostFailure.stderr.includes("[redacted]"), `${label}: host stderr credential was not redacted`);
      const credentialBytes = readFileSync(credentialDB);
      check(!credentialBytes.includes(Buffer.from(key)), `${label}: credential database bytes contain the credential`);

      const budgetDB = path.join(temp, `${label}-budget.sqlite`);
      await callDB(
        ["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--daily-calls", "1", "--json"],
        budgetDB,
      );
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], budgetDB);
      const budgetBefore = received.length;
      const budgetFirst = await callDB(["tutor", "ask", "--json"], budgetDB);
      if (!check(budgetFirst.code === 0, `${label}: budget first ask exit = ${budgetFirst.code}`)) return;
      const budgetSecond = await callDB(["tutor", "ask", "--json"], budgetDB);
      check(budgetSecond.code === 2, `${label}: budget second ask exit = ${budgetSecond.code}`);
      check(stringField(field(parse(budgetSecond.stderr), "error"), "code") === "budget_exhausted", `${label}: budget error code`);
      check(received.length === budgetBefore + 1, `${label}: exhausted budget still contacted the provider`);
      const budgetHistory = parse((await callDB(["tutor", "history", "--json"], budgetDB)).stdout);
      check(numberField(field(budgetHistory, "summary"), "calls") === 1, `${label}: budget history call count`);
      check(numberField(field(budgetHistory, "summary"), "daily_limit") === 1, `${label}: budget history daily limit`);

      const humanDB = path.join(temp, `${label}-human.sqlite`);
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], humanDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], humanDB);
      const humanAsk = await callDB(["tutor", "ask"], humanDB);
      if (!check(humanAsk.code === 0, `${label}: human tutor ask exit = ${humanAsk.code}`)) return;
      for (const want of [
        "Pass Quest — advisory AI suggestion (not a verified answer)",
        "--- AI suggestion (advisory) ---",
        "This is model output. It is not a verified answer, grade or exam fact.",
        "Privacy: first-party material only; no private source text is involved",
        "Content: the current passage, question, options, verified key and evidence are included.",
        "Stub coaching marker 0005",
      ]) {
        check(humanAsk.stdout.includes(want), `${label}: human advisory output is missing ${want}`);
      }
      check(!humanAsk.stdout.includes(key), `${label}: human advisory output contains the credential`);
      const humanShow = await callDB(["tutor", "show", "1"], humanDB);
      check(humanShow.stdout.includes("Pass Quest — advisory call #1"), `${label}: human show does not identify the call`);
      check(
        humanShow.stdout.includes("This stored text is model output, not a verified answer, grade or exam fact."),
        `${label}: human show omits the advisory disclaimer`,
      );
      const humanHistory = await callDB(["tutor", "history"], humanDB);
      check(humanHistory.stdout.includes("first-party"), `${label}: human history does not label first-party calls`);

      // The host posts request.body verbatim: a body with &, < and > must
      // arrive byte-identical, so the recorded SHA-256 can match provider logs.
      const specialDocument = isResponses
        ? hostResponses("Marks & Spencer <tag> & more")
        : hostChat("Marks & Spencer <tag> & more");
      const specialBody = JSON.stringify(specialDocument);
      const specialCalls = received.length;
      check(
        field(await host(hostDocument(wire, specialDocument, specialBody)), "status") === "ok",
        `${label}: host rejected a valid special-character body`,
      );
      check(received.length === specialCalls + 1, `${label}: special-character body did not reach the provider`);
      check(received[received.length - 1].raw === specialBody, `${label}: host did not post the request body verbatim`);
      check(received[received.length - 1].path === wire.suffix, `${label}: special-character body path`);

      // 15,000 astral code points are 30,000 UTF-16 units; the documented
      // 20,000-character bound counts code points.
      const astralDocument = isResponses ? hostResponses("🚀".repeat(15000)) : hostChat("🚀".repeat(15000));
      check(
        field(await host(hostDocument(wire, astralDocument)), "status") === "ok",
        `${label}: host rejected a 15000-code-point astral prompt`,
      );

      const boundsDB = path.join(temp, `${label}-bounds.sqlite`);
      await callDB(["tutor", "config", "--endpoint", endpoint, "--model", "smoke-model", "--json"], boundsDB);
      await callDB(["practice", "--question", "q1", "--answer", "B", "--json"], boundsDB);
      mode = "astral";
      const astral = await callDB(["tutor", "ask", "--json"], boundsDB);
      mode = "ok";
      if (!check(astral.code === 0, `${label}: astral response exit = ${astral.code}`)) return;
      check(
        numberField(field(parse(astral.stdout), "call"), "output_chars") === 12000,
        `${label}: a 12000-code-point astral response was rejected or clipped`,
      );

      const dbBytes = readFileSync(db);
      check(!dbBytes.includes(Buffer.from(key)), `${label}: main database bytes contain the credential`);
      const budgetBytes = readFileSync(budgetDB);
      check(!budgetBytes.includes(Buffer.from(key)), `${label}: budget database bytes contain the credential`);
    }
  },
};
