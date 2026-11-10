#!/usr/bin/env node
// Pass Quest tutor host.
//
// Reads one versioned JSON request from stdin, sends one OpenAI-compatible
// non-streaming chat request, and writes one versioned JSON result to stdout.
// This file is the product's only network egress. It never logs, never writes
// the credential anywhere except the Authorization header, and never follows a
// redirect. See docs/tutor.md and agent/README.md.

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
  const chat = request.chat;
  if (!chat || typeof chat !== "object" || Array.isArray(chat)) return "chat is required";
  if (typeof chat.model !== "string" || chat.model.trim() === "") return "chat.model is required";
  if (!Array.isArray(chat.messages) || chat.messages.length < 2) return "chat.messages must have at least two messages";
  let total = 0;
  for (const message of chat.messages) {
    if (!message || typeof message !== "object") return "every chat message must be an object";
    if (typeof message.role !== "string" || message.role.trim() === "") return "every chat message needs a role";
    if (typeof message.content !== "string" || message.content.trim() === "") return "every chat message needs content";
    total += message.content.length;
  }
  if (total > 20000) return "chat messages exceed the local size bound";
  if (!Number.isInteger(chat.max_tokens) || chat.max_tokens < 64 || chat.max_tokens > 2048) return "chat.max_tokens is invalid";
  if (chat.temperature !== 0) return "chat.temperature must be 0";
  if (chat.stream !== false) return "chat.stream must be false";
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

  const endpoint = validateEndpoint(request.endpoint);
  if (endpoint.error) return failure("config_error", endpoint.error, { latency_ms: Date.now() - started });
  if (!credential) return failure("missing_credential", `${CREDENTIAL_ENV} is not set`, { latency_ms: Date.now() - started });

  const chat = request.chat;
  const body = JSON.stringify({
    model: chat.model,
    messages: chat.messages.map((message) => ({ role: message.role, content: message.content })),
    max_tokens: chat.max_tokens,
    temperature: 0,
    stream: false,
  });

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
    if (aborted) return failure("timeout", `provider did not answer within ${request.timeout_ms} ms`, { latency_ms: latency });
    if (error instanceof Error && error.message === "__body_too_large__") {
      return failure("invalid_response", "provider response exceeds 1 MiB", { latency_ms: latency });
    }
    return failure("network_error", error instanceof Error ? error.message : String(error), { latency_ms: latency });
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
      });
    }
    if (status === 429) {
      return failure("rate_limited", message || "provider rate limit reached (HTTP 429)", {
        http_status: status,
        latency_ms: latency,
      });
    }
    return failure("provider_error", message || `provider returned HTTP ${status}`, {
      http_status: status,
      latency_ms: latency,
    });
  }

  let data;
  try {
    data = JSON.parse(responseBody);
  } catch {
    return failure("invalid_response", "provider response is not JSON", { http_status: status, latency_ms: latency });
  }
  const content = data && data.choices && data.choices[0] && data.choices[0].message
    ? data.choices[0].message.content
    : undefined;
  if (typeof content !== "string" || content.trim() === "") {
    return failure("invalid_response", "provider response has no text content", {
      http_status: status,
      latency_ms: latency,
    });
  }
  if (content.length > request.max_response_chars) {
    return failure("invalid_response", "provider text exceeds the configured bound", {
      http_status: status,
      latency_ms: latency,
    });
  }
  if (/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/.test(content)) {
    return failure("invalid_response", "provider text contains a terminal control character", {
      http_status: status,
      latency_ms: latency,
    });
  }

  const result = {
    protocol_version: PROTOCOL_VERSION,
    status: "ok",
    text: content.trim(),
    http_status: status,
    latency_ms: latency,
  };
  const usage = data.usage;
  if (
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
