/**
 * The Updates row against a scripted stub (goal 0015, P4): the window's one
 * network check, walked through every state it can show, one check per stub
 * answer. The stub is the loopback stand-in of lib/update-stub.ts, reached
 * only through the smoke-only overrides; the page script presses the button
 * and reads the rendered row, and never clicks a link.
 *
 * The tags the stub serves are derived from the running version (mygo.json,
 * which a Go test ties to internal/update.Version): the "equal" answer is the
 * running version itself, so a version bump cannot silently turn the
 * "already current" step into a "newer" one.
 */
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";

import { pageScript } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { resolve } from "../../lib/repo.ts";
import { UPDATE_RELEASES_PREFIX } from "../../lib/update-endpoint.ts";
import { releaseResponse, startUpdateStub } from "../../lib/update-stub.ts";
import type { UpdateStub, UpdateStubRequest } from "../../lib/update-stub.ts";
import { MIN_TODAY_TEXT } from "./boot.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

/**
 * The smoke-only timeout of this scenario's launch
 * (PASSQUEST_DESKTOP_UPDATE_TIMEOUT_MS), in milliseconds: seconds long, unlike
 * the Go package test's few milliseconds, so a loaded CI runner cannot turn an ordinary loopback check
 * into the timeout state, and short enough that the one stalled step does not
 * dominate the scenario. The stall itself is held past it by the stub.
 */
const TIMEOUT_MS = 3000;
/** The publication date the stub serves and the `newer` view must show. */
const PUBLISHED_AT = "2026-11-18T00:00:00Z";
const PUBLISHED = "2026-11-18";
/** The two release links' labels, in the row's order (views/settings.ts updateView). */
const LINK_TEXT: ReadonlyArray<readonly [string, string]> = [
  ["发行说明", "Release notes"],
  ["下载页面", "Download page"],
];

/**
 * One scripted stub answer and what the row must show for it. The text and
 * reason fragments mirror the two languages of core/i18n.ts (updateView and
 * UPDATE_REASONS): the page runs in the window's language, so either form is
 * accepted, and a reason the row shows for the wrong failure is a failure.
 */
interface StepExpectation {
  state: string;
  text: readonly [string, string];
  /** Release links the state must show; the `newer` answer only. */
  links: number;
  /** Where each link must point (only when `links` > 0). */
  href?: string;
  /** The release tag and date the `newer` text must carry. */
  tag?: string;
  /** The publication date the `newer` text must carry, as formatted by the app. */
  published?: string;
  /** A tag a `current` answer must never surface: the entry was ignored. */
  hidden?: string;
  /** The exact reason line (`<b>`) an `unavailable` answer must show. */
  reason?: readonly [string, string];
}

/** The page script: one press per stub answer, reading each rendered state. */
const script = (presses: number) => `
location.hash = "#/settings";
const row = '[data-testid="update-row"]';
const state = () => q(row)?.dataset.updateState ?? null;
await until("the update row", () => q('[data-act="row"][data-v="updates:"]'));
await click('[data-act="row"][data-v="updates:"]');
await until("the expanded update row", () => q(row + ".open") && q('[data-act="update-check"]:not([disabled])'));
const steps = [];
for (let press = 1; press <= ${presses}; press++) {
  await click('[data-act="update-check"]');
  // The click handler renders the checking state synchronously, so a press that
  // reached a check always shows it; a same-state answer (current after current)
  // cannot otherwise be told from a stale row.
  const checking = state() === "checking";
  await until("check " + press + " to finish", () => state() !== "checking");
  const shown = q(row);
  steps.push({
    press,
    checking,
    state: shown.dataset.updateState,
    text: shown.innerText.replace(/\\s+/g, " ").trim(),
    reason: q(row + " .rr-more b")?.textContent?.trim() ?? null,
    links: all(row + " a").map((link) => ({
      href: link.getAttribute("href"),
      target: link.getAttribute("target"),
      rel: link.getAttribute("rel"),
      text: link.textContent.trim(),
    })),
  });
}
const version = q(row + " .model")?.textContent?.trim() ?? null;
await today();
return { steps, version, todayText: document.body.innerText.length };
`;

interface ListingEntry {
  path: string;
  size: number;
  mtimeMs: number;
}

