// Rendering mechanics (prototype render / renderLayer / openLayer / closeLayer / focusKey). The
// screens themselves are supplied by the app (src/screens.ts) so this module stays view-agnostic.
// Modal layers make the app, dock and orb `inert`; closing a layer restores the focus it took.
import { tickClocks } from './clock';
import type { Html } from './html';
import { ui, type Layer } from './state';

export interface Screens {
  top(): Html;
  stage(): Html;
  dock(): Html;
  orb(): Html;
  layer(): Html;
  room(): string;
  title(): string;
  withCoach(): boolean;
  editing(): boolean;
  /** After a full stage render: e.g. centre the lit evidence on Feedback. */
  afterStage(entered: boolean): void;
}

let screens: Screens | null = null;

export function setScreens(s: Screens): void {
  screens = s;
}

const $ = <E extends Element = HTMLElement>(sel: string) => document.querySelector<E>(sel);

/** Modal = every layer except the coach popover. */
export const isModal = (l: Layer | null): boolean => !!l && l !== 'pop';

/** A selector that finds the same control after a re-render. */
export function focusKey(el: Element | null): string | null {
  if (!(el instanceof HTMLElement) || el === document.body) return null;
  if (el.dataset.act) return `[data-act="${el.dataset.act}"]` + (el.dataset.v !== undefined ? `[data-v="${CSS.escape(el.dataset.v)}"]` : '');
  if (el.dataset.field) return `[data-field="${CSS.escape(el.dataset.field)}"]`;
  if (el.id) return '#' + CSS.escape(el.id);
  return null;
}

export function moreCue(): void {
  const sc = $('#coscroll');
  const more = $('#more');
  if (!sc || !more) return;
  const atEnd = sc.scrollTop + sc.clientHeight >= sc.scrollHeight - 4;
  more.hidden = atEnd;
  sc.classList.toggle('end', atEnd);
}

export function render(full = true, anim = false): void {
  if (!screens) return;
  const root = document.documentElement;
  const stage = $('#stage');
  const top = $('#top');
  const dock = $('#dock');
  const orb = $('#orb');
  if (!stage || !top || !dock || !orb) return;
  const fk = focusKey(document.activeElement);
  const passage = $('#passage');
  const keepScroll = !anim && passage ? passage.scrollTop : null;
  root.dataset.theme = ui.theme;
  root.lang = ui.lang === 'zh' ? 'zh-CN' : 'en';
  root.classList.toggle('shot', ui.shot);
  document.body.dataset.room = screens.room();
  document.body.classList.toggle('editing', screens.editing());
  document.title = screens.title();
  top.innerHTML = screens.top().value;
  stage.classList.toggle('enter', anim);
  stage.classList.toggle('with-coach', screens.withCoach());
  if (full) stage.innerHTML = screens.stage().value;
  dock.innerHTML = screens.dock().value;
  orb.innerHTML = screens.orb().value;
  renderLayer(false);
  const fresh = $('#passage');
  if (full) screens.afterStage(anim || keepScroll === null);
  if (keepScroll !== null && fresh && !anim) fresh.scrollTop = keepScroll;
  tickClocks();
  moreCue();
  if (fk && !ui.layer) $(fk)?.focus({ preventScroll: true });
}

export function renderOrb(): void {
  const orb = $('#orb');
  if (!screens || !orb) return;
  orb.innerHTML = screens.orb().value;
  moreCue();
}

/** A control inside the layer that can be found again after a rebuild: a form field by name, else focusKey. */
function layerKey(el: Element | null): string | null {
  if (el instanceof HTMLInputElement || el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement) {
    if (el.form?.id && el.name) return `#${CSS.escape(el.form.id)} [name="${CSS.escape(el.name)}"]`;
  }
  return focusKey(el);
}

type Field = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
const fieldsOf = (root: ParentNode): Field[] => Array.from(root.querySelectorAll<Field>('form[id] input[name], form[id] select[name], form[id] textarea[name]'));

/**
 * Rebuilding the layer that is already shown (a background load, a language switch) keeps what the
 * learner typed or chose in its forms, the caret, and the focus; only the surrounding markup is new.
 * Opening a different layer starts clean.
 */
function rebuildLayer(layer: HTMLElement, markup: string): void {
  const same = !!ui.layer && layer.dataset.layer === ui.layer;
  const active = same && layer.contains(document.activeElement) ? document.activeElement : null;
  const focus = layerKey(active);
  const caret = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement ? [active.selectionStart, active.selectionEnd] : null;
  const kept = same ? fieldsOf(layer).map((f) => ({ key: layerKey(f), value: f.value, checked: f instanceof HTMLInputElement ? f.checked : false })) : [];
  layer.innerHTML = markup;
  layer.dataset.layer = ui.layer ?? '';
  for (const k of kept) {
    const f = k.key ? layer.querySelector<Field>(k.key) : null;
    if (!f) continue;
    if (f instanceof HTMLInputElement && (f.type === 'checkbox' || f.type === 'radio')) f.checked = k.checked;
    else f.value = k.value;
  }
  if (!focus) return;
  const el = layer.querySelector<HTMLElement>(focus);
  el?.focus({ preventScroll: true });
  if (caret && (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) {
    try {
      el.setSelectionRange(caret[0], caret[1]);
    } catch {
      // input types without a caret (number, date) refuse a selection range
    }
  }
}

export function renderLayer(anim: boolean): void {
  const layer = $('#layer');
  if (!screens || !layer) return;
  layer.classList.toggle('enter-layer', anim);
  rebuildLayer(layer, screens.layer().value);
  const modal = isModal(ui.layer);
  for (const sel of ['.app', '#dock', '#orb']) {
    const el = $(sel);
    if (el) el.inert = modal;
  }
  if (ui.layer && anim) {
    const af = layer.querySelector<HTMLElement>('[autofocus]') ?? layer.querySelector<HTMLElement>('.sheet .icon-btn, .dlg button, .csheet button');
    af?.focus({ preventScroll: true });
  }
}

export function openLayer(name: Layer): void {
  if (!ui.layer) ui.ret = focusKey(document.activeElement);
  ui.layer = name;
  renderLayer(true);
  if (name === 'pop') renderOrb();
}

export function closeLayer(): void {
  const was = ui.layer;
  ui.layer = null;
  renderLayer(false);
  if (was === 'pop') renderOrb();
  if (ui.ret) {
    $(ui.ret)?.focus({ preventScroll: true });
    ui.ret = null;
  }
}
