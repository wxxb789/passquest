// Settings: the provider and model registry, the services, the budget and the consent ledger, and the
// actions that change them. The registry, service and ledger loaders are shared with the rooms that
// show them (feedback, Anki, the consent dialog).
import { T } from '../core/i18n';
import { closeLayer, openLayer, render, renderLayer } from '../core/render';
import { data, ui, valueOf } from '../core/state';
import { toast } from '../core/toast';
import { $, type Act } from './dom';
import { failureLine, load, loadToday, write, writeOk } from './service';

export const loadServices = () => load((a) => a.services(), (l) => (data.services = l), valueOf(data.services));
export const loadModels = () => load((a) => a.models(), (l) => (data.models = l), valueOf(data.models));
export const loadConsents = () => load((a) => a.consents(), (l) => (data.settings.consents = l), valueOf(data.settings.consents));

export async function loadSettings(): Promise<void> {
  const s = data.settings;
  await Promise.all([
    loadModels(),
    loadServices(),
    load((a) => a.budget(), (l) => (s.budget = l), valueOf(s.budget)),
    loadConsents(),
  ]);
  if (ui.layer?.startsWith('sheet:')) renderLayer(false);
}

export const settingsActs: Record<string, Act> = {
  row(v) {
    if (!v) return;
    ui.settings.open = ui.settings.open === v ? null : v;
    render();
  },
  'model-use'(v) {
    if (!v) return;
    void write((a) => a.useModel(v, '')).then((res) => {
      if (!res) return;
      toast(T(`已使用 ${res.provider_name} · ${res.active.model ?? ''}`, `Using ${res.provider_name} · ${res.active.model ?? ''}`));
      void loadSettings();
      void loadToday();
    });
  },
  'model-test'(v) {
    if (!v) return;
    void write((a) => a.testModel(v, '')).then((res) => {
      if (!res) return;
      toast(res.failure ? `${T('测试未通过', 'Test failed')} · ${failureLine({ code: res.failure.code, message: res.failure.message, action: null })}` : T(`${res.provider_name} · ${res.model} 已验证`, `${res.provider_name} · ${res.model} verified`));
      void loadSettings();
    });
  },
  cred(v) {
    if (!v) return;
    ui.settings.credentialFor = v;
    openLayer('credential');
  },
  'credential-save'() {
    const provider = ui.settings.credentialFor;
    const input = $<HTMLInputElement>('#credform input[name="key"]');
    if (!provider || !input) return;
    const key = input.value;
    // the key must not linger in the DOM, in state, or in a re-render
    input.value = '';
    if (!key.trim()) return;
    closeLayer();
    void writeOk((a) => a.setCredential(provider, key)).then((ok) => {
      if (!ok) return;
      ui.settings.credentialFor = null;
      toast(T('密钥已存入系统密钥库', 'The key is in the OS credential store'));
      void loadSettings();
      void loadToday();
    });
  },
  'cred-clear'(v) {
    if (!v) return;
    void writeOk((a) => a.clearCredential(v)).then((ok) => {
      if (!ok) return;
      void loadSettings();
      void loadToday();
    });
  },
};
