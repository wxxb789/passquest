import { pageScript, STUB_MODEL } from "../../lib/desktop-page.ts";
import { field, stringField } from "../../lib/json.ts";
import { launch, setup } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

/**
 * A hand-configured endpoint (`tutor config --endpoint --model`, the legacy manual configuration): the
 * core previews its calls with no registry provider id. The coach and the refine panel must show that
 * preview — its call strip and a Send naming the endpoint's host — not a missing configuration.
 */
export const manual: Scenario = {
  id: "manual",
  async run(s) {
    const endpoint = s.shared.provider.endpoint;
    if (!(await setup(s, [["tutor", "config", "--endpoint", endpoint, "--model", STUB_MODEL.split("/")[1]]]))) return;
    const launched = await launch(s, {
      label: "manual",
      script: pageScript(`
const shown = (element) => ({
  send: element.matches("[data-act]"),
  text: element.textContent.replace(/\\s+/g, " ").trim(),
});
await today();
press("Enter");
await answer("1");
const coach = await until("the coach preview or its failure", () => q('[data-act="coach-send"]') || q('[data-testid="coach-failure"]'), 20000);
const coachShown = { ...shown(coach), strip: !!q('[data-testid="call-strip"]') };
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
press("e");
await until("the front field", () => q('[data-field="front"]'));
await click('.esheet [data-act="refine"]');
const refine = await until("the refine preview or its failure", () => q('[data-act="refine-send"]') || q('[data-testid="refine-failure"]'), 20000);
return { coach: coachShown, refine: { ...shown(refine), strip: !!q('[data-testid="refine-strip"]') } };
`),
    });
    if (!launched) return;
    const host = new URL(endpoint).host;
    for (const name of ["coach", "refine"]) {
      const shown = field(launched.value, name);
      s.check(
        field(shown, "send") === true && field(shown, "strip") === true,
        `manual: the ${name} shows ${String(stringField(shown, "text"))} instead of the call strip and Send for the configured endpoint`,
      );
      s.check(stringField(shown, "text")?.includes(host) === true, `manual: the ${name}'s Send does not name the endpoint ${host}: ${String(stringField(shown, "text"))}`);
    }
  },
};
