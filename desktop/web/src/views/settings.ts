// Settings (prototype vSettings + budget/consent/services/routes sheets). Providers, models,
// capabilities and authorization are four separate facts. Credentials are shown only as
// present / missing; keys entered here go to the OS credential store.
import type { BudgetView, ConsentLedger, ModelRegistry, ProviderRow, Services } from '../api/types';
import { html, type Html } from '../core/html';
import { CAPABILITY, shortStamp, T } from '../core/i18n';
import type { Loadable } from '../core/state';
import { sat, type Flag } from '../components/satellite';
import { sheetWrap } from '../components/sheet';
import { ic } from '../icons';
import { errorCard, loadingCard, retryButton } from './states';

export interface SettingsData {
  models: Loadable<ModelRegistry>;
  budget: Loadable<BudgetView>;
  consents: Loadable<ConsentLedger>;
  services: Loadable<Services>;
}

export interface SettingsUi {
  open: string | null;
  confirming: { source_id: string; provider: string } | null;
  busy: boolean;
}

const monogram = (name: string) => name.replace(/[^A-Za-z0-9]/g, '').slice(0, 2).toUpperCase() || '··';

function auth(p: ProviderRow): Html {
  if (p.credential === 'present')
    return html`<span class="auth ok" data-credential="present">${ic('key', 'xs')}${T('有密钥', 'Key present')}${p.credential_from_env ? html` <span class="muted">env</span>` : ''}</span>`;
  if (p.credential === 'missing') return html`<span class="auth no" title="no_credential" data-credential="missing">${ic('key', 'xs')}${T('无密钥', 'No key')}</span>`;
  return html`<span class="auth no">—</span>`;
}

function primaryAction(p: ProviderRow, busy: boolean): Html {
  const d = busy ? 'disabled' : '';
  if (p.in_use) return html`<span class="inuse">${ic('check', 'xs')}${T('使用中', 'In use')}</span>`;
  if (p.credential === 'missing') return html`<button class="btn ghost sm" data-act="cred" data-v="${p.id}">${T('添加密钥', 'Add key')}</button>`;
  if (!p.model) return html`<button class="btn ghost sm" data-act="row" data-v="${p.id}">${T('配置', 'Configure')}</button>`;
  if (p.text_chat.state === 'verified') return html`<button class="btn ghost sm" data-act="model-use" data-v="${p.id}" ${d}>${T('使用', 'Use')}</button>`;
  return html`<button class="btn ghost sm" data-act="model-test" data-v="${p.id}" ${d}>${T('测试', 'Test')}</button>`;
}

function row(p: ProviderRow, open: boolean, busy: boolean): Html {
  const c = CAPABILITY[p.text_chat.state];
  const d = busy ? 'disabled' : '';
  return html`<div class="rrow ${open ? 'open' : ''}" data-provider="${p.id}">
      <span class="pv-name"><span class="mono-mk" aria-hidden="true">${monogram(p.name)}</span>${p.name}</span>
      <span class="model ${p.model ? '' : 'none'}">${p.model ?? T('未配置', 'not configured')}</span>
      <span class="cap ${c[3]}" title="text.chat ${p.text_chat.state}${p.text_chat.failure_type ? ' · ' + p.text_chat.failure_type : ''}">${ic(c[2], 's')}${T(c[0], c[1])}${
        p.text_chat.checked_at ? html`<span class="t num">${shortStamp(p.text_chat.checked_at)}</span>` : ''
      }</span>
      ${auth(p)}
      <span class="act">${primaryAction(p, busy)}</span>
      <button class="disc" data-act="row" data-v="${p.id}" aria-expanded="${open}" aria-label="${T(`${p.name} 的全部能力`, `All capabilities of ${p.name}`)}">${ic('chev', 's')}</button>
      ${
        open
          ? html`<div class="capgrid">${p.capabilities.map((k) => {
              const s = CAPABILITY[k.state];
              return html`<span class="cg ${s[3]}"><code>${k.name}</code><span>${T(s[0], s[1])}</span></span>`;
            })}</div>
            <div class="sh-acts" style="grid-column:1 / -1;margin:4px 0 0">
              ${p.model && !p.in_use ? html`<button class="btn ghost sm" data-act="model-use" data-v="${p.id}" ${d}>${T('设为使用', 'Use this model')}</button>` : ''}
              ${p.model ? html`<button class="btn ghost sm" data-act="model-test" data-v="${p.id}" ${d}>${T('测试（1 次外部调用）', 'Test (1 external call)')}</button>` : ''}
              ${p.credential !== 'not_required' ? html`<button class="btn ghost sm" data-act="cred" data-v="${p.id}">${p.credential === 'present' ? T('更换密钥', 'Replace key') : T('添加密钥', 'Add key')}</button>` : ''}
              ${p.credential === 'present' && !p.credential_from_env ? html`<button class="btn quiet sm" data-act="cred-clear" data-v="${p.id}" ${d}>${T('移除密钥', 'Remove key')}</button>` : ''}
              ${p.text_chat.action ? html`<span class="fine" style="margin:0"><code>${p.text_chat.action}</code></span>` : ''}
            </div>`
          : ''
      }
    </div>`;
}

