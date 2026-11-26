import { useSyncExternalStore } from 'react';

/** Маршрутизация на History API: у каждого экрана свой адрес, работают «Назад» и обновление страницы. */
export function navigate(path: string) {
  const target = new URL(path, document.baseURI);
  if (window.location.href !== target.href) window.history.pushState(null, '', target.href);
  window.dispatchEvent(new PopStateEvent('popstate'));
  if (!target.hash) document.querySelector('.workspace')?.scrollTo?.({ top: 0 });
}

const subscribe = (cb: () => void) => {
  window.addEventListener('popstate', cb);
  return () => window.removeEventListener('popstate', cb);
};

export const usePath = () => useSyncExternalStore(subscribe, () => window.location.pathname);

export function pairPath(id: string, section?: 'brief'): string {
  return `/pairs/${encodeURIComponent(id)}${section === 'brief' ? '/brief' : ''}`;
}

export function pairMonthPath(id: string, month: number): string {
  return `${pairPath(id)}/months/${month}`;
}

export function pairDecisionPath(pair: string, signalId?: string): string {
  const target = signalId ? 'decision-' + encodeURIComponent(signalId) : 'sig-title';
  return pairPath(pair) + '#' + target;
}

export type Route =
  | { view: 'overview' | 'signals' | 'sources' | 'thresholds' | 'quality' | 'report' }
  | { view: 'pair'; id: string; month?: number }
  | { view: 'brief'; id: string }
  | { view: 'form'; token: string }
  | { view: 'not-found' };

export function match(path: string): Route {
  const p = path.replace(/\/+$/, '') || '/';
  if (p === '/' || p === '/overview') return { view: 'overview' };
  for (const v of ['signals', 'sources', 'thresholds', 'quality', 'report'] as const) if (p === `/${v}`) return { view: v };
  try {
    const month = p.match(/^\/pairs\/([^/]+)\/months\/([1-9]\d*)$/);
    if (month && Number.isSafeInteger(Number(month[2]))) return { view: 'pair', id: decodeURIComponent(month[1]), month: Number(month[2]) };
    const pair = p.match(/^\/pairs\/([^/]+)(\/brief)?$/);
    if (pair) return { view: pair[2] ? 'brief' : 'pair', id: decodeURIComponent(pair[1]) };
    const form = p.match(/^\/form\/([^/]+)$/);
    if (form) return { view: 'form', token: decodeURIComponent(form[1]) };
  } catch (error) {
    if (!(error instanceof URIError)) throw error;
  }
  return { view: 'not-found' };
}