/** Every file under `dir` (recursive), with the name, size and mtime a check must not change. */
function listingOf(dir: string): ListingEntry[] {
  const out: ListingEntry[] = [];
  const walk = (current: string) => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) walk(full);
      else {
        const stat = statSync(full);
        out.push({ path: path.relative(dir, full).split(path.sep).join("/"), size: stat.size, mtimeMs: stat.mtimeMs });
      }
    }
  };
  if (existsSync(dir)) walk(dir);
  return out.sort((a, b) => a.path.localeCompare(b.path));
}

/** What changed between two listings, one phrase per file; empty when nothing did. */
function listingChanges(before: ListingEntry[], after: ListingEntry[]): string[] {
  const old = new Map(before.map((entry) => [entry.path, entry]));
  const now = new Map(after.map((entry) => [entry.path, entry]));
  const parts: string[] = [];
  for (const [name, entry] of old) {
    const next = now.get(name);
    if (!next) parts.push(`removed ${name}`);
    else if (next.size !== entry.size || next.mtimeMs !== entry.mtimeMs) {
      parts.push(`changed ${name} (${entry.size} -> ${next.size} bytes, mtime ${entry.mtimeMs} -> ${next.mtimeMs})`);
    }
  }
  for (const name of now.keys()) {
    if (!old.has(name)) parts.push(`added ${name}`);
  }
  return parts;
}

