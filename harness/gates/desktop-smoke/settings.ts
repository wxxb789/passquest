import { randomBytes } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

import { pageScript, STUB_MODEL, STUB_PROVIDER } from "../../lib/desktop-page.ts";
import { arrayField } from "../../lib/json.ts";
import { executable, run } from "../../lib/process.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

/** Every regular file under `dir`. */
function filesUnder(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).flatMap((name) => {
    const full = path.join(dir, name);
    return statSync(full).isDirectory() ? filesUnder(full) : [full];
  });
}

function containsSecret(bytes: Buffer, secret: string): boolean {
  return bytes.includes(Buffer.from(secret, "utf8")) || bytes.includes(Buffer.from(secret, "utf16le"));
}

/** Removes a credential the window stored, through the OS tool; best effort, never throws. */
async function forgetCredential(service: string, account: string): Promise<void> {
  if (process.platform === "win32") {
    await run(executable("cmdkey"), [`/delete:${service}/${account}`]);
  } else if (process.platform === "darwin") {
    await run("security", ["delete-generic-password", "-s", service, "-a", account]);
  } else {
    await run("secret-tool", ["clear", "service", service, "account", account]);
  }
}

export const settings: Scenario = {
  id: "settings",
  async run(s) {
    const { provider } = s.shared;
    const key = `pq-smoke-${randomBytes(16).toString("hex")}`;
    const account = `api-key:${STUB_PROVIDER}`;
    provider.received.length = 0;
    try {
      const launched = await launch(s, {
        label: "settings",
        script: pageScript(`
const KEY = ${JSON.stringify(key)};
const row = '.rrow[data-provider="${STUB_PROVIDER}"]';
const leaks = [];
const look = (when) => {
  if (document.documentElement.outerHTML.includes(KEY) || document.body.innerText.includes(KEY)) leaks.push(when);
};
location.hash = "#/settings";
await until("the stub provider row", () => q(row + ' [data-credential="missing"]'));
await click('[data-act="cred"][data-v="${STUB_PROVIDER}"]');
const input = await until("the key field", () => q('#credform input[name="key"]'));
input.value = KEY;
await click('[data-act="credential-save"]');
await until("the stored key", () => q(row + ' [data-credential="present"]'), 20000);
look("after saving");
await click('[data-act="model-test"][data-v="${STUB_MODEL}"]');
await until("the verified model", () => q(row + ' [data-capability="verified"]'), 30000);
look("after the model test");
if (!q('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]')) await click('[data-act="row"][data-v="${STUB_PROVIDER}"]');
await until("the remove-key button", () => q('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]'));
await click('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]');
await until("the cleared key", () => q(row + ' [data-credential="missing"]'), 20000);
look("after clearing");
return { leaks };
`),
      });
      if (!launched) return;
      const leaks = arrayField(launched.value, "leaks");
      s.check(leaks.length === 0, `settings: the key is in the DOM ${leaks.join(", ")}`);
      // The launch environment carries no key, so the provider can only have
      // received it from the OS credential store.
      s.check(
        provider.received.some((request) => request.authorization === `Bearer ${key}`),
        "settings: the provider call after entering the key did not carry it (OS store path)",
      );
      // The data directory, the launch's result and, for the package smoke,
      // the installed tree: no credential may reach any of them.
      const scanned = [
        ...filesUnder(s.dataDir),
        launched.resultPath,
        ...(s.shared.secretScanRoots ?? []).flatMap((root) => filesUnder(root)),
      ];
      const holders = scanned.filter((file) => containsSecret(readFileSync(file), key)).map((file) => path.relative(s.dir, file));
      if (containsSecret(Buffer.from(launched.output), key)) holders.push("app output");
      s.check(holders.length === 0, `settings: the key bytes are in ${holders.join(", ")}`);
      s.check(scanned.some((file) => file.endsWith(".sqlite")), "settings: no SQLite file was scanned for the key");
      if (process.platform === "win32") {
        // The full list names stored targets only (`/list:<target>` echoes the target it looked for).
        const listed = await run(executable("cmdkey"), ["/list"]);
        s.check(
          listed.code === 0 && !listed.stdout.includes(s.service),
          "settings: the cleared key is still in the Windows credential store",
        );
      }
    } finally {
      await forgetCredential(s.service, account);
    }
  },
};
