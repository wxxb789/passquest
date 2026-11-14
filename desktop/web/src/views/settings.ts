// Settings (prototype vSettings + budget/consent/services sheets). Providers, models, capabilities
// and authorization are four separate facts. Credentials are shown only as present / missing (and
// where the presence comes from); keys entered here go to the OS credential store.
import type { Budget, Consent, CredentialStatus, Model, ModelList, ModelProvider, Services } from '../api/types';
import { html, type Html } from '../core/html';
import { capabilityOf, shortStamp, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { sat, type Flag } from '../components/satellite';
import { sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { unusableLine } from './coach';
import { errorCard, loadingCard, retryButton } from './states';

export interface SettingsData {
  models: Loadable<ModelList>;
  budget: Loadable<Budget>;
  consents: Loadable<Consent[]>;
  services: Loadable<Services>;
  /** Private sources known locally (imported, reviewed), for granting consent. */
  sources: string[];
}

export interface SettingsUi {
  open: string | null;
  busy: boolean;
}

const monogram = (name: string) => name.replace(/[^A-Za-z0-9]/g, '').slice(0, 2).toUpperCase() || '··';
const textChat = (m: Model | undefined) => m?.capabilities.find((c) => c.id === 'text.chat');

/** The model a provider row shows: the active one, else the registry default, else the first. */
function shownModel(p: ModelProvider): Model | undefined {
  return p.models.find((m) => m.active) ?? p.models.find((m) => m.default) ?? p.models[0];
}

function credentialOf(p: ModelProvider, s: Services | null): CredentialStatus | undefined {
  return s?.credentials.find((c) => c.provider === p.id);
}

function auth(p: ModelProvider, cred: CredentialStatus | undefined): Html {
  if (p.auth === 'present')
    return html`<span class="auth ok" data-credential="present">${ic('key', 'xs')}${T('有密钥', 'Key present')}${
      cred?.from === 'env' ? html` <span class="muted">${T('环境变量', 'env')}</span>` : cred?.from === 'store' ? html` <span class="muted">${T('密钥库', 'store')}</span>` : ''
    }</span>`;
  return html`<span class="auth no" data-credential="missing">${ic('key', 'xs')}${T('无密钥', 'No key')}</span>`;
}

function primaryAction(p: ModelProvider, m: Model | undefined, busy: boolean): Html {
  const d = busy ? 'disabled' : '';
  if (p.active && m?.active) return html`<span class="inuse">${ic('check', 'xs')}${T('使用中', 'In use')}</span>`;
  if (p.auth !== 'present') return html`<button class="btn ghost sm" data-act="cred" data-v="${p.id}">${T('添加密钥', 'Add key')}</button>`;
  if (!m) return html`<span class="muted">—</span>`;
  if (textChat(m)?.verified) return html`<button class="btn ghost sm" data-act="model-use" data-v="${p.id}/${m.id}" ${d}>${T('使用', 'Use')}</button>`;
  return html`<button class="btn ghost sm" data-act="model-test" data-v="${p.id}/${m.id}" ${d}>${T('测试', 'Test')}</button>`;
}

function modelLine(p: ModelProvider, m: Model, busy: boolean): Html {
  const tc = textChat(m);
  const c = capabilityOf(tc?.state ?? 'unsupported');
  const d = busy ? 'disabled' : '';
  return html`<div class="capgrid" data-model="${p.id}/${m.id}"><span class="cg ${c[3]}"><code>${m.id}</code><span>${m.label ?? ''}${m.active ? html` · <b>${T('使用中', 'in use')}</b>` : ''}</span></span>
      ${m.capabilities.map((k) => {
        const s = capabilityOf(k.state);
        return html`<span class="cg ${s[3]}"><code>${k.id}</code><span>${T(s[0], s[1])}${k.checked_at ? html` · <span class="num">${shortStamp(k.checked_at)}</span>` : ''}</span></span>`;
      })}
      <span class="acts-row">${m.active ? '' : html`<button class="btn ghost sm" data-act="model-use" data-v="${p.id}/${m.id}" ${d}>${T('设为使用', 'Use this model')}</button>`}<button class="btn ghost sm" data-act="model-test" data-v="${p.id}/${m.id}" ${d}>${T('测试（1 次外部调用）', 'Test (1 external call)')}</button>${
        tc?.failure_type ? html`<span class="fine" style="margin:0">${T('上次失败', 'last failure')}: ${tc.failure_type}</span>` : ''
      }</span></div>`;
}

function row(p: ModelProvider, open: boolean, busy: boolean, s: Services | null): Html {
  const m = shownModel(p);
  const tc = textChat(m);
  const c = capabilityOf(tc?.state ?? 'unsupported');
  const cred = credentialOf(p, s);
  const d = busy ? 'disabled' : '';
  return html`<div class="rrow ${open ? 'open' : ''}" data-provider="${p.id}">
      <span class="pv-name" title="${p.name}"><span class="mono-mk" aria-hidden="true">${monogram(p.name)}</span><span class="nm">${p.name}</span></span>
      <span class="model ${m ? '' : 'none'}">${m?.id ?? T('没有模型', 'no model')}</span>
      <span class="cap ${c[3]}" data-capability="${tc?.state ?? 'unsupported'}">${ic(c[2], 's')}${T(c[0], c[1])}${tc?.checked_at ? html`<span class="t num">${shortStamp(tc.checked_at)}</span>` : ''}</span>
      ${auth(p, cred)}
      <span class="act">${primaryAction(p, m, busy)}</span>
      <button class="disc" data-act="row" data-v="${p.id}" aria-expanded="${open}" aria-label="${T(`${p.name} 的全部模型与能力`, `All models and capabilities of ${p.name}`)}">${ic('chev', 's')}</button>
      ${
        open
          ? html`<div class="rr-more" style="grid-column:1 / -1">${p.models.map((x) => modelLine(p, x, busy))}
            <div class="sh-acts" style="margin:4px 0 0">
              <span class="fine" style="margin:0">${T('环境变量', 'Environment variable')} <code>${p.credential_env}</code>${cred?.from === 'env' ? T('（已设置，优先于密钥库）', ' (set; it wins over the store)') : ''}</span>
              <button class="btn ghost sm" data-act="cred" data-v="${p.id}">${p.auth === 'present' ? T('更换密钥', 'Replace key') : T('添加密钥', 'Add key')}</button>
              ${cred?.from === 'store' ? html`<button class="btn quiet sm" data-act="cred-clear" data-v="${p.id}" ${d}>${T('移除密钥', 'Remove key')}</button>` : ''}
            </div></div>`
          : ''
      }
    </div>`;
}

export function vSettings(d: SettingsData, u: SettingsUi): Html {
  const m = d.models;
  if (m.status === 'error') return errorCard(m.error, retryButton('reload'));
  if (m.status !== 'ready') return loadingCard();
  const reg = m.value;
  const b = d.budget.status === 'ready' ? d.budget.value : null;
  const c = d.consents.status === 'ready' ? d.consents.value : null;
  const s = d.services.status === 'ready' ? d.services.value : null;
  const granted = c ? c.filter((x) => x.granted).length : 0;
  // A source never decided has no recorded event; only a recorded "no" is a revocation.
  const revoked = c ? c.filter((x) => !x.granted && x.recorded_at).length : 0;
  const svcFlag: Flag = s ? (s.node_available ? 'ok' : 'bad') : '';
  const unusable = unusableLine(reg);
  return html`<div class="scene orbit solo" style="--obj-w:1000px" data-testid="settings">
    <section class="obj card reg rise" style="--i:0" aria-labelledby="reg-t">
      <div class="reg-h">
        <h1 class="h-disp" id="reg-t">${T('模型与提供方', 'Models & providers')}</h1>
        <p>${ic('lock', 's')}${
          s === null
            ? T('凭据只显示有 / 无，从不显示密钥本身。', 'Credentials are shown only as present / missing, never displayed.')
            : s.credential_store
              ? T('凭据保存在系统密钥库，这里只显示有 / 无，从不显示密钥本身。', 'Credentials live in the OS secret store — shown only as present / missing, never displayed.')
              : T('本机没有可用的系统密钥库，只能使用环境变量。', 'No OS credential store here; only environment variables work.')
        }</p>
        ${unusable ? html`<p class="held" data-testid="providers-unusable">${ic('info', 's')}${unusable}</p>` : ''}
      </div>
      <div class="reg-cols" aria-hidden="true"><span><i>1</i>${T('提供方', 'Provider')}</span><span><i>2</i>${T('模型', 'Model')}</span><span><i>3</i>${T('能力', 'Capability')}<span class="id">text.chat</span></span><span><i>4</i>${T(
        '授权',
        'Authorization',
      )}</span><span style="justify-self:end">${T('状态', 'State')}</span><span></span></div>
      <div class="reg-b">${reg.providers.map((p) => row(p, u.open === p.id, u.busy, s))}</div>
      <div class="reg-f">
        <span>${ic('seal', 'xs')}${T('只有 text.chat 能被验证', 'Only text.chat is ever verified')}</span>
        <span>${ic('adv', 'xs')}${T('已声明 · 未验证：提供方声称支持，未经测试', 'Declared, not tested: claimed by the provider')}</span>
        <span>${ic('minus', 'xs')}${T('不支持', 'Unsupported')}</span>
      </div>
    </section>
    <div class="rail r">
      ${sat('budget', 'pulse', T('每日调用预算', 'Daily call budget'), b ? html`<span class="num">${b.total_calls} / ${b.total_limit}</span><small>${T('今日 · UTC', 'today · UTC')}</small>` : html`<small>—</small>`, '', 1)}
      ${sat(
        'consent',
        'shield',
        T('私有来源的授权', 'Consent · private sources'),
        c ? html`${T(`${granted} 项授权`, `${granted} granted`)}<small>${T(`· ${revoked} 已撤回`, `· ${revoked} revoked`)}</small>` : html`<small>—</small>`,
        c && revoked > 0 ? 'warm' : c && granted > 0 ? 'ok' : '',
        2,
      )}
      ${sat(
        'services',
        'server',
        T('本地服务', 'Local services'),
        s ? html`Node<small>${s.node_available ? T('可用', 'available') : T('不可用', 'unavailable')}</small>` : html`<small>—</small>`,
        svcFlag,
        3,
      )}
    </div>
  </div>`;
}

// ---------------------------------------------------------------- sheets

function budgetSheet(b: Budget): Html {
  return sheetWrap(
    'pulse',
    T('每日调用预算', 'Daily call budget'),
    T(`每个提供方每天 ${b.per_provider_limit} 次 · 按 UTC 日计`, `${b.per_provider_limit} per provider per day · UTC days`),
    html`${b.providers.map(
      (p) =>
        html`<div class="bud" data-provider="${p.provider}"><span>${p.label || p.provider}${p.active ? html` <span class="muted">· ${T('使用中', 'in use')}</span>` : ''}</span><span class="m"><i style="width:${
          p.limit ? Math.min(100, (p.calls / p.limit) * 100).toFixed(1) : 0
        }%"></i></span><span class="v num">${p.calls} / ${p.limit}</span></div>`,
    )}
      <div class="bsum"><span>${T('今日合计', 'Today, total')}</span><b class="num">${b.total_calls} / ${b.total_limit}</b></div>
      <p class="fine">${T(
        `真实上限 = 提供方数 × 单个限额（${b.provider_count} × ${b.per_provider_limit}）。`,
        `The real ceiling is providers × limit (${b.provider_count} × ${b.per_provider_limit}).`,
      )}<br>${T('故障转移只去：', 'Failover only to a provider that is ')}<b>${T('已验证 + 有余额 + 授权范围完全一致', 'verified, has budget and an exactly equal consent scope')}</b>${T(' 的提供方。', '.')}</p>`,
  );
}

function consentSheet(c: Consent[], d: SettingsData, u: SettingsUi): Html {
  const providers = d.models.status === 'ready' ? d.models.value.providers : [];
  const name = (id: string | undefined) => (id ? (providers.find((p) => p.id === id)?.name ?? id) : T('所有提供方', 'every provider'));
  const dis = u.busy ? 'disabled' : '';
  const rows = c.map(
    (x) => html`<div class="svc" data-testid="consent" data-source="${x.source_id}" data-provider="${x.provider ?? ''}" data-granted="${x.granted}"><span class="state-dot ${x.granted ? 'ok' : 'bad'}"></span><span>${x.source_id} → ${name(x.provider)} <span class="muted">${shortStamp(x.recorded_at)}${x.reviewer ? ` · ${x.reviewer}` : ''}</span></span><span class="st ${x.granted ? 'ok' : 'bad'}">${
      x.granted ? T('已授予', 'granted') : x.recorded_at ? T('已撤回', 'revoked') : T('未授权', 'not granted')
    }${
      x.granted
        ? html` <button class="btn ghost sm" data-act="consent-revoke" data-v="${x.source_id}|${x.provider ?? ''}" ${dis}>${T('撤回', 'Revoke')}</button>`
        : x.recorded_at
          ? html` <button class="btn ghost sm" data-act="consent-grant" data-v="${x.source_id}|${x.provider ?? ''}" ${dis}>${T('重新授予', 'Grant again')}</button>`
          : ''
    }</span></div>`,
  );
  const known = new Set(c.map((x) => x.source_id));
  const fresh = d.sources.filter((s) => !known.has(s));
  const grantForm = d.sources.length
    ? html`<form class="pq-form confirm" id="consentform" data-form="consent"><h3>${T('授予前确认范围', 'Confirm the scope before granting')}</h3>
        <p class="fine">${T('授权后，这份私有导入资料的题干、选项、证据句等原文可以发给所选提供方；可以随时撤回。', 'Once granted, the stem, options and evidence text of this private import may be sent to the chosen provider; you can revoke it at any time.')}</p>
        <label class="field"><span class="fl">${T('来源', 'Source')}</span><select name="source">${[...fresh, ...d.sources.filter((s) => known.has(s))].map((s) => html`<option value="${s}">${s}</option>`)}</select></label>
        <label class="field"><span class="fl">${T('提供方', 'Provider')}</span><select name="provider">${providers.map((p) => html`<option value="${p.id}">${p.name}</option>`)}</select></label>
        <div class="sh-acts" style="margin-top:8px"><button class="btn primary sm" data-act="consent-form" ${dis}>${T('确认授予', 'Grant')}</button></div></form>`
    : '';
  return sheetWrap(
    'shield',
    T('私有来源的授权', 'Consent · private sources'),
    T('只追加事件，不改历史', 'Appends an event; history stays'),
    html`${rows.length ? rows : html`<p class="fine">${T('还没有任何授权记录。', 'No consent recorded yet.')}</p>`}
        ${grantForm}
        <p class="fine">${T('第一方自有内容（技能统计、已审核战术）不需要授权。', 'First-party content (skill counts, reviewed tactics) needs no consent.')}</p>`,
  );
}

function servicesSheet(s: Services): Html {
  return sheetWrap(
    'server',
    T('本地服务', 'Local services'),
    T('全部在本机运行', 'All on this machine'),
    html`<div class="svc" data-testid="svc-node" data-available="${s.node_available}"><span class="state-dot ${s.node_available ? 'ok' : 'bad'}"></span><span>${T('Agent 宿主', 'Agent host')} <span class="muted">(Node${
      s.node_path ? html` · <code>${s.node_path}</code>` : ''
    })</span></span><span class="st ${s.node_available ? 'ok' : 'bad'}">${s.node_available ? T('可用 · 按次启动', 'available · started per call') : T('不可用', 'unavailable')}</span></div>
      ${s.node_available || !s.node_action ? '' : html`<p class="fine">${s.node_action}</p>`}
      <div class="svc"><span class="state-dot off"></span><span>AnkiConnect <code class="muted">${s.anki_endpoint}</code></span><span class="st">${T('推送时才连接', 'contacted only on push')}</span></div>
      <div class="svc"><span class="state-dot ${s.search_configured ? 'ok' : 'off'}"></span><span>${T('搜索提供方', 'Search provider')} <code class="muted">${s.search_env}</code></span><span class="st">${
        s.search_configured ? T('已配置', 'configured') : T('未配置', 'not configured')
      }</span></div>
      <div class="svc"><span class="state-dot ${s.credential_store ? 'ok' : 'bad'}"></span><span>${T('系统密钥库', 'OS credential store')}</span><span class="st">${s.credential_store ? T('可用', 'available') : T('不可用', 'unavailable')}</span></div>
      <div class="svc"><span class="state-dot ok"></span><span>${T('数据库', 'Database')}</span><span class="st"><code>${s.database}</code></span></div>`,
  );
}

export function settingsSheet(key: string, d: SettingsData, u: SettingsUi): Html | null {
  switch (key) {
    case 'budget':
      return d.budget.status === 'ready' ? budgetSheet(d.budget.value) : null;
    case 'consent':
      return d.consents.status === 'ready' ? consentSheet(d.consents.value, d, u) : null;
    case 'services':
      return d.services.status === 'ready' ? servicesSheet(d.services.value) : null;
  }
  return null;
}