export function vSettings(d: SettingsData, u: SettingsUi): Html {
  const m = d.models;
  if (m.status === 'error') return errorCard(m.error, retryButton('reload'));
  if (m.status !== 'ready') return loadingCard();
  const reg = m.value;
  const active = reg.providers.find((p) => p.id === reg.active_id) ?? null;
  const b = d.budget.status === 'ready' ? d.budget.value : null;
  const c = d.consents.status === 'ready' ? d.consents.value : null;
  const s = d.services.status === 'ready' ? d.services.value : null;
  const granted = c ? c.current.filter((x) => x.granted).length : 0;
  const revoked = c ? c.current.length - granted : 0;
  const svcFlag: Flag = s ? (s.anki_connect.reachable === false || !s.node.available ? 'bad' : 'ok') : '';
  return html`<div class="scene orbit solo" style="--obj-w:1000px">
    <section class="obj card reg rise" style="--i:0" aria-labelledby="reg-t">
      <div class="reg-h">
        <h1 class="h-disp" id="reg-t">${T('模型与提供方', 'Models & providers')}</h1>
        <p>${ic('lock', 's')}${
          reg.credential_store.available
            ? T('凭据保存在系统密钥库，这里只显示有 / 无，从不显示密钥本身。', 'Credentials live in the OS secret store — shown only as present / missing, never displayed.')
            : T(`本机没有可用的系统密钥库（${reg.credential_store.reason ?? ''}），只能使用环境变量。`, `No OS credential store here (${reg.credential_store.reason ?? ''}); only environment variables work.`)
        }</p>
      </div>
      <div class="reg-cols" aria-hidden="true"><span><i>1</i>${T('提供方', 'Provider')}</span><span><i>2</i>${T('模型', 'Model')}</span><span><i>3</i>${T('能力', 'Capability')}<span class="id">text.chat</span></span><span><i>4</i>${T(
        '授权',
        'Authorization',
      )}</span><span style="justify-self:end">${T('状态', 'State')}</span><span></span></div>
      <div class="reg-b">${reg.providers.map((p) => row(p, u.open === p.id, u.busy))}</div>
      <div class="reg-f">
        <span>${ic('seal', 'xs')}${T('只有 text.chat 能被验证', 'Only text.chat is ever verified')}</span>
        <span>${ic('adv', 'xs')}${T('已声明 · 未验证：提供方声称支持，未经测试', 'Declared, not tested: claimed by the provider')}</span>
        <span>${ic('minus', 'xs')}${T('不支持', 'Unsupported')}</span>
      </div>
    </section>
    <div class="rail r">
      ${sat('budget', 'pulse', T('每日调用预算', 'Daily call budget'), b ? html`<span class="num">${b.total_calls} / ${b.total_limit}</span><small>${T('今日', 'today')}</small>` : html`<small>—</small>`, '', 1)}
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
        s ? html`AnkiConnect<small>${s.anki_connect.reachable === null ? T('未检查', 'not checked') : s.anki_connect.reachable ? T('可连接', 'reachable') : T('无法连接', 'unreachable')}</small>` : html`<small>—</small>`,
        svcFlag,
        3,
      )}
      ${sat('routes', 'route', T('用途分配', 'Which model does what'), active?.model ? html`${active.model}<small>${reg.failover.length ? T(`· ${reg.failover.length} 个故障转移`, `· ${reg.failover.length} failover`) : T('· 无故障转移', '· no failover')}</small>` : html`<small>${T('未选择模型', 'no model chosen')}</small>`, '', 4)}
    </div>
  </div>`;
}

