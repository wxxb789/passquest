/**
 * A loopback stand-in for the GitHub Releases endpoint the desktop app checks
 * (goal 0015), for the gates that drive the real window: a scripted launch
 * reaches it only through the smoke-only `PASSQUEST_DESKTOP_UPDATE_URL`
 * override, and its URL is built from the constant endpoint shared with the
 * local-only gate (lib/update-endpoint.ts), so a captured request can be
 * compared with `internal/update.Endpoint` field by field. It records every
 * request and answers from a queue that cycles, so one scripted launch can
 * walk the check's states in order and a second launch can walk the same
 * sequence again. Nothing here leaves 127.0.0.1.
 */

import { readFileSync } from "node:fs";

import { resolve } from "./repo.ts";
import { UPDATE_ENDPOINT } from "./update-endpoint.ts";

/**
 * The constant endpoint's path, bound to the `internal/update.Endpoint` literal in
 * `internal/update/update.go`: the stub serves, and compares captured requests against, the one
 * endpoint lib/update-endpoint.ts defines, so neither the source nor the harness can drift from
 * the other. A file that no longer declares the constant, or declares a different endpoint, throws
 * rather than serving a request the app would never make.
 */
function updatePath(root: string): string {
  const source = readFileSync(resolve(root, "internal/update/update.go"), "utf8");
  const literal = /^const Endpoint = "([^"]+)"$/m.exec(source)?.[1];
  if (!literal) throw new Error('internal/update/update.go: no `const Endpoint = "..."` line for the update stub to bind to');
  if (literal !== UPDATE_ENDPOINT) {
    throw new Error(`internal/update/update.go: Endpoint ${literal} is not ${UPDATE_ENDPOINT}, the endpoint the stub stands in for`);
  }
  return new URL(UPDATE_ENDPOINT).pathname;
}

export interface UpdateStubRequest {
  method: string;
  /** Path plus query, exactly as received. */
  path: string;
  /** Lower-cased header names to values, as received. */
  headers: Record<string, string>;
  /** The raw body; empty for the app's GET. */
  body: string;
}

export interface UpdateStub {
  /** The URL the app must be pointed at: the constant path on a 127.0.0.1 port. */
  endpoint: string;
  /** The constant endpoint's own path, what every captured request must carry. */
  path: string;
  requests: UpdateStubRequest[];
  /**
   * The answers, consumed in order and repeated from the start when
   * exhausted; each one receives the request it answers. A handler that
   * never settles holds the response open (the stalled-check state).
   */
  queue: Array<(request: UpdateStubRequest) => Response | Promise<Response>>;
  stop(): void;
}

/** A release document in the shape GitHub serves, as `internal/update` reads it. */
export function releaseResponse(release: { tag: string; published?: string; draft?: boolean; prerelease?: boolean }): Response {
  return Response.json({
    tag_name: release.tag,
    published_at: release.published ?? "2026-11-18T00:00:00Z",
    draft: release.draft ?? false,
    prerelease: release.prerelease ?? false,
  });
}

export function startUpdateStub(root: string): UpdateStub {
  const endpointPath = updatePath(root);
  const stub: UpdateStub = {
    endpoint: "",
    path: endpointPath,
    requests: [],
    queue: [() => releaseResponse({ tag: "v9.9.9" })],
    stop: () => server.stop(true),
  };
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const url = new URL(request.url);
      const entry: UpdateStubRequest = {
        method: request.method,
        path: url.pathname + url.search,
        headers: Object.fromEntries(Array.from(request.headers, ([name, value]) => [name.toLowerCase(), value])),
        body: await request.text(),
      };
      stub.requests.push(entry);
      return stub.queue[(stub.requests.length - 1) % stub.queue.length]!(entry);
    },
  });
  stub.endpoint = `http://127.0.0.1:${server.port}${endpointPath}`;
  return stub;
}
