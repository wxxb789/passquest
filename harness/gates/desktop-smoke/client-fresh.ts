import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

import { launchDesktop } from "../../lib/desktop.ts";
import { cleanEnv } from "../../lib/desktop-page.ts";
import { resolve } from "../../lib/repo.ts";
import type { Scenario } from "./scenario.ts";

const CLIENT_PATH = "desktop/web/src/api/mygo.ts";
const GENERATE_TIMEOUT_MS = 30_000;

export const clientFresh: Scenario = {
  id: "client-fresh",
  async run(s) {
    const generated = path.join(s.dir, "mygo.generated.ts");
    // From the repository root the generator finds the Go sources and keeps their doc comments.
    const result = await launchDesktop({
      binary: s.shared.binary,
      env: { ...cleanEnv(), MYGO_GENERATE: generated },
      timeoutMs: GENERATE_TIMEOUT_MS,
      cwd: s.shared.ctx.root,
    });
    const how = result.error ?? `exit ${result.code}${result.timedOut ? ", timed out" : ""}`;
    if (!s.check(existsSync(generated), `client-fresh: the binary wrote no client with MYGO_GENERATE (${how})`)) return;
    const committedPath = resolve(s.shared.ctx.root, CLIENT_PATH);
    if (!s.check(existsSync(committedPath), `client-fresh: ${CLIENT_PATH} is missing`)) return;
    const fresh = readFileSync(generated, "utf8").replace(/\r\n/g, "\n").split("\n");
    const committed = readFileSync(committedPath, "utf8").replace(/\r\n/g, "\n").split("\n");
    let line = 0;
    while (line < Math.max(fresh.length, committed.length) && fresh[line] === committed[line]) line++;
    s.check(
      line === fresh.length && line === committed.length,
      `client-fresh: ${CLIENT_PATH} is stale: the binary generates a different client from line ${line + 1}; regenerate it with MYGO_GENERATE`,
    );
  },
};
