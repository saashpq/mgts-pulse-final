import type { Severity, Status } from './api';

export const SEVERITY_TEXT: Record<Severity, string> = { high: 'Критично', medium: 'Внимание', low: 'Наблюдать' };
export const STATUS_TEXT: Record<Status, string> = {
  attention: 'Требует вмешательства',
  watch: 'Под наблюдением',
  ok: 'Без срочных сигналов',
};
export const SOURCE_TEXT = { rules: 'Формула', text: 'Текст ответа', hybrid: 'Оценки и текст' } as const;
export const SOURCE_HINT = {
  rules: 'считается по оценкам и порогам, без ИИ',
  text: 'тема из комментария участника, источник разбора указан у цитаты',
  hybrid: 'оценки по порогам, подтверждено разбором текста',
} as const;
export const SEVERITY_RANK: Record<Severity, number> = { high: 0, medium: 1, low: 2 };
export const SEVERITY_MARK: Record<Severity, string> = { high: '!', medium: '•', low: '○' };

/** Тональность словом: число −1..1 HR ничего не говорит. */
export const toneOf = (v: number): 'neg' | 'neu' | 'pos' => (v < -0.2 ? 'neg' : v > 0.2 ? 'pos' : 'neu');
export const TONE_TEXT = { neg: 'негатив', neu: 'нейтрально', pos: 'позитив' } as const;

/** «1 месяц», «2 месяца», «5 месяцев». */
export function plural(n: number, one: string, few: string, many: string) {
  const a = Math.abs(n) % 100;
  const b = a % 10;
  if (a > 10 && a < 20) return many;
  if (b === 1) return one;
  if (b >= 2 && b <= 4) return few;
  return many;
}
export const VERDICT_TEXT = { confirmed: 'Подтверждён HR', false: 'Ложный сигнал', resolved: 'Решён' } as const;
export const ROLE_TEXT = { intern: 'Стажёр', mentor: 'Наставник' } as const;

export const fmt = (v: number | null | undefined, digits = 2) =>
  v === null || v === undefined || Number.isNaN(v) ? '—' : v.toFixed(digits).replace('.', ',');

export const pct = (v: number) => `${Math.round(v * 100)}%`;

/** Оценки за указанный и непосредственно предшествующий месяцы программы. */
export function monthScores(series: (number | null)[], month: number): [number | null, number | null] {
  return [series[month - 1] ?? null, series[month - 2] ?? null];
}

export const months = (n: number) => Array.from({ length: n }, (_, i) => `M${i + 1}`);

export function reactionDeadline(signal: { due_days: number; seen_at?: string }) {
  const start = signal.seen_at ? Date.parse(signal.seen_at) : NaN;
  const days = signal.due_days;
  if (!Number.isFinite(start)) {
    return { at: Infinity, label: `Срок реакции — ${days} ${plural(days, 'день', 'дня', 'дней')} с появления сигнала` };
  }
  const at = start + days * 86_400_000;
  const date = new Date(at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
  return { at, label: `до ${date}` };
}

/**
 * Десятичная запятая в текстах бэкенда (сводка, объяснения сигналов, бриф): «3.13» → «3,13».
 * Не трогает даты и версии (18.11.2026, 1.2.3) и числа внутри слов (qwen2.5).
 */
export const commaNum = (s: string) => s.replace(/(?<![\p{L}\d.,])(\d+)\.(\d+)(?![\d.]*\d)/gu, '$1,$2');

/** Граница интервала в процентах: 0,995 не превращается в «100%», 0,004 — в «0%» (это были бы точные значения). */
const bound = (v: number) => {
  const r = Math.round(v * 100);
  if (v > 0 && v < 1) return Math.min(99, Math.max(1, r));
  return r;
};

/** «80% (95% ДИ 28–99%)»; без границ — просто «80%». */
export function pctCI(value: number | null | undefined, lo?: number | null, hi?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—';
  const base = pct(value);
  if (lo === null || lo === undefined || hi === null || hi === undefined) return base;
  return `${base} (95% ДИ ${bound(lo)}–${bound(hi)}%)`;
}

/** «4/5 = 80% (95% ДИ 28–99%)». */
export const ratioCI = (x: number, n: number, value: number | null | undefined, lo?: number | null, hi?: number | null) =>
  `${x}/${n} = ${pctCI(value ?? (n ? x / n : null), lo, hi)}`;

/** Число со знаком и запятой: «+0,8», «−0,4», «0,0». */
export const signed = (v: number, digits = 1) => {
  const s = Math.abs(v).toFixed(digits).replace('.', ',');
  if (Number(s.replace(',', '.')) === 0) return s;
  return (v > 0 ? '+' : '−') + s;
};

/** Согласие по κ словами (Landis & Koch). */
export function kappaText(k: number | null | undefined) {
  if (k === null || k === undefined || Number.isNaN(k)) return 'не посчитать';
  if (k <= 0.2) return 'слабое';
  if (k <= 0.4) return 'удовлетворительное';
  if (k <= 0.6) return 'умеренное';
  if (k <= 0.8) return 'существенное';
  return 'почти полное';
}

/** Смещение стажёр − наставник простыми словами. */
export function biasText(bias: number) {
  const a = fmt(Math.abs(bias), 2);
  if (Math.abs(bias) < 0.1) return 'в среднем стажёр и наставник оценивают одинаково';
  return bias > 0 ? `стажёр оценивает в среднем на ${a} выше наставника` : `наставник оценивает в среднем на ${a} выше стажёра`;
}

/** Кто выделил цитату: словарь или модель. */
export const analyzerText = (a?: string) => (!a ? '' : a.startsWith('llm') ? `модель${a.includes(':') ? ' ' + a.slice(a.indexOf(':') + 1) : ''}` : 'словарь тем');
