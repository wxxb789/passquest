#!/usr/bin/env node
// Pass Quest tutor host.
//
// Reads one versioned JSON request from stdin, posts the exact request body it
// carries to one OpenAI-compatible non-streaming endpoint (chat completions or
// Responses, selected by the endpoint's path), and writes one versioned JSON
// result to stdout. This file is the product's only network egress. It never
// logs, never writes the credential anywhere except the Authorization header,
// and never follows a redirect. See docs/tutor.md and agent/README.md.

const PROTOCOL_VERSION = 1;
const CREDENTIAL_ENV = "PASSQUEST_TUTOR_API_KEY";
const MAX_STDIN_BYTES = 1 << 20;
const MAX_BODY_BYTES = 1 << 20;
const MAX_ERROR_CHARS = 300;

function loopback(hostname) {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "0:0:0:0:0:0:0:1";
}

function failure(code, message, extra = {}) {
  return {
    protocol_version: PROTOCOL_VERSION,
    status: "error",
    error: { code, message: sanitize(message).slice(0, MAX_ERROR_CHARS) },
    ...extra,
  };
}

const credential = (process.env[CREDENTIAL_ENV] || "").trim();

function sanitize(value) {
  const text = typeof value === "string" ? value : String(value ?? "");
  const clean = text.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/g, " ");
  return credential ? clean.split(credential).join("[redacted]") : clean;
}

