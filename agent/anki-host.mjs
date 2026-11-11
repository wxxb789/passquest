#!/usr/bin/env node
// Pass Quest Anki host.
//
// Reads one versioned JSON request from stdin, posts each allow-listed
// AnkiConnect action to a loopback endpoint, and writes one versioned JSON
// result to stdout. It is the product's only Anki egress. It never logs note
// text, never follows a redirect and never accepts a non-loopback endpoint.
// See docs/notes.md and agent/README.md.

const PROTOCOL_VERSION = 1;
const MAX_STDIN_BYTES = 1 << 20;
const MAX_RESPONSE_BYTES = 1 << 20;
const MAX_ACTIONS = 50;
const ALLOWED_ACTIONS = new Set([
  "version",
  "createDeck",
  "modelNames",
  "createModel",
  "addNote",
  "updateNoteFields",
  "notesInfo",
]);

function failure(message) {
  return { protocol_version: PROTOCOL_VERSION, state: "failed", results: [], error: String(message).slice(0, 300) };
}

async function readStdin() {
  const chunks = [];
  let total = 0;
  for await (const chunk of process.stdin) {
    total += chunk.length;
    if (total > MAX_STDIN_BYTES) throw new Error("request exceeds 1 MiB");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

function ipv4Loopback(parts) {
  return parts.length === 4 && parts[0] === "127" && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

function loopback(hostname) {
  const host = hostname.startsWith("[") && hostname.endsWith("]") ? hostname.slice(1, -1) : hostname;
  if (host.toLowerCase() === "localhost") return true;
  if (host === "::1") return true;
  // Go's netip.IsLoopback accepts an IPv4-mapped loopback; URL serializes it as
  // ::ffff:7f00:1, so both the dotted and the hex tail must be accepted here.
  const mapped = /^::ffff:(.+)$/i.exec(host);
  if (mapped) {
    const tail = mapped[1];
    if (tail.includes(".")) return ipv4Loopback(tail.split("."));
    const hex = /^([0-9a-f]{1,4}):([0-9a-f]{1,4})$/i.exec(tail);
    if (!hex) return false;
    const high = parseInt(hex[1], 16);
    const low = parseInt(hex[2], 16);
    return ipv4Loopback([String(high >> 8), String(high & 255), String(low >> 8), String(low & 255)]);
  }
  const parts = host.split(".");
  return parts.length === 4 && ipv4Loopback(parts);
}

function validateEndpoint(raw) {
  if (typeof raw !== "string" || raw.trim() === "") throw new Error("endpoint is required");
  let url;
  try {
    url = new URL(raw.trim());
  } catch {
    throw new Error("endpoint is not a URL");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") throw new Error("endpoint must be http or https");
  if (url.username || url.password) throw new Error("endpoint must not contain userinfo");
  if (url.search || url.hash) throw new Error("endpoint must not contain a query or fragment");
  if (url.pathname !== "" && url.pathname !== "/") throw new Error("endpoint path must be empty");
  if (!loopback(url.hostname)) throw new Error("endpoint must be loopback");
  return url.toString();
}

function validateRequest(request) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request must be an object");
  if (request.protocol_version !== PROTOCOL_VERSION) throw new Error("unsupported protocol_version");
  const endpoint = validateEndpoint(request.endpoint);
  if (!Number.isInteger(request.timeout_ms) || request.timeout_ms < 100 || request.timeout_ms > 120000) {
    throw new Error("timeout_ms must be an integer in 100..120000");
  }
  if (!Array.isArray(request.actions) || request.actions.length < 1 || request.actions.length > MAX_ACTIONS) {
    throw new Error(`actions must be an array of 1..${MAX_ACTIONS}`);
  }
  let total = 0;
  const actions = request.actions.map((action, index) => {
    if (!action || typeof action !== "object" || Array.isArray(action)) {
      throw new Error(`action ${index + 1} must be an object`);
    }
    if (typeof action.action !== "string" || !ALLOWED_ACTIONS.has(action.action)) {
      throw new Error(`action ${index + 1} is not allowed`);
    }
    if (
      action.params !== undefined &&
      action.params !== null &&
      (typeof action.params !== "object" || Array.isArray(action.params))
    ) {
      throw new Error(`action ${index + 1} params must be an object`);
    }
    const params = action.params || {};
    total += Buffer.byteLength(JSON.stringify(params));
    return { action: action.action, params };
  });
  if (total > MAX_STDIN_BYTES) throw new Error("action params exceed 1 MiB");
  return { endpoint, timeout_ms: request.timeout_ms, actions };
}

async function readLimited(response, limit) {
  if (!response.body) {
    const text = await response.text();
    if (Buffer.byteLength(text) > limit) throw new Error("__too_large__");
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
        // The oversized response is already a failure.
      }
      throw new Error("__too_large__");
    }
    chunks.push(Buffer.from(value));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function ankiError(value) {
  if (typeof value === "string") return value.slice(0, 500);
  if (value === null || value === undefined) return "";
  return String(value).slice(0, 500);
}

async function postAction(endpoint, action, controller) {
  const body = JSON.stringify({ action: action.action, version: 6, params: action.params });
  let response;
  try {
    response = await fetch(endpoint, {
      method: "POST",
      redirect: "error",
      signal: controller.signal,
      headers: { "content-type": "application/json", accept: "application/json", "user-agent": "passquest-anki/1" },
      body,
    });
  } catch (error) {
    const reason = controller.signal.aborted
      ? "timeout"
      : error instanceof Error
        ? error.message
        : String(error);
    return { action: action.action, state: "unreachable", error: reason.slice(0, 500) };
  }
  let text;
  try {
    text = await readLimited(response, MAX_RESPONSE_BYTES);
  } catch (error) {
    const reason = error instanceof Error && error.message === "__too_large__"
      ? "response exceeds 1 MiB"
      : error instanceof Error
        ? error.message
        : String(error);
    return { action: action.action, state: "error", error: reason.slice(0, 500), http_status: response.status };
  }
  if (response.status < 200 || response.status > 299) {
    return { action: action.action, state: "error", error: `HTTP ${response.status}`, http_status: response.status };
  }
  let payload;
  try {
    payload = JSON.parse(text);
  } catch {
    return { action: action.action, state: "error", error: "response is not JSON", http_status: response.status };
  }
  const error = ankiError(payload && payload.error);
  if (error) {
    return { action: action.action, state: "error", error, http_status: response.status };
  }
  return {
    action: action.action,
    state: "ok",
    result: payload && Object.prototype.hasOwnProperty.call(payload, "result") ? payload.result : null,
    http_status: response.status,
  };
}

async function main() {
  const raw = await readStdin();
  let request;
  try {
    request = JSON.parse(raw);
  } catch {
    return failure("request is not valid JSON");
  }
  let valid;
  try {
    valid = validateRequest(request);
  } catch (error) {
    return failure(error instanceof Error ? error.message : String(error));
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), valid.timeout_ms);
  const results = [];
  try {
    for (let i = 0; i < valid.actions.length; i++) {
      const result = await postAction(valid.endpoint, valid.actions[i], controller);
      results.push(result);
      if (result.state === "unreachable") {
        for (let rest = i + 1; rest < valid.actions.length; rest++) {
          results.push({ action: valid.actions[rest].action, state: "unreachable", error: result.error });
        }
        break;
      }
    }
  } finally {
    clearTimeout(timer);
  }
  const unreachable = results.some((result) => result.state === "unreachable");
  const failed = results.some((result) => result.state === "error");
  return {
    protocol_version: PROTOCOL_VERSION,
    state: unreachable ? "unreachable" : failed ? "failed" : "ok",
    results,
  };
}

main()
  .then((result) => {
    process.stdout.write(`${JSON.stringify(result)}\n`);
  })
  .catch((error) => {
    process.stdout.write(`${JSON.stringify(failure(error instanceof Error ? error.message : String(error)))}\n`);
  });
