import { pageScript } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { TITLEBAR_LIBRARY } from "../../lib/ux-page.ts";
import { launch } from "./scenario.ts";
import type { Scenario, ScenarioRun } from "./scenario.ts";

/** The seven routes fact 2 names, walked at the minimum window no other gate drives. */
const ROUTES = ["today", "practice", "feedback", "assess", "report", "anki", "settings"];

/**
 * One launch at 880x600 walks every route in both appearances — the hash for
 * the route, the page's own theme control for the appearance — and at each
 * stop records the room the window reports, the page's `:root[data-titlebar]`
 * marker and the title-area checks (harness/lib/ux-page.ts).
 */
const WALK_SCRIPT = `
${TITLEBAR_LIBRARY}
const routes = ${JSON.stringify(ROUTES)};
const sizeOf = () => ({ width: document.documentElement.clientWidth, height: document.documentElement.clientHeight });
// The walk's own precondition (Q-0012-05's rule): every stop below is read at
// the 880x600 page area the launch asked for, and nothing else may stand in.
{
  const deadline = Date.now() + 3000;
  while ((sizeOf().width !== 880 || sizeOf().height !== 600) && Date.now() < deadline) await sleep(100);
}
const size = sizeOf();
const stops = [];
const theme = () => document.documentElement.dataset.theme;
const applyTheme = async (want) => {
  if (theme() !== want) q('[data-act="theme"]').click();
  await until("the " + want + " theme", () => theme() === want);
};
for (const route of routes) {
  location.hash = "#/" + route;
  // The dock marks the route the app rendered; Feedback keeps the practice entry.
  const marked = route === "feedback" ? "practice" : route;
  await until("the " + route + " route", () => q("#dock-" + marked + '[aria-current="page"]'));
  await until("a settled stage", () => (q("#stage")?.children.length ?? 0) > 0 && !q('#stage [aria-busy="true"]'));
  await sleep(400);
  await until("no loading card", () => !q('#stage [aria-busy="true"]'));
  for (const want of ["light", "dark"]) {
    await applyTheme(want);
    await sleep(150);
    stops.push(Object.assign({ route: route, want: want, shown: theme() }, TITLEBAR.check()));
  }
}
// The synthetic-pointer assertions (goal 0022, Done-when 2): the bridge takes over a press at the
// strip's derived probe point (defaultPrevented) and the window state moves; a control inside the
// title area keeps its press and its click. Recorded, never asserted here: the gate decides per OS.
const pressAt = (x, y, detail) => {
  const hit = document.elementFromPoint(x, y);
  if (!(hit instanceof Element)) return null;
  const event = new MouseEvent("mousedown", { bubbles: true, cancelable: true, button: 0, detail: detail, clientX: x, clientY: y });
  hit.dispatchEvent(event);
  return { prevented: event.defaultPrevented, hit: hit.id || hit.tagName.toLowerCase() };
};
const pointer = { room: null, marker: null, why: "", issues: [], probe: null, takeover: null, maxBefore: null, maxAfter: null, maxBack: null, themePress: null, themeBefore: null, themeFirst: null, themeBack: null };
const final = TITLEBAR.check();
pointer.room = final.room;
pointer.marker = final.marker;
pointer.why = final.why;
pointer.issues = final.issues;
const controls = window.mygo && window.mygo.window ? window.mygo.window : null;
// Waits for a wanted window state, then records what was read either way: the gate judges the
// reading, not whether it flipped in time (a press that never arrives reads as the old state).
const settleMax = async (want) => {
  const deadline = Date.now() + 5000;
  for (;;) {
    const now = await controls.isMaximized();
    if (now === want || Date.now() > deadline) return now;
    await sleep(50);
  }
};
if (final.probe) {
  pointer.probe = final.probe;
  const x = final.probe[0];
  const y = final.probe[1];
  if (controls) {
    pointer.maxBefore = await controls.isMaximized();
    pointer.takeover = pressAt(x, y, 2);
    if (pointer.takeover && pointer.takeover.prevented) {
      pointer.maxAfter = await settleMax(true);
      const again = pressAt(x, y, 2);
      if (again && again.prevented) pointer.maxBack = await settleMax(false);
    }
  }
}
// The theme action re-renders the header, so the button is re-queried before every use.
const themeButton = () => q('[data-act="theme"]');
const settleTheme = async (want) => {
  const deadline = Date.now() + 3000;
  for (;;) {
    const now = theme();
    if (now === want || Date.now() > deadline) return now;
    await sleep(50);
  }
};
if (themeButton()) {
  const box = themeButton().getBoundingClientRect();
  pointer.themePress = pressAt(box.left + box.width / 2, box.top + box.height / 2, 1);
  pointer.themeBefore = theme();
  themeButton().click();
  pointer.themeFirst = await settleTheme(pointer.themeBefore === "light" ? "dark" : "light");
  themeButton().click();
  pointer.themeBack = await settleTheme(pointer.themeBefore);
}
return { size: size, stops: stops, pointer: pointer };
`;

