// Theme and language persistence. Only these two preferences live in the webview's localStorage;
// study state lives in the Go core. URL parameters (?theme, ?lang, ?shot) win, as in the prototype.

export type Theme = 'light' | 'dark';
export type Lang = 'zh' | 'en';

const KEY = 'pq-stage-';

function read(name: 'theme' | 'lang'): string | null {
  try {
    return localStorage.getItem(KEY + name);
  } catch {
    return null;
  }
}

export interface Prefs {
  theme: Theme;
  lang: Lang;
  shot: boolean;
}

export function loadPrefs(): Prefs {
  const q = new URLSearchParams(location.search);
  const theme = q.get('theme') ?? read('theme');
  const lang = q.get('lang') ?? read('lang');
  return {
    theme: theme === 'dark' ? 'dark' : 'light',
    lang: lang === 'en' ? 'en' : 'zh',
    shot: q.get('shot') === '1',
  };
}

export function savePref(name: 'theme' | 'lang', value: string): void {
  try {
    localStorage.setItem(KEY + name, value);
  } catch {
    // storage unavailable: the preference lasts for this window only
  }
  const u = new URL(location.href);
  if (u.searchParams.has(name)) {
    u.searchParams.set(name, value);
    history.replaceState(null, '', u.pathname + u.search + u.hash);
  }
}
