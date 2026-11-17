import { field } from "../../lib/json.ts";
import { launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

export const lifecycle: Scenario = {
  id: "lifecycle",
  async run(s) {
    const launched = await launch(s, {
      label: "lifecycle",
      script: "await new Promise((done) => setTimeout(done, 2000));\nreturn { waited: true };",
      probeListeners: true,
    });
    if (launched) s.check(field(launched.value, "waited") === true, `lifecycle: script returned ${JSON.stringify(launched.value)}`);
  },
};
