import { pageScript } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { TITLEBAR_LIBRARY } from "../../lib/ux-page.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

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
return { size: size, stops: stops };
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
  },
};
