// Keyboard (prototype lines 2121–2165). Esc closes a layer first, then leaves the Anki editor, then
// drops the mark popover. Nothing fires inside inputs or with Ctrl/Meta/Alt. `?` opens the shortcut
// sheet, Shift+T / Shift+L switch theme and language, and G + letter (within 1200 ms) jumps between
// rooms. While a layer is open, only Esc and the global keys work.
import { ui, type Route } from './state';

export interface KeyActions {
  closeLayer(): void;
  exitAnkiEdit(): void;
  hideMarkPop(): void;
  keys(): void;
  theme(): void;
  lang(): void;
  go(route: Route): void;
  todayStart(): void;
  practicePick(key: string): boolean;
  practiceSubmit(): void;
  practiceHint(): void;
  practiceSkip(): void;
  /** Turns the pending passage selection into a mark; false when there is none. */
  practiceMark(): boolean;
  coachMode(index: number): void;
  feedbackNext(): void;
  feedbackNote(): void;
  feedbackCycle(step: 1 | -1): void;
  assessPick(key: string): boolean;
  assessConfirm(): void;
  assessSkip(): void;
  ankiEdit(): void;
}

const CHORDS: Record<string, Route> = { T: 'today', P: 'practice', A: 'assess', R: 'report', K: 'anki', S: 'settings', F: 'feedback' };

export function installKeyboard(a: KeyActions): void {
  let gChord = 0;
  document.addEventListener('keydown', (e) => {
    const target = e.target instanceof HTMLElement ? e.target : null;
    const tag = (target?.tagName ?? '').toLowerCase();
    if (e.key === 'Escape') {
      if (ui.layer) {
        a.closeLayer();
        e.preventDefault();
      } else if (ui.route === 'anki' && ui.anki.editing) {
        a.exitAnkiEdit();
        e.preventDefault();
      }
      a.hideMarkPop();
      return;
    }
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable || e.ctrlKey || e.metaKey || e.altKey) return;
    const k = e.key;
    const up = k.toUpperCase();
    if (k === '?') {
      a.keys();
      e.preventDefault();
      return;
    }
    if (e.shiftKey && up === 'T') return a.theme();
    if (e.shiftKey && up === 'L') return a.lang();
    if (ui.layer) return;
    if (gChord && Date.now() - gChord < 1200) {
      gChord = 0;
      const r = CHORDS[up];
      if (r) {
        a.go(r);
        e.preventDefault();
      }
      return;
    }
    if (up === 'G' && !e.shiftKey) {
      gChord = Date.now();
      return;
    }
    const onControl = tag === 'a' || tag === 'button';
    // An answer option is a button, but Enter on it means submit / confirm (the key cap says so),
    // never the browser's own activation, which would only pick the same option again.
    const onOption = !!target?.closest('.opt');
    const submitKey = k === 'Enter' && (!onControl || onOption);
    if (submitKey && onOption) e.preventDefault();
    switch (ui.route) {
      case 'today':
        if (k === 'Enter' && !onControl) a.todayStart();
        break;
      case 'practice':
        if (up === 'M' && a.practiceMark()) break;
        if (k.length === 1 && a.practicePick(k)) break;
        if (submitKey) a.practiceSubmit();
        else if (up === 'H') a.practiceHint();
        else if (up === 'S') a.practiceSkip();
        break;
      case 'feedback':
        if ('123'.includes(k) && k.length === 1) a.coachMode(Number(k) - 1);
        else if (up === 'N') a.feedbackNext();
        else if (up === 'K') a.feedbackNote();
        else if (k === 'ArrowDown' || k === 'ArrowUp') {
          a.feedbackCycle(k === 'ArrowDown' ? 1 : -1);
          e.preventDefault();
        }
        break;
      case 'assess':
        if (k.length === 1 && a.assessPick(k)) break;
        if (submitKey) a.assessConfirm();
        else if (up === 'S') a.assessSkip();
        break;
      case 'anki':
        if (up === 'E') a.ankiEdit();
        break;
    }
  });
}