// ---------------------------------------------------------------- sheets

function budgetSheet(b: BudgetView): Html {
  return sheetWrap(
    'pulse',
    T('每日调用预算', 'Daily call budget'),
    T(`每个提供方每天 ${b.per_provider_limit} 次`, `${b.per_provider_limit} per provider per day`),
    html`${b.providers.map(
      (p) =>
        html`<div class="bud ${p.usable ? '' : 'dim'}"><span>${p.name}</span><span class="m"><i style="width:${p.limit ? Math.min(100, (p.used / p.limit) * 100).toFixed(1) : 0}%"></i></span><span class="v num">${
          p.usable ? `${p.used} / ${p.limit}` : (p.reason ?? '—')
        }</span></div>`,
    )}
      <div class="bsum"><span>${T('今日合计', 'Today, total')}</span><b class="num">${b.total_calls} / ${b.total_limit}</b></div>
      <p class="fine">${T(
        `真实上限 = 提供方数 × 单个限额（${b.providers.length} × ${b.per_provider_limit}）。`,
        `The real ceiling is providers × limit (${b.providers.length} × ${b.per_provider_limit}).`,
      )}<br>${T('故障转移只去：', 'Failover only to a provider that is ')}<b>${T('已验证 + 有余额 + 授权范围完全一致', 'verified, has budget and an exactly equal consent scope')}</b>${T(' 的提供方。', '.')}</p>`,
  );
}

function consentSheet(c: ConsentLedger, u: SettingsUi, providerName: (id: string) => string): Html {
  const events = c.events.map((e) =>
    e.granted
      ? html`<div class="evt"><span class="t num">${shortStamp(e.at)}</span><b>${e.reviewer}</b> ${T('授予', 'granted')} <b>${providerName(e.provider)}</b> · ${e.source_id}</div>`
      : html`<div class="evt rv"><span class="t num">${shortStamp(e.at)}</span><b>${e.reviewer}</b> ${T('撤回', 'revoked')} <b>${providerName(e.provider)}</b> · ${T(`${e.source_id} 文本此后扣留`, `${e.source_id} text withheld from now`)}</div>`,
  );
  const pairs = c.current.map(
    (x) => html`<div class="svc"><span class="state-dot ${x.granted ? 'ok' : 'bad'}"></span><span>${x.source_id} → ${providerName(x.provider)}</span><span class="st ${x.granted ? 'ok' : 'bad'}">${
      x.granted ? T('已授予', 'granted') : T('已撤回', 'revoked')
    }${
      x.granted
        ? html` <button class="btn ghost sm" data-act="consent-revoke" data-v="${x.source_id}|${x.provider}" ${u.busy ? 'disabled' : ''}>${T('撤回', 'Revoke')}</button>`
        : html` <button class="btn ghost sm" data-act="consent-ask" data-v="${x.source_id}|${x.provider}">${T('重新授予…', 'Grant…')}</button>`
    }</span></div>`,
  );
  const k = u.confirming;
  const conf = k
    ? html`<div class="confirm"><h3>${T('授予前确认范围', 'Confirm the scope before granting')}</h3>
          <div class="ledger" style="background:var(--glass-3)">
            <div><span class="lk">${T('来源', 'Source')}</span><span class="lv">${k.source_id} · ${T('本地导入的私有资料', 'private local import')}</span></div>
            <div><span class="lk">${T('提供方', 'Provider')}</span><span class="lv">${providerName(k.provider)}</span></div>
            <div><span class="lk">${T('可发送', 'May send')}</span><span class="lv">${c.scope_fields.join(', ')}</span></div>
          </div>
          <div class="sh-acts" style="margin-top:12px"><button class="btn primary sm" data-act="consent-yes" ${u.busy ? 'disabled' : ''}>${T('确认授予', 'Grant')}</button><button class="btn quiet sm" data-act="consent-cancel">${T('取消', 'Cancel')}</button></div></div>`
    : '';
  return sheetWrap(
    'shield',
    T('私有来源的授权', 'Consent · private sources'),
    T('只追加事件，不改历史', 'Appends an event; history stays'),
    html`${pairs.length ? pairs : html`<p class="fine">${T('还没有任何授权记录。', 'No consent recorded yet.')}</p>`}
        ${conf}
        ${events.length ? html`<div class="evts" style="margin-top:14px">${events}</div>` : ''}
        <p class="fine">${T('第一方自有内容（技能统计、已审核战术）不需要授权。', 'First-party content (skill counts, reviewed tactics) needs no consent.')}</p>`,
  );
}

