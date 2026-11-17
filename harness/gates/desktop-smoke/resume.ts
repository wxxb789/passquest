import { Database } from "bun:sqlite";
import { existsSync } from "node:fs";

import { pageScript } from "../../lib/desktop-page.ts";
import { field, stringField } from "../../lib/json.ts";
import { attemptsOf, cli, instant, launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

/** The latest displayed, unanswered practice item as the store keeps it (what a relaunch resumes). */
function openPresentation(database: string): { token: string; presentedAt: string } | null {
  if (!existsSync(database)) return null;
  let db: Database | undefined;
  try {
    db = new Database(database, { readonly: true });
    const row = db
      .query("SELECT token, presented_at FROM practice_presentations WHERE closed_at = '' ORDER BY rowid DESC LIMIT 1")
      .get();
    const token = stringField(row, "token");
    const presentedAt = stringField(row, "presented_at");
    return token && presentedAt ? { token, presentedAt } : null;
  } catch {
    // Not created yet, or busy with a write: look again on the next poll.
    return null;
  } finally {
    db?.close();
  }
}

export const resume: Scenario = {
  id: "resume",
  async run(s) {
    const displayed: { item: { token: string; presentedAt: string } | null } = { item: null };
    const killedRun = await launch(s, {
      label: "present",
      script: pageScript(`
await today();
press("Enter");
await until("a practice item", () => q('[data-testid="practice-item"]'));
// Wait for the kill. The pending promise stays reachable through window: WebKit fails a script whose
// unsettled promise is garbage-collected ("Completion handler ... is no longer reachable").
await new Promise((hold) => { window.__pqHold = hold; });
`),
      // Kill one poll after the store holds the open presentation.
      killWhen: async () => {
        if (displayed.item) return true;
        displayed.item = openPresentation(s.database);
        return false;
      },
    });
    if (!killedRun) return;
    const kept = displayed.item;
    if (!s.check(killedRun.killed && kept !== null, "resume: no item was presented, so nothing was killed mid-item") || !kept) return;
    const relaunchedAt = Date.now();
    const relaunched = await launch(s, {
      label: "relaunch",
      script: pageScript(`
await until("the resumed item", () => q('[data-testid="practice-item"]'), 20000);
return { item: data('[data-testid="practice-item"]') };
`),
    });
    if (!relaunched) return;
    const item = field(relaunched.value, "item");
    s.check(stringField(item, "token") === kept.token, "resume: the relaunched window shows another presentation token");
    s.check(stringField(item, "presentedAt") === kept.presentedAt, "resume: the relaunched window shows another presentation time");
    s.check(instant(stringField(item, "presentedAt")) < relaunchedAt, "resume: the item was presented again on relaunch");
    const attempts = attemptsOf(await cli(s, ["report"]));
    s.check(attempts.length === 0, `resume: the killed item wrote ${attempts.length} attempt(s)`);
  },
};
