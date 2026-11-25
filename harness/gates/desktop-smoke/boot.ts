import { field, numberField } from "../../lib/json.ts";
import { TITLEBAR_LIBRARY } from "../../lib/ux-page.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

export const MIN_TODAY_TEXT = 200;

const BOOT_SCRIPT = `
${TITLEBAR_LIBRARY}
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
let shotBound = null;
let shotError = "";
if (window.mygo && typeof window.mygo.call === "function") {
  try {
    await window.mygo.call("SmokeShot.Capture", "boot");
    shotBound = true;
  } catch (error) {
    shotBound = false;
    shotError = String((error && error.message) || error);
  }
}
// Both caption-button sheets the app ships must be accepted where GTK parses them (Linux); the
// call answers the parse error, and the page's own appearance is left in place.
let buttonsThemeError = "";
if (window.mygo && typeof window.mygo.call === "function") {
  const current = document.documentElement.dataset.theme || "light";
  for (const theme of ["light", "dark", current]) {
    try {
      await window.mygo.call("Study.SetWindowButtonsTheme", theme);
    } catch (error) {
      buttonsThemeError = String((error && error.message) || error);
    }
  }
}
// The boot state is read here, before the title walk navigates: the checks it feeds
// describe the first Today, and a re-render after the return must not decide them.
const bootState = {
  textLength: document.body.innerText.length,
  stageChildren: node ? node.children.length : -1,
  stageTextLength: node ? node.innerText.trim().length : 0,
  stageText: node ? node.innerText.replace(/\\s+/g, " ").trim().slice(0, 240) : "",
  errorBars: errors.length,
  errorText: Array.from(errors, (element) => element.textContent ?? "").join(" | ").slice(0, 300),
  ready: window.__pq === undefined ? null : window.__pq.ready === true,
  probe: diag ? diag.probe : null,
  opaqueErrors: diag ? diag.opaque : null,
  shotBound,
  shotError,
  buttonsThemeError,
};
const titleSnap = async () => {
  let title = null;
  let titleError = "";
  if (window.mygo && typeof window.mygo.call === "function") {
    try {
      title = String(await window.mygo.call("SmokeWindow.Title"));
    } catch (error) {
      titleError = String((error && error.message) || error);
    }
  }
  return { title, titleError, documentTitle: document.title };
};
// The window's title follows the page's asynchronously, so each stop waits for the two to
// agree (a five-second cap: a value that never agrees is what the boot checks then report).
const titleInSync = async () => {
  const syncDeadline = Date.now() + 5000;
  for (;;) {
    const snap = await titleSnap();
    if (snap.title === null || snap.title === snap.documentTitle || Date.now() > syncDeadline) return snap;
    await new Promise((done) => setTimeout(done, 50));
  }
};
const titleAtToday = await titleInSync();
location.hash = "#/settings";
{
  const routeDeadline = Date.now() + 5000;
  while (document.title === titleAtToday.documentTitle && Date.now() < routeDeadline) await new Promise((done) => setTimeout(done, 50));
}
const titleAtSettings = await titleInSync();
location.hash = "#/today";
{
  const routeDeadline = Date.now() + 5000;
  while (document.title !== titleAtToday.documentTitle && Date.now() < routeDeadline) await new Promise((done) => setTimeout(done, 50));
}
const titleBackAtToday = await titleInSync();
return {
  ...bootState,
  titlebar: TITLEBAR.read(),
  titles: { today: titleAtToday, settings: titleAtSettings, back: titleBackAtToday },
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
    const shotBound = field(value, "shotBound");
    if (shotBound !== null && shotBound !== undefined) {
      const shotError = String(field(value, "shotError") ?? "");
      s.check(
        shotBound === false && shotError.includes("SmokeShot.Capture"),
        `boot: a call to SmokeShot.Capture without PASSQUEST_DESKTOP_SHOT_DIR answered ${JSON.stringify(shotError)} (bound ${JSON.stringify(shotBound)})`,
      );
    }
    const buttonsThemeError = String(field(value, "buttonsThemeError") ?? "");
    s.check(buttonsThemeError === "", `boot: the title buttons' theme call failed: ${buttonsThemeError}`);
    const titles = field(value, "titles");
    if (titles !== undefined) {
      // Three stops, each read back from the platform and compared with the page's
      // own document.title: Today, Settings, and Today again.
      for (const where of ["today", "settings", "back"]) {
        const stop = field(titles, where);
        const title = field(stop, "title");
        const titleError = String(field(stop, "titleError") ?? "");
        const documentTitle = String(field(stop, "documentTitle") ?? "");
        // A fixture app without a mygo runtime cannot answer the call; the real
        // app always has one, and its failure is an assertion failure.
        if (title === null && titleError === "") continue;
        if (typeof title !== "string") {
          s.check(false, `boot: the window title at ${where} reads ${JSON.stringify(title)} (${titleError || "no error"})`);
          continue;
        }
        s.check(
          title === documentTitle,
          `boot: the window title at ${where} is ${JSON.stringify(title)}, the page's document.title is ${JSON.stringify(documentTitle)}`,
        );
        s.check(title.startsWith("Pass Quest — "), `boot: the window title at ${where} is ${JSON.stringify(title)}, want it to start with "Pass Quest — "`);
      }
      const atToday = String(field(field(titles, "today"), "documentTitle") ?? "");
      const atSettings = String(field(field(titles, "settings"), "documentTitle") ?? "");
      const backAtToday = String(field(field(titles, "back"), "documentTitle") ?? "");
      s.check(
        atToday !== "" && atSettings !== atToday,
        `boot: document.title stayed ${JSON.stringify(atToday)} from Today to Settings`,
      );
      s.check(
        backAtToday === atToday,
        `boot: document.title after returning to Today is ${JSON.stringify(backAtToday)}, want ${JSON.stringify(atToday)}`,
      );
    }
    const titlebar = field(value, "titlebar");
    if (titlebar !== undefined) {
      const room = field(titlebar, "room");
      const key = ["height", "left", "right", "bottom"].map((name) => numberField(room, name) ?? "?").join("/");
      s.detail?.(`boot: room ${key} px, marker ${String(field(titlebar, "marker"))}`);
    }
  },
};
