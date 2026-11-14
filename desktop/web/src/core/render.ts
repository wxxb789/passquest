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

export function renderLayer(anim: boolean): void {
  const layer = $('#layer');
  if (!screens || !layer) return;
  layer.classList.toggle('enter-layer', anim);
  layer.innerHTML = screens.layer().value;
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
