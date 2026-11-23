// Rendering mechanics (prototype render / renderLayer / openLayer / closeLayer / focusKey). The
// screens themselves are supplied by the app (src/screens.ts) so this module stays view-agnostic.
// A modal layer is a native <dialog>; a rebuild replaces the element, which breaks the engine's own
// focus restore, so the app still records the opener (ui.ret).
import { resumeMarkPop } from '../components/passage';
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
  stage.classList.remove('enter-hint');
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
  landFocus();
}

const lost = () => !document.activeElement || document.activeElement === document.body || document.activeElement === document.documentElement;

/**
 * After a re-render that removed the focused control (a route change, a new item), focus lands on
 * the stage itself (`#stage`, tabindex -1) instead of falling back to <body>: the next Tab starts in
 * the new view and the room's keys keep working (the stage is not a control, so Enter is not a click).
 */
export function landFocus(): void {
  if (ui.layer || !lost()) return;
  $('#stage')?.focus({ preventScroll: true });
}

/**
 * Re-renders the orb (the coach sheet). Focus that was inside it stays inside: on the same control
 * when it is still there and enabled (a send button is disabled while its call runs), else on the
 * coach sheet itself, never on <body>.
 */
export function renderOrb(): void {
  const orb = $('#orb');
  if (!screens || !orb) return;
  const inside = orb.contains(document.activeElement);
  const fk = inside ? focusKey(document.activeElement) : null;
  orb.innerHTML = screens.orb().value;
  moreCue();
  if (!inside || orb.contains(document.activeElement)) return;
  const same = fk ? orb.querySelector<HTMLElement>(fk) : null;
  const next = same && !(same as HTMLButtonElement).disabled ? same : (orb.querySelector<HTMLElement>('#coach') ?? orb.querySelector<HTMLElement>('button'));
  next?.focus({ preventScroll: true });
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

const layerDialog = (): HTMLDialogElement | null => $('#layer')?.querySelector<HTMLDialogElement>('dialog[open]') ?? null;

/** Shows the layer's dialog with showModal(): the top layer blocks the page and keeps Tab inside. */
function showDialog(layer: HTMLElement): void {
  const dialog = layer.querySelector<HTMLDialogElement>('dialog');
  if (!dialog || dialog.open) return;
  dialog.addEventListener('click', (e) => {
    // An engine's backdrop click reaches the dialog as its target (padding does too, so the point
    // decides); a real pointer click carries a click count, a scripted or keyboard click carries 0.
    if (e.target !== dialog || e.detail <= 0) return;
    const r = dialog.getBoundingClientRect();
    if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) closeLayer();
  });
  dialog.addEventListener('close', () => {
    // The engine's own close (a real Escape reaches cancel) ends the layer; a rebuild's close of the
    // replaced dialog must not.
    if (!dialog.isConnected || !ui.layer) return;
    closeLayer();
  });
  dialog.showModal();
}

/**
 * Rebuilding the layer that is already shown (a background load, a language switch) keeps what the
 * learner typed or chose in its forms, the caret, and the focus; only the surrounding markup is new.
 * Opening a different layer starts clean; a modal closes and shows its dialog around the swap, since
 * an open <dialog> would keep the page blocked and the new element needs the focusing steps.
 */
function rebuildLayer(layer: HTMLElement, markup: string): void {
  const same = !!ui.layer && layer.dataset.layer === ui.layer;
  const shown = same ? layerDialog() : null;
  const active = same && layer.contains(document.activeElement) ? document.activeElement : null;
  const focus = layerKey(active);
  const caret = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement ? [active.selectionStart, active.selectionEnd] : null;
  const kept = same ? fieldsOf(layer).map((f) => ({ key: layerKey(f), value: f.value, checked: f instanceof HTMLInputElement ? f.checked : false })) : [];
  shown?.close();
  layer.innerHTML = markup;
  layer.dataset.layer = ui.layer ?? '';
  if (isModal(ui.layer)) showDialog(layer);
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
  // The coach pop is no <dialog>: the engine runs no focusing steps, so its focus entry is the app's.
  if (ui.layer && anim && !isModal(ui.layer)) {
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
  layerDialog()?.close();
  renderLayer(false);
  if (was === 'pop') renderOrb();
  // A modal's showModal() hides the mark popover; closing the layer re-shows it while the mark stands.
  resumeMarkPop();
  if (ui.ret) {
    $(ui.ret)?.focus({ preventScroll: true });
    ui.ret = null;
  }
  landFocus();
}
