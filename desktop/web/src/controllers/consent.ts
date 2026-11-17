// Consent grants and revocations, from whichever room asks (coach, Anki refinement, settings): a
// change makes every preview and the ledger stale, so the rooms that show them read again.
import { closeLayer, openLayer, render } from '../core/render';
import { data, idle, ui } from '../core/state';
import { $, type Act } from './dom';
import { writeOk } from './service';
import { loadPreview, loadRefinePreview } from './anki';
import { loadCoachPreview } from './feedback';
import { loadConsents, loadModels, loadSettings } from './settings';

function consentArgs(v: string | undefined): [string, string] | null {
  const [source, provider] = (v ?? '').split('|');
  return source ? [source, provider ?? ''] : null;
}

/** After a consent change the preview and the ledger are stale. */
function consentChanged(): void {
  data.coach.preview = idle();
  if (ui.route === 'feedback') void loadCoachPreview();
  if (ui.route === 'settings') void loadSettings();
  if (ui.route === 'anki') {
    data.anki.preview = idle();
    void loadPreview();
    if (ui.anki.refine && !data.anki.refine.suggestion) void loadRefinePreview();
  }
  render(true, false);
}

export const consentActs: Record<string, Act> = {
  'consent-grant'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.grantConsent(k[0], k[1])).then((ok) => ok && consentChanged());
  },
  'consent-revoke'(v) {
    const k = consentArgs(v);
    if (k) void writeOk((a) => a.revokeConsent(k[0], k[1])).then((ok) => ok && consentChanged());
  },
  'consent-all-ask'(v) {
    if (!v) return;
    ui.anki.consentSource = v;
    // The dialog lists the registry's providers the grant reaches and names the source by its title
    // and the providers keeping their own refusal (from a fresh ledger read); the loads re-render it in place.
    if (data.models.status === 'idle' || data.models.status === 'error') void loadModels();
    void loadConsents();
    openLayer('consent-all');
  },
  'consent-all'(v) {
    if (!v || v !== ui.anki.consentSource) return;
    closeLayer();
    ui.anki.consentSource = null;
    void writeOk((a) => a.grantConsent(v, '')).then((ok) => ok && consentChanged());
  },
  'consent-form'() {
    const form = $<HTMLFormElement>('#consentform');
    if (!form) return;
    const fd = new FormData(form);
    const source = String(fd.get('source') ?? '');
    const provider = String(fd.get('provider') ?? '');
    if (source && provider) consentActs['consent-grant']!(`${source}|${provider}`, form);
  },
};
