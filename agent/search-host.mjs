#!/usr/bin/env node
// Pass Quest search host.
//
// Reads one versioned JSON request from stdin, posts the exact bounded body it
// carries to the configured search endpoint, and writes one versioned JSON
// result to stdout. It is the product's only search egress. It never logs, never
// writes the credential anywhere except the Authorization header, never follows
// a redirect and never fetches a result URL. See docs/find.md and
// agent/README.md.

const PROTOCOL_VERSION = 1;
const CREDENTIAL_ENV = "PASSQUEST_SEARCH_KEY";
const MAX_STDIN_BYTES = 1 << 20;
const MAX_BODY_BYTES = 1 << 20;
const MAX_ERROR_CHARS = 300;

const credential = (process.env[CREDENTIAL_ENV] || "").trim();

function sanitize(value) {
  const text = typeof value === "string" ? value : String(value ?? "");
  const clean = text.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/g, " ");
  return credential ? clean.split(credential).join("[redacted]") : clean;
}

function failure(code, message, extra = {}) {
  return {
    protocol_version: PROTOCOL_VERSION,
    status: "error",
    error: { code, message: sanitize(message).slice(0, MAX_ERROR_CHARS) },
    ...extra,
  };
}

function loopback(hostname) {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "0:0:0:0:0:0:0:1") {
    return true;
  }
  const mapped = /^::ffff:(.+)$/i.exec(host);
  if (!mapped) return false;
  const tail = mapped[1];
  if (tail.includes(".")) {
    const parts = tail.split(".");
    return parts.length === 4 && parts[0] === "127" && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
  }
  const hex = /^([0-9a-f]{1,4}):([0-9a-f]{1,4})$/i.exec(tail);
  if (!hex) return false;
  const value = (parseInt(hex[1], 16) << 16) | parseInt(hex[2], 16);
  return (value >>> 24) === 127;
}

function validateEndpoint(raw) {
  if (typeof raw !== "string" || raw.trim() === "") return { error: "endpoint is required" };
  let url;
  try {
    url = new URL(raw.trim());
  } catch {
    return { error: "endpoint is not a URL" };
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return { error: "endpoint must be http or https" };
  if (url.username || url.password) return { error: "endpoint must not contain userinfo" };
  if (url.search) return { error: "endpoint must not contain a query string" };
  if (url.hash) return { error: "endpoint must not contain a fragment" };
  if (url.protocol === "http:" && !loopback(url.hostname)) {
    return { error: "endpoint must use https off-loopback" };
  }
  return { url };
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

function codePointLength(text) {
  return Array.from(text).length;
}

function validateBody(body, query, limit) {
  let parsed;
  try {
    parsed = JSON.parse(body);
  } catch {
    return "body is not valid JSON";
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return "body must be an object";
  const keys = Object.keys(parsed).sort().join(",");
  if (keys !== "limit,query") return "body must carry exactly query and limit";
  if (typeof parsed.query !== "string" || parsed.query.trim() === "") return "body query is required";
  if (parsed.query !== query) return "body query does not match the request";
  if (!Number.isInteger(parsed.limit) || parsed.limit < 1 || parsed.limit > 100) return "body limit is invalid";
  if (parsed.limit !== limit) return "body limit does not match the request";
  if (Buffer.byteLength(body) > MAX_STDIN_BYTES) return "body exceeds 1 MiB";
  return null;
}

function validateRequest(request) {
  if (!request || typeof request !== "object" || Array.isArray(request)) return "request must be an object";
  if (request.protocol_version !== PROTOCOL_VERSION) return `protocol_version must be ${PROTOCOL_VERSION}`;
  if (typeof request.endpoint !== "string" || request.endpoint.trim() === "") return "endpoint is required";
  if (!Number.isInteger(request.timeout_ms) || request.timeout_ms < 1000 || request.timeout_ms > 120000) {
    return "timeout_ms must be an integer in 1000..120000";
  }
  if (typeof request.query !== "string" || request.query.trim() === "") return "query is required";
  if (codePointLength(request.query) > 300) return "query exceeds 300 characters";
  if (/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/.test(request.query)) {
    return "query contains a terminal control character";
  }
  if (!Number.isInteger(request.limit) || request.limit < 1 || request.limit > 100) {
    return "limit must be an integer in 1..100";
  }
  if (typeof request.body !== "string" || request.body.trim() === "") return "body is required";
  return validateBody(request.body, request.query, request.limit);
}

async function readLimitedBody(response, limit) {
  if (!response.body) {
    const text = await response.text();
    if (Buffer.byteLength(text) > limit) throw new Error("__body_too_large__");
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

  // The body was validated against the typed query and limit above; posting
  // these exact bytes keeps the client's recorded SHA-256 and size truthful.
  const body = request.body;
  const headers = {
    "content-type": "application/json",
    accept: "application/json",
    "user-agent": "passquest-search/1",
  };
  if (credential) headers.authorization = `Bearer ${credential}`;

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), request.timeout_ms);
  let response;
  let responseBody = "";
  try {
    response = await fetch(endpoint.url, {
      method: "POST",
      redirect: "error",
      signal: controller.signal,
      headers,
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
    if (status === 401 || status === 403) {
      return failure("auth_failed", `provider rejected the request (HTTP ${status})`, {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    if (status === 429) {
      return failure("rate_limited", "provider rate limit reached (HTTP 429)", {
        http_status: status,
        latency_ms: latency,
        request_sent: true,
      });
    }
    return failure("provider_error", `provider returned HTTP ${status}`, {
      http_status: status,
      latency_ms: latency,
      request_sent: true,
    });
  }

  return {
    protocol_version: PROTOCOL_VERSION,
    status: "ok",
    http_status: status,
    latency_ms: latency,
    request_sent: true,
    body: responseBody,
  };
}

main()
  .then((result) => {
    process.stdout.write(`${JSON.stringify(result)}\n`);
  })
  .catch((error) => {
    process.stdout.write(
      `${JSON.stringify(failure("host_error", error instanceof Error ? error.message : String(error)))}`,
    );
  });