function servicesSheet(s: Services): Html {
  const a = s.anki_connect;
  return sheetWrap(
    'server',
    T('本地服务', 'Local services'),
    T('全部在本机运行', 'All on this machine'),
    html`<div class="svc"><span class="state-dot ${s.node.available ? 'ok' : 'bad'}"></span><span>${T('Agent 宿主', 'Agent host')} <span class="muted">(Node${s.node.version ? ' ' + s.node.version : ''})</span></span><span class="st ${
      s.node.available ? 'ok' : 'bad'
    }">${s.node.available ? T('可用 · 按次启动', 'available · started per call') : (s.node.reason ?? T('不可用', 'unavailable'))}</span></div>
      <div class="svc"><span class="state-dot ${a.reachable === null ? 'off' : a.reachable ? 'ok' : 'bad'}"></span><span>AnkiConnect <code class="muted">${a.endpoint}</code></span><span class="st ${
        a.reachable === null ? '' : a.reachable ? 'ok' : 'bad'
      }">${a.reachable === null ? T('未检查', 'not checked') : a.reachable ? T('可连接', 'reachable') : T('无法连接', 'unreachable')}</span></div>
      <div class="svc"><span class="state-dot ${s.search.configured ? 'ok' : 'off'}"></span><span>${T('搜索提供方', 'Search provider')}</span><span class="st">${
        s.search.configured ? T(`已配置 · 今日 ${s.search.calls_today} 次调用`, `configured · ${s.search.calls_today} calls today`) : T(`未配置 · ${s.search.calls_today} 次调用`, `not configured · ${s.search.calls_today} calls`)
      }</span></div>`,
  );
}

function routesSheet(reg: ModelRegistry, s: Services | null): Html {
  const active = reg.providers.find((p) => p.id === reg.active_id);
  const model = active?.model ? html`<code>${active.model}</code>` : html`<span class="muted">${T('未选择', 'none chosen')}</span>`;
  const name = (id: string) => reg.providers.find((p) => p.id === id)?.name ?? id;
  return sheetWrap(
    'route',
    T('用途分配', 'Which model does what'),
    T('每项用途只走一个模型', 'Each use goes to exactly one model'),
    html`<div class="route-row"><span>${T('导师 · 解释 / 追问 / 微课', 'Coach · explain / probe / micro-lesson')}</span><i></i>${model}</div>
      <div class="route-row"><span>${T('学习简报', 'Study brief')}</span><i></i>${model}</div>
      <div class="route-row"><span>${T('找练习', 'Find practice')} <span class="id">search</span></span><i></i><span class="muted">${
        s ? (s.search.configured ? T('已配置', 'configured') : T('未配置', 'not configured')) : '—'
      }</span></div>
      <div class="route-row"><span>${T('故障转移', 'Failover')}</span><i></i><span class="muted">${
        reg.failover.length ? reg.failover.map(name).join(', ') : T('无 · 没有其他符合条件的提供方', 'none · no other eligible provider')
      }</span></div>`,
  );
}

export function settingsSheet(key: string, d: SettingsData, u: SettingsUi): Html | null {
  const reg = d.models.status === 'ready' ? d.models.value : null;
  const providerName = (id: string) => reg?.providers.find((p) => p.id === id)?.name ?? id;
  switch (key) {
    case 'budget':
      return d.budget.status === 'ready' ? budgetSheet(d.budget.value) : null;
    case 'consent':
      return d.consents.status === 'ready' ? consentSheet(d.consents.value, u, providerName) : null;
    case 'services':
      return d.services.status === 'ready' ? servicesSheet(d.services.value) : null;
    case 'routes':
      return reg ? routesSheet(reg, d.services.status === 'ready' ? d.services.value : null) : null;
  }
  return null;
}