export const titlebar: Scenario = {
  id: "titlebar",
  async run(s) {
    const launched = await launch(s, {
      label: "titlebar",
      env: { PASSQUEST_DESKTOP_CONTENT_SIZE: "880x600" },
      script: pageScript(WALK_SCRIPT),
    });
    if (!launched) return;
    const size = field(launched.value, "size");
    const width = numberField(size, "width");
    const height = numberField(size, "height");
    s.check(
      width === 880 && height === 600,
      `titlebar: viewport precondition: the 880x600 launch reads a ${String(width)}x${String(height)} page area`,
    );
    if (width !== 880 || height !== 600) return;
    const stops = arrayField(launched.value, "stops");
    s.check(
      stops.length === ROUTES.length * 2,
      `titlebar: the 880x600 walk recorded ${stops.length} stop(s), want ${ROUTES.length} routes x 2 appearances`,
    );
    const seen = new Map<string, Set<string>>();
    for (const stop of stops) {
      const route = stringField(stop, "route") ?? "?";
      const want = stringField(stop, "want") ?? "?";
      const shown = stringField(stop, "shown") ?? "?";
      const room = field(stop, "room");
      const sizes = ["height", "left", "right", "bottom"].map((key) => numberField(room, key));
      const [height = 0, left = 0, right = 0, bottom = 0] = sizes;
      const marker = field(stop, "marker");
      const why = stringField(stop, "why") ?? "";
      s.check(shown === want, `titlebar: ${route} was recorded as ${shown}, not ${want}: the walk must really see the appearance it asked for`);
      const atRoute = seen.get(route) ?? new Set<string>();
      atRoute.add(shown);
      seen.set(route, atRoute);
      const complete = sizes.every((value) => typeof value === "number" && Number.isFinite(value));
      s.check(complete, `titlebar: ${route} ${shown} carries no complete room reading: ${JSON.stringify(room)}`);
      const hasRoom = complete && (height > 0 || left > 0 || right > 0 || bottom > 0);
      s.check(
        marker === hasRoom,
        `titlebar: ${route} ${shown} reads the room ${JSON.stringify(room)} and marker ${JSON.stringify(marker)}: the marker must be set exactly when a room value is above zero`,
      );
      if (!hasRoom) {
        s.check(why !== "", `titlebar: ${route} ${shown} reports no title bar and no reason; the record must say what was skipped and why`);
      }
      for (const issue of arrayField(stop, "issues")) {
        s.check(false, `titlebar: ${route} ${shown} reports ${JSON.stringify(issue)}`);
      }
    }
    s.check(
      seen.size === ROUTES.length && ROUTES.every((route) => seen.has(route)),
      `titlebar: the walk saw ${[...seen.keys()].join(", ") || "no route"}, want ${ROUTES.join(", ")}`,
    );
    for (const [route, themes] of seen) {
      s.check(themes.size === 2, `titlebar: ${route} was walked in ${[...themes].join(", ") || "no appearance"}, want both`);
    }
    if (stops.length > 0) {
      const rooms = new Map<string, number>();
      let markers = 0;
      for (const stop of stops) {
        const room = field(stop, "room");
        const key = ["height", "left", "right", "bottom"].map((name) => numberField(room, name) ?? "?").join("/");
        rooms.set(key, (rooms.get(key) ?? 0) + 1);
        if (field(stop, "marker") === true) markers++;
      }
      s.detail?.(`titlebar: 880x600 room ${[...rooms].map(([key, count]) => `${key} px x${count}`).join("; ")} at ${stops.length} stops, marker ${markers}/${stops.length}`);
    }
    assertPointer(s, field(launched.value, "pointer"));
  },
};

