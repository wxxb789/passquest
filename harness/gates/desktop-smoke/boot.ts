import { field, numberField } from "../../lib/json.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

const MIN_TODAY_TEXT = 200;

const BOOT_SCRIPT = `
const stage = () => document.querySelector("#stage");
const ready = () => window.__pq === undefined || window.__pq.ready === true;
const content = () => (stage()?.children.length ?? 0) > 0 && document.body.innerText.length > ${MIN_TODAY_TEXT};
const deadline = Date.now() + 15000;
while (!(ready() && content()) && Date.now() < deadline) await new Promise((done) => setTimeout(done, 100));
const diag = window.__pqErrors;
if (diag) {
  diag.startProbe();
  const probeDeadline = Date.now() + 3000;
  while (diag.probe !== "clear" && Date.now() < probeDeadline) await new Promise((done) => setTimeout(done, 50));
}
const node = stage();
const errors = document.querySelectorAll("#errbar, .error-bar");
return {
  textLength: document.body.innerText.length,
  stageChildren: node ? node.children.length : -1,
  stageTextLength: node ? node.innerText.trim().length : 0,
  stageText: node ? node.innerText.replace(/\\s+/g, " ").trim().slice(0, 240) : "",
  errorBars: errors.length,
  errorText: Array.from(errors, (element) => element.textContent ?? "").join(" | ").slice(0, 300),
  ready: window.__pq === undefined ? null : window.__pq.ready === true,
  probe: diag ? diag.probe : null,
  opaqueErrors: diag ? diag.opaque : null,
};
`;

export const boot: Scenario = {
  id: "boot",
  async run(s) {
    const launched = await launch(s, { label: "boot", script: BOOT_SCRIPT });
    if (!launched) return;
    const value = launched.value;
    const textLength = numberField(value, "textLength") ?? 0;
    const stageChildren = numberField(value, "stageChildren") ?? -1;
    s.check(textLength > MIN_TODAY_TEXT, `boot: blank Today, body text length ${textLength} <= ${MIN_TODAY_TEXT}`);
    s.check(stageChildren >= 0, "boot: #stage is missing");
    s.check(
      stageChildren > 0 && (numberField(value, "stageTextLength") ?? 0) > 0,
      `boot: #stage is empty (${stageChildren} children)`,
    );
    const errorBars = numberField(value, "errorBars") ?? -1;
    s.check(errorBars === 0, `boot: ${errorBars} error bar(s): ${String(field(value, "errorText") ?? "")}`);
    const probe = field(value, "probe");
    s.check(
      probe === "clear",
      `boot: the bundle's own error did not reach window.onerror in full (probe=${String(probe)}, opaque=${String(field(value, "opaqueErrors"))}); classifying opaque errors would hide real ones`,
    );
    const ready = field(value, "ready");
    s.check(
      ready === null || ready === true,
      `boot: window.__pq.ready = ${JSON.stringify(ready)}; the stage shows: ${String(field(value, "stageText") ?? "")}`,
    );
  },
};
