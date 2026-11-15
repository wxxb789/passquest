// Hash routes (prototype route()): #/<route>[/<sub>]. Sub-routes:
//   practice/hint      open the hint gate (the prototype's practice/assisted forced the state;
//                      here assistance is only ever recorded by the core, so it opens the gate)
//   anki/edit          open the field editor
//   feedback/nocoach   start with the coach sheet collapsed
//   report/annN        open the annotation sheet on item N
//   anything else      open the bottom sheet of that name, e.g. #/settings/consent
import { ROUTES, type Route } from './state';

export interface Location {
  route: Route;
  sub: string | null;
}

export function parseHash(hash: string): Location {
  const m = /^#\/(\w+)(?:\/([\w-]+))?/.exec(hash);
  const r = m?.[1];
  const route = (ROUTES as readonly string[]).includes(r ?? '') ? (r as Route) : 'today';
  return { route, sub: m?.[2] ?? null };
}

export type SubAction =
  | { kind: 'hint' }
  | { kind: 'edit' }
  | { kind: 'nocoach' }
  | { kind: 'ann'; item: number | null }
  | { kind: 'sheet'; name: string };

export function subAction(loc: Location): SubAction | null {
  const sub = loc.sub;
  if (!sub) return null;
  if (loc.route === 'practice' && (sub === 'hint' || sub === 'assisted')) return { kind: 'hint' };
  if (loc.route === 'anki' && sub === 'edit') return { kind: 'edit' };
  if (loc.route === 'feedback' && sub === 'nocoach') return { kind: 'nocoach' };
  if (sub.startsWith('ann')) return { kind: 'ann', item: Number(sub.slice(3)) || null };
  return { kind: 'sheet', name: sub };
}

export function go(route: Route, sub?: string): void {
  const next = '#/' + route + (sub ? '/' + sub : '');
  if (location.hash === next) window.dispatchEvent(new HashChangeEvent('hashchange'));
  else location.hash = next;
}

export function installRouter(onRoute: (loc: Location) => void): void {
  window.addEventListener('hashchange', () => onRoute(parseHash(location.hash)));
}
