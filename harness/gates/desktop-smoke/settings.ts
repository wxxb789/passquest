import { randomBytes } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

import {
  pageScript,
  STUB_MODEL,
  STUB_PROVIDER,
  STUB_RESPONSES_MODEL,
  STUB_RESPONSES_PROVIDER,
  stubResponsesProvider,
  writeStubRegistry,
} from "../../lib/desktop-page.ts";
import { arrayField, stringField } from "../../lib/json.ts";
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
    const responsesAccount = `api-key:${STUB_RESPONSES_PROVIDER}`;
    const registry = path.join(s.dir, "registry.json");
    writeStubRegistry(registry, provider.endpoint, [stubResponsesProvider(provider)]);
    provider.received.length = 0;
    try {
      const launched = await launch(s, {
        label: "settings",
        env: { PASSQUEST_PROVIDER_REGISTRY: registry },
        script: pageScript(`
const KEY = ${JSON.stringify(key)};
const row = '.rrow[data-provider="${STUB_PROVIDER}"]';
const responsesRow = '.rrow[data-provider="${STUB_RESPONSES_PROVIDER}"]';
const leaks = [];
const look = (when) => {
  if (document.documentElement.outerHTML.includes(KEY) || document.body.innerText.includes(KEY)) leaks.push(when);
};
location.hash = "#/settings";
await until("the stub provider row", () => q(row + ' [data-credential="missing"]'));
await until("the responses provider row", () => q(responsesRow + ' [data-credential="missing"]'));
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
await click('[data-act="cred"][data-v="${STUB_RESPONSES_PROVIDER}"]');
const responsesInput = await until("the responses key field", () => q('#credform input[name="key"]'));
responsesInput.value = KEY;
await click('[data-act="credential-save"]');
await until("the stored responses key", () => q(responsesRow + ' [data-credential="present"]'), 20000);
look("after saving the responses key");
await click('[data-act="model-test"][data-v="${STUB_RESPONSES_MODEL}"]');
await until("the verified responses model", () => q(responsesRow + ' [data-capability="verified"]'), 30000);
look("after the responses model test");
if (!q('[data-act="cred-clear"][data-v="${STUB_RESPONSES_PROVIDER}"]')) await click('[data-act="row"][data-v="${STUB_RESPONSES_PROVIDER}"]');
await until("the responses remove-key button", () => q('[data-act="cred-clear"][data-v="${STUB_RESPONSES_PROVIDER}"]'));
await click('[data-act="cred-clear"][data-v="${STUB_RESPONSES_PROVIDER}"]');
await until("the cleared responses key", () => q(responsesRow + ' [data-credential="missing"]'), 20000);
look("after clearing the responses key");
const endpointFormats = await until("the endpoint formats line", () => q('[data-testid="endpoint-formats"]'));
return { leaks, endpointFormats: (endpointFormats && endpointFormats.textContent) || "" };
`),
      });
      if (!launched) return;
      const leaks = arrayField(launched.value, "leaks");
      s.check(leaks.length === 0, `settings: the key is in the DOM ${leaks.join(", ")}`);
      const formats = stringField(launched.value, "endpointFormats") ?? "";
      s.check(
        formats.includes("/chat/completions") && formats.includes("/responses"),
        `settings: the endpoint formats line does not name both suffixes: ${JSON.stringify(formats)}`,
      );
      // The launch environment carries no key, so the provider can only have
      // received it from the OS credential store.
      s.check(
        provider.received.some((request) => request.authorization === `Bearer ${key}`),
        "settings: the provider call after entering the key did not carry it (OS store path)",
      );
      s.check(
        provider.received.some((request) => request.path === "/v1/responses" && request.authorization === `Bearer ${key}`),
        "settings: the responses endpoint was not verified through the OS store path at /v1/responses",
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
      await forgetCredential(s.service, responsesAccount);
    }
  },
};