async function readStdin() {
  const chunks = [];
  let total = 0;
  for await (const chunk of process.stdin) {
    total += chunk.length;
    if (total > MAX_STDIN_BYTES) throw new Error("host request exceeds 1 MiB");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

function validateEndpoint(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    return { error: "endpoint is not a URL" };
  }
  if (!url.host) return { error: "endpoint has no host" };
  if (url.username || url.password) return { error: "endpoint must not contain userinfo" };
  if (url.search) return { error: "endpoint must not contain a query string" };
  if (url.hash) return { error: "endpoint must not contain a fragment" };
  if (url.protocol !== "https:" && url.protocol !== "http:") return { error: "endpoint must be http or https" };
  if (url.protocol === "http:" && !loopback(url.hostname)) {
    return { error: "endpoint must use https off-loopback" };
  }
  return { url };
}

// endpointFormat returns "chat-completions" or "responses" for an endpoint
// whose path names one of the two wire forms, "" for a parsable path that
// names neither, and null for an endpoint validateEndpoint refuses.
function endpointFormat(raw) {
  if (typeof raw !== "string") return null;
  let url;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  const trimmed = url.pathname.replace(/\/+$/, "");
  if (trimmed.endsWith("/chat/completions")) return "chat-completions";
  if (trimmed.endsWith("/responses")) return "responses";
  return "";
}

const CHAT_BODY_KEYS = ["max_tokens", "messages", "model", "stream", "temperature"];
const RESPONSES_BODY_KEYS = ["instructions", "input", "max_output_tokens", "model", "store", "stream", "temperature"];

// canonicalBody reduces a request body to its five meaningful fields so the Go
// client's serialization and the parsed body can be compared as values.
function canonicalBody(value) {
  const messages = Array.isArray(value && value.messages)
    ? value.messages.map((message) => ({ role: message && message.role, content: message && message.content }))
    : null;
  return JSON.stringify({
    model: value && value.model,
    messages,
    max_tokens: value && value.max_tokens,
    temperature: value && value.temperature,
    stream: value && value.stream,
  });
}

// canonicalResponses reduces a Responses document to its meaningful fields so
// the Go client's serialization and the parsed body can be compared as values.
function canonicalResponses(value) {
  const input = Array.isArray(value && value.input)
    ? value.input.map((item) => ({
        role: item && item.role,
        content: Array.isArray(item && item.content)
          ? item.content.map((part) => ({ type: part && part.type, text: part && part.text }))
          : null,
      }))
    : null;
  return JSON.stringify({
    model: value && value.model,
    instructions: value && value.instructions,
    input,
    max_output_tokens: value && value.max_output_tokens,
    temperature: value && value.temperature,
    stream: value && value.stream,
    store: value && value.store,
  });
}

// sameBody reports whether the posted body carries exactly the validated wire
// document. The host rejects a mismatch instead of guessing which one to send.
function sameBody(posted, wire, keys, canonical) {
  if (!posted || typeof posted !== "object" || Array.isArray(posted)) return false;
  if (Object.keys(posted).sort().join(",") !== [...keys].sort().join(",")) return false;
  return canonical(posted) === canonical(wire);
}

function validateRequest(request) {
  if (!request || typeof request !== "object" || Array.isArray(request)) return "request must be a JSON object";
  if (request.protocol_version !== PROTOCOL_VERSION) return `protocol_version must be ${PROTOCOL_VERSION}`;
  if (typeof request.endpoint !== "string" || request.endpoint.trim() === "") return "endpoint is required";
  if (!Number.isInteger(request.timeout_ms) || request.timeout_ms < 1000 || request.timeout_ms > 120000) {
    return "timeout_ms must be an integer in 1000..120000";
  }
  if (!Number.isInteger(request.max_response_chars) || request.max_response_chars < 1 || request.max_response_chars > 12000) {
    return "max_response_chars must be an integer in 1..12000";
  }
  const endpoint = validateEndpoint(request.endpoint);
  if (endpoint.error) return endpoint.error;
  const format = endpointFormat(request.endpoint);
  if (format === "") return "endpoint must end in /chat/completions or /responses";
  const hasChat = request.chat !== undefined;
  const hasResponses = request.responses !== undefined;
  if (hasChat && hasResponses) return "request must carry exactly one of chat or responses";
  if (format === "responses") {
    if (!hasResponses) return "endpoint ends in /responses but the request carries a chat document";
    return validateResponsesRequest(request);
  }
  if (!hasChat) return "endpoint ends in /chat/completions but the request carries a responses document";
  const chat = request.chat;
  if (!chat || typeof chat !== "object" || Array.isArray(chat)) return "chat is required";
  if (typeof chat.model !== "string" || chat.model.trim() === "") return "chat.model is required";
  if (!Array.isArray(chat.messages) || chat.messages.length < 2) return "chat.messages must have at least two messages";
  let total = 0;
  for (const message of chat.messages) {
    if (!message || typeof message !== "object") return "every chat message must be an object";
    if (typeof message.role !== "string" || message.role.trim() === "") return "every chat message needs a role";
    if (typeof message.content !== "string" || message.content.trim() === "") return "every chat message needs content";
    // Code points, matching the Go client's utf8.RuneCountInString bound; a
    // UTF-16 length would reject astral text the documented bound allows.
    total += Array.from(message.content).length;
  }
  if (total > 20000) return "chat messages exceed the local size bound";
  if (!Number.isInteger(chat.max_tokens) || chat.max_tokens < 64 || chat.max_tokens > 2048) return "chat.max_tokens is invalid";
  if (chat.temperature !== 0) return "chat.temperature must be 0";
  if (chat.stream !== false) return "chat.stream must be false";
  if (typeof request.body !== "string" || request.body.trim() === "") return "body is required";
  if (Buffer.byteLength(request.body) > MAX_STDIN_BYTES) return "body exceeds 1 MiB";
  let posted;
  try {
    posted = JSON.parse(request.body);
  } catch {
    return "body is not valid JSON";
  }
  if (!sameBody(posted, chat, CHAT_BODY_KEYS, canonicalBody)) return "body does not match chat";
  return null;
}

function validateResponsesRequest(request) {
  const responses = request.responses;
  if (!responses || typeof responses !== "object" || Array.isArray(responses)) return "responses is required";
  if (typeof responses.model !== "string" || responses.model.trim() === "") return "responses.model is required";
  if (typeof responses.instructions !== "string" || responses.instructions.trim() === "") {
    return "responses.instructions is required";
  }
  if (!Array.isArray(responses.input) || responses.input.length < 1) return "responses.input must have at least one item";
  let total = Array.from(responses.instructions).length;
  for (const item of responses.input) {
    if (!item || typeof item !== "object" || Array.isArray(item)) return "every responses input item must be an object";
    if (typeof item.role !== "string" || item.role.trim() === "") return "every responses input item needs a role";
    if (!Array.isArray(item.content) || item.content.length < 1) return "every responses input item needs content";
    for (const part of item.content) {
      if (!part || typeof part !== "object" || Array.isArray(part)) return "every responses content part must be an object";
      if (part.type !== "input_text") return "every responses content part must be input_text";
      if (typeof part.text !== "string" || part.text.trim() === "") return "every responses content part needs text";
      total += Array.from(part.text).length;
    }
  }
  if (total > 20000) return "responses content exceeds the local size bound";
  if (!Number.isInteger(responses.max_output_tokens) || responses.max_output_tokens < 64 || responses.max_output_tokens > 2048) {
    return "responses.max_output_tokens is invalid";
  }
  if (responses.temperature !== 0) return "responses.temperature must be 0";
  if (responses.stream !== false) return "responses.stream must be false";
  if (responses.store !== false) return "responses.store must be false";
  if (typeof request.body !== "string" || request.body.trim() === "") return "body is required";
  if (Buffer.byteLength(request.body) > MAX_STDIN_BYTES) return "body exceeds 1 MiB";
  let posted;
  try {
    posted = JSON.parse(request.body);
  } catch {
    return "body is not valid JSON";
  }
  if (!sameBody(posted, responses, RESPONSES_BODY_KEYS, canonicalResponses)) return "body does not match responses";
  return null;
}

async function readLimitedBody(response, limit) {
  if (!response.body) {
    const text = await response.text();
    if (text.length > limit) throw new Error("__body_too_large__");
    return text;
  }
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > limit) {
      try {
        await reader.cancel();
      } catch {
        // The oversized response is already a failure; cancellation is best effort.
      }
      throw new Error("__body_too_large__");
    }
    chunks.push(Buffer.from(value));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function providerMessage(body) {
  try {
    const parsed = JSON.parse(body);
    const message = parsed && parsed.error && parsed.error.message;
    return typeof message === "string" ? message : "";
  } catch {
    return "";
  }
}

function responsesOutputText(data) {
  const output = Array.isArray(data && data.output) ? data.output : [];
  const parts = [];
  for (const item of output) {
    if (!item || item.type !== "message" || !Array.isArray(item.content)) continue;
    for (const part of item.content) {
      if (part && part.type === "output_text" && typeof part.text === "string") parts.push(part.text);
    }
  }
  return parts.join("");
}

function responsesRefused(data) {
  const output = Array.isArray(data && data.output) ? data.output : [];
  for (const item of output) {
    if (!item || item.type !== "message" || !Array.isArray(item.content)) continue;
    for (const part of item.content) {
      if (part && part.type === "refusal") return true;
    }
  }
  return false;
}

async function main() {
  const started = Date.now();
  const raw = await readStdin();
  let request;
  try {
    request = JSON.parse(raw);
  } catch {
    return failure("host_error", "request is not valid JSON", { latency_ms: Date.now() - started });
  }
  const invalid = validateRequest(request);
  if (invalid) return failure("config_error", invalid, { latency_ms: Date.now() - started });
  const format = endpointFormat(request.endpoint);

  const endpoint = validateEndpoint(request.endpoint);
  if (endpoint.error) return failure("config_error", endpoint.error, { latency_ms: Date.now() - started });
  if (!credential) return failure("missing_credential", `${CREDENTIAL_ENV} is not set`, { latency_ms: Date.now() - started });

  // The body was validated against its wire document above; posting these
  // exact bytes keeps the client's recorded SHA-256 and size truthful.
  const body = request.body;

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), request.timeout_ms);
  let response;
  let responseBody = "";
  try {
    response = await fetch(endpoint.url, {
      method: "POST",
      redirect: "error",
      signal: controller.signal,
      headers: {
        "content-type": "application/json",
        accept: "application/json",
        authorization: `Bearer ${credential}`,
        "user-agent": "passquest-tutor/1",
      },
      body,
    });
    responseBody = await readLimitedBody(response, MAX_BODY_BYTES);
  } catch (error) {
    const aborted = controller.signal.aborted;
    const latency = Date.now() - started;
    if (aborted) {
      return failure("timeout", `provider did not answer within ${request.timeout_ms} ms`, {
        latency_ms: latency,
        request_sent: true,
      });
    }
    if (error instanceof Error && error.message === "__body_too_large__") {
      return failure("invalid_response", "provider response exceeds 1 MiB", {
        latency_ms: latency,
        request_sent: true,
      });
    }
    return failure("network_error", error instanceof Error ? error.message : String(error), {
      latency_ms: latency,
      request_sent: true,
    });
  } finally {
    clearTimeout(timer);
  }

  const latency = Date.now() - started;
  const status = response.status;
  if (status < 200 || status > 299) {
    const message = providerMessage(responseBody);
    if (status === 401 || status === 403) {
      return failure("auth_failed", message || `provider rejected the credential (HTTP ${status})`, {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    if (status === 429) {
      return failure("rate_limited", message || "provider rate limit reached (HTTP 429)", {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    return failure("provider_error", message || `provider returned HTTP ${status}`, {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }

  let data;
  try {
    data = JSON.parse(responseBody);
  } catch {
    return failure("invalid_response", "provider response is not JSON", {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }
  let content;
  if (format === "responses") {
    const state = data && typeof data.status === "string" ? data.status : "unknown";
    if (state !== "completed") {
      const reason = data && data.incomplete_details && typeof data.incomplete_details.reason === "string"
        ? ` (${data.incomplete_details.reason})`
        : "";
      return failure("invalid_response", `provider response status is ${state}${reason}`, {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    const providerIssue = data && data.error && typeof data.error.message === "string" ? data.error.message.trim() : "";
    if (providerIssue !== "") {
      return failure("invalid_response", `provider response carried an error: ${providerIssue}`, {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    if (responsesRefused(data)) {
      return failure("invalid_response", "provider response refused to answer", {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    content = responsesOutputText(data);
  } else {
    content = data && data.choices && data.choices[0] && data.choices[0].message
      ? data.choices[0].message.content
      : undefined;
  }
  if (typeof content !== "string" || content.trim() === "") {
    return failure("invalid_response", "provider response has no text content", {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }
  const contentChars = Array.from(content).length;
  if (contentChars > request.max_response_chars) {
    return failure("invalid_response", "provider text exceeds the configured bound", {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }
  if (/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/.test(content)) {
    return failure("invalid_response", "provider text contains a terminal control character", {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }

  const result = {
    protocol_version: PROTOCOL_VERSION,
    status: "ok",
    text: content.trim(),
    http_status: status,
    latency_ms: latency,
    request_sent: true,
  };
  const usage = data.usage;
  if (format === "responses") {
    if (
      usage &&
      Number.isInteger(usage.input_tokens) &&
      Number.isInteger(usage.output_tokens) &&
      usage.input_tokens >= 0 &&
      usage.output_tokens >= 0
    ) {
      result.usage = { prompt_tokens: usage.input_tokens, completion_tokens: usage.output_tokens };
    }
  } else if (
    usage &&
    Number.isInteger(usage.prompt_tokens) &&
    Number.isInteger(usage.completion_tokens) &&
    usage.prompt_tokens >= 0 &&
    usage.completion_tokens >= 0
  ) {
    result.usage = { prompt_tokens: usage.prompt_tokens, completion_tokens: usage.completion_tokens };
  }
  return result;
}

main()
  .then((result) => {
    process.stdout.write(`${JSON.stringify(result)}\n`);
  })
  .catch((error) => {
    process.stdout.write(
      `${JSON.stringify(failure("host_error", error instanceof Error ? error.message : String(error)))}\n`,
    );
  });
