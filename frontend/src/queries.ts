import type { QueryClient } from '@tanstack/react-query';

/** Корневые ключи запросов. Инвалидация точечная: каждое действие перечитывает только то, что могло измениться. */
export type QueryRoot = 'overview' | 'signals' | 'pair' | 'brief' | 'sources' | 'thresholds';

/** Что меняется от новых ответов и решений HR — всё, кроме порогов. */
export const DATA_KEYS: QueryRoot[] = ['overview', 'signals', 'pair', 'brief', 'sources'];
/** Решение HR по сигналу. */
export const VERDICT_KEYS: QueryRoot[] = ['overview', 'signals', 'pair', 'brief'];
/** Сохранение порогов: пересчитываются сигналы и сами пороги. */
export const THRESHOLD_KEYS: QueryRoot[] = ['thresholds', 'overview', 'signals', 'pair', 'brief'];
export const ALL_KEYS: QueryRoot[] = [...DATA_KEYS, 'thresholds'];

export function invalidate(qc: QueryClient, roots: QueryRoot[]) {
  const set = new Set<unknown>(roots);
  return qc.invalidateQueries({ predicate: (q) => set.has(q.queryKey[0]) });
}

/** 30 с: переходы между экранами не дёргают сервер, свежесть обеспечивает SSE и инвалидация после действий. */
export const STALE_TIME = 30_000;