export const update: Scenario = {
  id: "update",
  async run(s) {
    let running = "";
    try {
      running = stringField(JSON.parse(readFileSync(resolve(s.shared.ctx.root, "mygo.json"), "utf8")), "version") ?? "";
    } catch {
      running = "";
    }
    if (!s.check(/^\d+\.\d+\.\d+$/.test(running), `update: mygo.json carries no plain version to compare release tags against (${JSON.stringify(running)})`)) return;
    const [major = 0, minor = 0, patch = 0] = running.split(".").map(Number);
    const equalTag = `v${running}`;
    const newerTag = `v${major}.${minor}.${patch + 1}`;
    // A tag strictly below the running version, so the step really exercises
    // "an older release reads as current": the running version back through its
    // own patch, its own minor and its own major. v0.0.0 has nothing below it,
    // and the scenario stops with a named failure rather than serving a tag
    // that is not older.
    const below = patch > 0 ? [major, minor, patch - 1] : minor > 0 ? [major, minor - 1, 0] : major > 0 ? [major - 1, 0, 0] : null;
    if (below === null) {
      s.check(false, `update: the running version ${running} has no older semver tag; the step for an older release cannot be built`);
      return;
    }
    const olderTag = `v${below.join(".")}`;
    const prereleaseTag = `v${major}.${minor}.${patch + 1}-rc.1`;
    const releasePage = `${UPDATE_RELEASES_PREFIX}tag/${newerTag}`;
    const current: readonly [string, string] = ["当前版本就是已发布的最新版本", "The running version is the latest published release"];
    const didNotFinish: readonly [string, string] = ["检查没有完成", "The check did not finish"];

    // The stub binds to the constant endpoint in internal/update's source; a
    // tree without it (a self-test fixture that never gets this far) is a named
    // failure, not a throw.
    let stub: UpdateStub;
    try {
      stub = startUpdateStub(s.shared.ctx.root);
    } catch (error) {
      s.check(false, `update: the update stub did not start: ${error instanceof Error ? error.message : String(error)}`);
      return;
    }
    let stopped = false;
    const stop = () => {
      if (stopped) return;
      stopped = true;
      stub.stop();
    };
    try {
      // The stub's answers and the row's expectations, one row per check:
      // newer, current (equal), an older tag and a pre-release (both current),
      // 404, a malformed body, a rate limit, a stall past the timeout, and the
      // stub closing while it answers the last request.
      const scripted: Array<{ answer: (request: UpdateStubRequest) => Response | Promise<Response>; expect: StepExpectation }> = [
        {
          answer: () => releaseResponse({ tag: newerTag, published: PUBLISHED_AT }),
          expect: { state: "newer", text: ["新版本", "A newer version exists"], links: 2, href: releasePage, tag: newerTag, published: PUBLISHED },
        },
        { answer: () => releaseResponse({ tag: equalTag }), expect: { state: "current", text: current, links: 0 } },
        { answer: () => releaseResponse({ tag: olderTag }), expect: { state: "current", text: current, links: 0, hidden: olderTag } },
        { answer: () => releaseResponse({ tag: prereleaseTag, prerelease: true }), expect: { state: "current", text: current, links: 0, hidden: prereleaseTag } },
        { answer: () => new Response(null, { status: 404 }), expect: { state: "no_release", text: ["还没有发布任何版本", "No release has been published"], links: 0 } },
        {
          answer: () => new Response("not a release document", { status: 200 }),
          expect: { state: "unavailable", text: didNotFinish, links: 0, reason: ["回应不是发布信息", "the response was not a release document"] },
        },
        {
          answer: () => new Response("rate limited", { status: 429 }),
          expect: { state: "unavailable", text: didNotFinish, links: 0, reason: ["请求次数被限制", "the request was rate-limited by GitHub"] },
        },
        {
          // Held past the client's timeout by the stub, which releases it when
          // the stub stops: the check must time out on its own, and no handler
          // promise may outlive the launch.
          answer: () => stub.hold(),
          expect: { state: "unavailable", text: didNotFinish, links: 0, reason: ["请求超时", "the request timed out"] },
        },
        {
          // The offline step: stopping the stub *is* the answer, so this request
          // never gets one - stop(true) closes this very connection, and the
          // promise that never settles is never sent on the wire.
          answer: () => {
            stop();
            return new Promise<Response>(() => {});
          },
          expect: { state: "unavailable", text: didNotFinish, links: 0, reason: ["无法连接到 GitHub", "the request could not reach GitHub"] },
        },
      ];
      stub.queue = scripted.map((step) => step.answer);

      // The data directory before the window starts and after it exits: the
      // app's own boot creates its database there, and nothing beyond that may
      // appear, change or vanish. The launch's content size is part of this
      // check: it clears MyGo's window state key (internal/desktop/smoke.go
      // sizeWindow), the way desktop-ux does, so the window framework saves no
      // window-state.json either.
      const bootFile = path.relative(s.dataDir, s.database).split(path.sep).join("/");
      const before = listingOf(s.dataDir);
      const launched = await launch(s, {
        label: "update",
        probeListeners: true,
        env: {
          PASSQUEST_DESKTOP_UPDATE_URL: stub.endpoint,
          PASSQUEST_DESKTOP_UPDATE_TIMEOUT_MS: String(TIMEOUT_MS),
          PASSQUEST_DESKTOP_CONTENT_SIZE: "1280x820",
        },
        script: pageScript(script(scripted.length)),
      });
      if (!launched) return;

      // Every request the window made, against the one allowed shape: method,
      // path, header set and values, no body. Accept-Encoding is Go's own, so a
      // Go change that adds a header fails here loudly (P1's Go test pins the
      // same set).
      const allowed = ["accept", "accept-encoding", "host", "user-agent"];
      s.check(stub.requests.length === scripted.length, `update: the stub served ${stub.requests.length} request(s), want one per check (${scripted.length})`);
      for (const [index, request] of stub.requests.entries()) {
        const n = index + 1;
        s.check(request.method === "GET", `update: request ${n} used ${request.method}, want GET`);
        s.check(request.path === stub.path, `update: request ${n} reached ${request.path}, want ${stub.path}`);
        s.check(request.body === "", `update: request ${n} carried a body: ${JSON.stringify(request.body.slice(0, 80))}`);
        const extra = Object.keys(request.headers).filter((name) => !allowed.includes(name));
        s.check(extra.length === 0, `update: request ${n} carried header(s) ${extra.join(", ")}, outside the allowed set (${allowed.join(", ")})`);
        s.check(request.headers.accept === "application/vnd.github+json", `update: request ${n} Accept = ${JSON.stringify(request.headers.accept)}, want "application/vnd.github+json"`);
        s.check(request.headers["user-agent"] === "PassQuest", `update: request ${n} User-Agent = ${JSON.stringify(request.headers["user-agent"])}, want "PassQuest"`);
        s.check(request.headers["accept-encoding"] === "gzip", `update: request ${n} Accept-Encoding = ${JSON.stringify(request.headers["accept-encoding"])}, want Go's own "gzip"`);
        const authority = new URL(stub.endpoint).host;
        s.check(request.headers.host === authority, `update: request ${n} Host = ${JSON.stringify(request.headers.host)}, want ${authority}`);
      }
      // The offline step's own answer closed the stub mid-request; nothing may
      // answer here anymore, or the state was not the closed-stub one.
      let closed = false;
      try {
        await fetch(stub.endpoint, { signal: AbortSignal.timeout(2000) });
      } catch {
        closed = true;
      }
      s.check(closed, "update: the stub still answers after the run; the last check was served, not refused");

      // Everything but the app's own boot held still: the before listing was
      // taken before the launch, so the one permitted change is the database
      // the window's start-up creates, named from this run's own path - a
      // check that writes anything (a stamp, a cache, a window state), touches
      // an existing file or removes one fails with the file named.
      const after = listingOf(s.dataDir);
      const changes = listingChanges(before, after);
      s.check(
        changes.length === 1 && changes[0] === `added ${bootFile}`,
        `update: the data directory changed across the checks, want only the app's own boot adding ${bootFile}: ${changes.join("; ") || "nothing changed"}`,
      );
      s.check(after.some((entry) => entry.path === bootFile), `update: the data directory holds no ${bootFile} after the launch; the before/after listing proves nothing`);

      // What the window showed, one check at a time.
      const steps = arrayField(launched.value, "steps");
      s.check(steps.length === scripted.length, `update: the page reported ${steps.length} check(s), want ${scripted.length}`);
      scripted.forEach(({ expect }, index) => {
        const step = steps[index];
        if (!step) return;
        const n = index + 1;
        s.check(field(step, "checking") === true, `update: step ${n} never showed the checking state; the press did not run a check`);
        const state = stringField(step, "state") ?? "";
        s.check(state === expect.state, `update: step ${n} shows state ${state}, want ${expect.state}`);
        const text = stringField(step, "text") ?? "";
        s.check(text.includes(expect.text[0]) || text.includes(expect.text[1]), `update: step ${n} (${state}) text does not name its state (${expect.text.join(" / ")}): ${text}`);
        const links = arrayField(step, "links");
        s.check(links.length === expect.links, `update: step ${n} (${state}) shows ${links.length} link(s), want ${expect.links}`);
        for (const [linkIndex, link] of links.entries()) {
          const m = linkIndex + 1;
          const href = stringField(link, "href");
          s.check(href === expect.href, `update: step ${n} link ${m} href = ${JSON.stringify(href)}, want ${JSON.stringify(expect.href)}`);
          s.check(stringField(link, "target") === "_blank", `update: step ${n} link ${m} target = ${JSON.stringify(stringField(link, "target"))}, want "_blank"`);
          s.check(stringField(link, "rel") === "noopener", `update: step ${n} link ${m} rel = ${JSON.stringify(stringField(link, "rel"))}, want "noopener"`);
          const label = LINK_TEXT[linkIndex] ?? LINK_TEXT[LINK_TEXT.length - 1]!;
          const shown = stringField(link, "text") ?? "";
          s.check(shown === label[0] || shown === label[1], `update: step ${n} link ${m} text = ${JSON.stringify(shown)}, want ${label.join(" / ")}`);
        }
        if (expect.hidden !== undefined) {
          s.check(!text.includes(expect.hidden), `update: step ${n} (${state}) surfaces the ignored tag ${expect.hidden}`);
        }
        if (expect.tag !== undefined) s.check(text.includes(expect.tag), `update: step ${n} text does not carry the release tag ${expect.tag}: ${text}`);
        if (expect.published !== undefined) s.check(text.includes(expect.published), `update: step ${n} text does not carry the publication date ${expect.published}: ${text}`);
        if (expect.reason !== undefined) {
          const reason = field(step, "reason");
          const shown = typeof reason === "string" ? reason : "";
          s.check(shown === expect.reason[0] || shown === expect.reason[1], `update: step ${n} reason = ${JSON.stringify(shown)}, want ${expect.reason.join(" / ")}`);
        }
      });
      s.check(stringField(launched.value, "version") === running, `update: the Updates row shows version ${JSON.stringify(stringField(launched.value, "version"))}, want ${running}`);
      const todayText = numberField(launched.value, "todayText") ?? 0;
      s.check(todayText > MIN_TODAY_TEXT, `update: Today does not render after the checks: body text length ${todayText} <= ${MIN_TODAY_TEXT}`);
    } finally {
      stop();
    }
  },
};