/**
 * The synthetic-pointer assertions (goal 0022). A probe point only exists where the window reports
 * a room: Windows (P2) asserts the takeover and the maximize/restore pair, macOS (P3) the same with
 * the AppleActionOnDoubleClick caveat in its message, and Linux (P4) records the maximize reading
 * without asserting it, because CI's Xvfb has no window manager to act on it (Q-0022-04).
 */
function assertPointer(s: ScenarioRun, pointer: unknown): void {
  const room = field(pointer, "room");
  const probe = field(pointer, "probe");
  const why = stringField(pointer, "why") ?? "";
  for (const issue of arrayField(pointer, "issues")) {
    s.check(false, `titlebar: the pointer stage's title-area check reports ${JSON.stringify(issue)}`);
  }
  if (probe === null || probe === undefined) {
    if (process.platform === "win32") {
      s.check(false, `titlebar: the Windows window reported no drag probe point (${why || "no reason"}); the room was ${JSON.stringify(room)}`);
    }
    s.detail?.(`titlebar pointer: no probe point, the takeover not observable (${why || "no reason recorded"})`);
    return;
  }
  const takeover = field(pointer, "takeover");
  const prevented = field(takeover, "prevented");
  s.check(prevented === true, `titlebar: a double-click press at the strip's probe ${JSON.stringify(probe)} was not taken over as a window drag (hit ${JSON.stringify(field(takeover, "hit"))})`);
  const before = field(pointer, "maxBefore");
  const after = field(pointer, "maxAfter");
  const back = field(pointer, "maxBack");
  if (prevented === true) {
    if (process.platform === "linux") {
      s.detail?.(`titlebar: maximize reading recorded, not asserted (no window manager under CI's Xvfb, Q-0022-04): ${String(before)} -> ${String(after)} -> ${String(back)}`);
    } else {
      const mac = process.platform === "darwin" ? "; on macOS AppleActionOnDoubleClick decides the double-click (None and Minimize change this, the default zooms), Q-0022-03" : "";
      s.check(after === true, `titlebar: the double-click press did not maximize (isMaximized ${String(before)} -> ${String(after)})${mac}`);
      s.check(back === false, `titlebar: the second double-click press did not restore (isMaximized ${String(back)})${mac}`);
    }
  }
  const themePress = field(pointer, "themePress");
  s.check(field(themePress, "prevented") === false, `titlebar: a press on the theme control inside the title area was taken over as a window drag: ${JSON.stringify(themePress)}`);
  const themeBefore = stringField(pointer, "themeBefore") ?? "";
  const themeFirst = stringField(pointer, "themeFirst");
  const themeBack = stringField(pointer, "themeBack");
  s.check(
    themeFirst !== undefined && themeFirst !== "" && themeFirst !== themeBefore,
    `titlebar: the theme control did not switch the theme (${themeBefore} -> ${String(themeFirst)})`,
  );
  s.check(
    themeBack === themeBefore && themeBefore !== "",
    `titlebar: the theme control did not switch the theme back (-> ${String(themeBack)}, want ${themeBefore || "the first reading"})`,
  );
}
