# Goal 0012 rendered-screen contact sheets

Each `.webp` file is a contact sheet for one state of the C · Stage desktop workspace. A sheet has
four tiles in a 2×2 grid: light-zh and light-en on the top row, dark-zh and dark-en on the bottom row.
Every tile is a 1280×800 page area, downscaled to 640×400.

The screens were shot on commit `354f08c`. They come from the MyGo automation (e2e) build of
`cmd/passquest-desktop`, running on an isolated data directory. That directory was seeded through
the CLI with placeholder notices, not exam material. A loopback provider stub and a loopback
AnkiConnect stub stood in for the real services, so no external network calls were made.

The sheets show what the gates checked, but they do not replace the gates. Acceptance still rests
on `desktop-ux` and the other gates in [`docs/harness.md`](../../harness.md).

| Sheet | State |
| --- | --- |
| [today](today.webp) | Today stage before any attempt: the next-step hero and the surrounding status satellites. |
| [today-brief](today-brief.webp) | Today after practice and two runs, with the study-brief sheet open. |
| [practice-sheet](practice-sheet.webp) | The "choose practice" sheet opened from Today. |
| [shortcuts](shortcuts.webp) | The keyboard shortcuts panel (`?`). |
| [practice-guided](practice-guided.webp) | A guided tactic walkthrough item, recorded as assisted. |
| [practice-independent](practice-independent.webp) | An independent skill item that can count as transfer. |
| [hint-gate](hint-gate.webp) | The confirmation gate shown before a hint makes the attempt assisted. |
| [feedback-coach](feedback-coach.webp) | Feedback on a wrong answer with the coach panel and its reply from the stub. |
| [anki-draft](anki-draft.webp) | An Anki note draft proposed from the answer: a front/back card preview. |
| [anki-edit](anki-edit.webp) | The note draft with the field editor open. |
| [anki-refine](anki-refine.webp) | The editor with the "refine with AI" call strip (advisory, asks first). |
| [anki-notes](anki-notes.webp) | The notes list after the draft was accepted, still local and not pushed. |
| [assess-idle](assess-idle.webp) | The assessment entry: a timed, unassisted, unseen run. |
| [assess-running](assess-running.webp) | Item 1 of a running 3-item, 30-minute assessment run. |
| [report](report.webp) | The run report with a like-for-like comparison of run #2 against run #1. |
| [report-annotations](report-annotations.webp) | Per-item annotations unlocked after the run: evidence and reasoning. |
| [settings](settings.webp) | Models and providers with the stub provider row expanded. |
