/** Клиент API «Пульса стажировки». Типы повторяют JSON бэкенда (internal/signals, internal/api). */

export type Severity = 'high' | 'medium' | 'low';
export type Status = 'attention' | 'watch' | 'ok';
export type Num = number | null;

export interface Evidence {
  response_id: string;
  role: 'intern' | 'mentor';
  period: number;
  question?: string;
  text?: string;
  value?: number;
  quote?: string;
  /** кто выделил цитату: 'offline' (словарь) или 'llm:<модель>' (R4, может отсутствовать) */
  analyzer?: string;
}

export interface Verdict {
  status: 'confirmed' | 'false' | 'resolved';
  comment?: string;
  at: string;
  /** что сделано по сигналу: «встреча 1:1 проведена», «сменили задачи»… (R4) */
  action?: string;
  /** месяц решения (R4) */
  period?: number;
}

export interface Signal {
  id: string;
  pair: string;
  type: string;
  title: string;
  severity: Severity;
  source: 'rules' | 'text' | 'hybrid';
  first_period: number;
  last_period: number;
  explanation: string;
  action: string;
  due_days: number;
  evidence: Evidence[];
  first_seen: number;
  verdict?: Verdict;
  /** главный сигнал пары: остальные активные сигналы её подтверждают */
  primary?: boolean;
  /** id подтверждающих сигналов (только у главного) */
  related?: string[];
  /** когда сигнал впервые увидели (для просрочки) */
  seen_at?: string;
  /** срок реакции прошёл, решения HR нет */
  overdue?: boolean;
}

export interface PairReport {
  response_ids?: Record<'intern' | 'mentor', string[]>;
  month_reasons?: string[];
  pair: string;
  intern: string;
  mentor: string;
  track?: string;
  status: Status;
  headline: string;
  signals: Signal[];
  resolved: Signal[];
  history: (Severity | '')[];
  intern_index: Num[];
  mentor_index: Num[];
  agreement_intern?: Num[];
  agreement_mentor?: Num[];
  agreement_questions?: string[];
  detection: { ours: number; naive: number };
  summary: string;
  /** согласие сторон по паре: смещение стажёр − наставник (R4) */
  agreement?: Agreement | null;
}

/** Bland–Altman: смещение (стажёр − наставник) и границы, в которые попадают 95% разниц. */
export interface Agreement {
  bias: number;
  sd: number;
  loa_lo: number;
  loa_hi: number;
  /** число пар-месяцев с оценками обеих сторон */
  n: number;
  method?: string;
}

/** Доля с 95% доверительным интервалом (Клоппер–Пирсон). */
export interface Rate {
  value: number | null;
  lo?: number | null;
  hi?: number | null;
}

/** Оценка системы на эпизодах с известным началом (синтетика или подтверждённые HR сигналы). */
export interface Evaluation {
  episodes: number;
  found: number;
  recall: Rate;
  alerts: number;
  true_alerts: number;
  precision: Rate;
  /** медиана (первое срабатывание − начало проблемы), месяцы */
  median_delay: number | null;
  /** правило «средняя < 3» */
  naive?: { found: number; median_delay: number | null } | null;
  earlier?: number;
  same?: number;
  later?: number;
  only_ours?: number;
  only_naive?: number;
  /** стабильные пары, где всё же была тревога */
  false_pairs?: string[] | number | null;
}

export interface ThemeKappa {
  theme: string;
  kappa: number | null;
  both: number;
  only_offline: number;
  only_llm: number;
}

export interface Quality {
  ground_truth?: 'synthetic' | 'hr_verdicts';
  evaluation?: Evaluation | null;
  alert_load?: { active_cards: number; per_10_pairs: number; attention_share: number; flapping?: string[] | null } | null;
  scales?: { noise_sigma: number | null; ceiling?: { question: string; text?: string; role?: string; share: number; n?: number }[] | null; ceiling_threshold?: number; flat_share?: number | null } | null;
  agreement?: Agreement | null;
  icc_note?: string;
  text_ai?: {
    analyzer?: string;
    model?: string;
    prompt_hash?: string;
    total: number;
    rejected: number;
    reasons?: Record<string, number> | null;
    agreement?: ThemeKappa[] | null;
  } | null;
  goldset?: ({ date?: string; model?: string; macro_f1?: number | null } & Record<string, unknown>) | null;
  action_effect?: { with_action: number; measured: number; median_delta: number | null; note?: string; caveat?: string } | null;
  caveat?: string;
}

/** Строка журнала «действие → эффект». */
export interface ActionRow {
  signal: string;
  pair: string;
  type: string;
  title: string;
  status: Verdict['status'];
  action?: string;
  period: number;
  role?: 'intern' | 'mentor' | string;
  before: number | null;
  after: number | null;
  delta: number | null;
  months: number;
}

export interface ProgramReport {
  reaction?: { total: number; measured: number; pending: number; unknown: number; median_hours: number | null };
  low_score?: number;
  current: number;
  periods: number;
  pairs: PairReport[];
  attention: number;
  watch: number;
  intern_avg: Num[];
  mentor_avg: Num[];
  fill_rate: number;
  signals_now: number;
  confirmed: number;
  false_positive: number;
  earlier_or_only: number;
  avg_lead_time?: number;
  digest: string;
  /** экран «Качество системы» (R4) */
  quality?: Quality | null;
  /** журнал действий HR и изменение индекса после них (R4) */
  actions?: ActionRow[] | null;
}

export interface AnalyzerInfo {
  mode: 'offline' | 'llm';
  label: string;
  model: string;
}

export interface Analysis {
  hr_question?: string;
  source?: { response_id: string; question: string; period: number; role: string };
  sentiment: number;
  themes: string[];
  quote: string;
  analyzer: string;
  note?: string;
  /** что скрыто перед отправкой в модель: «телефон», «e-mail», «паспорт», «СНИЛС», «ФИО» */
  masked?: string[];
}

export interface Comment {
  response_id: string;
  role: 'intern' | 'mentor';
  period: number;
  question: string;
  text: string;
  analysis: Analysis;
}

export interface QuestionSeries {
  deltas?: Num[];
  code: string;
  role: 'intern' | 'mentor';
  text: string;
  dimension: string;
  mirror?: string;
  values: Num[];
}

export interface PairDetail {
  responses?: { id: string; role: 'intern' | 'mentor'; period: number }[];
  response_versions?: Record<string, unknown[]>;
  low_score?: number;
  report: PairReport;
  current: number;
  periods: number;
  questions: QuestionSeries[];
  comments: Comment[];
  themes: Record<string, { label: string; negative: boolean }>;
}

export interface FlatSignal extends Signal {
  intern: string;
  mentor: string;
  track?: string;
  active: boolean;
}

/** Сгруппированный вид: главный сигнал пары и подтверждающие. */
export interface GroupedSignal extends FlatSignal {
  related_signals?: FlatSignal[];
}

export type SignalsView = 'grouped' | 'all';

export interface Question {
  code: string;
  role: 'intern' | 'mentor';
  text: string;
  kind: 'scale' | 'text';
  max?: number;
}

export interface FormInfo {
  closed?: boolean;
  title: string;
  role: 'intern' | 'mentor';
  person: string;
  questions: Question[];
  period: number;
  periods: number;
}

export interface Sources {
  form_window?: { period: number; closed: boolean };
  periods?: number;
  links: { pair: string; person: string; role: string; url: string; token: string }[];
  events: { at: string; source: string; text: string }[];
  counts: Record<string, number>;
  analyzer: AnalyzerInfo;
  webhook: { path: string; secret_set: boolean };
  demo: boolean;
}

export interface ImportReport {
  report: {
    sheets: { name: string; role: string; mapped: string[] | null; unmapped: string[] | null; accepted: number; errors?: string[] }[];
    accepted: number;
    rejected: number;
    new_pairs?: string[];
    start_month?: string;
  };
  changed: number;
  unchanged: number;
  /** сколько названий колонок сервер запомнил (remember=true) */
  remembered?: number;
}

/** Допустимое значение сопоставления колонки: код вопроса, служебная колонка или 'skip'. */
export interface ImportTarget {
  value: string;
  label: string;
  role: 'intern' | 'mentor' | 'any';
}

export interface PreviewColumn {
  index: number;
  title: string;
  /** предложение сервера; null — не распознано */
  target: string | null;
  samples: string[];
}

export interface PreviewSheet {
  name: string;
  role: 'intern' | 'mentor' | '';
  rows: number;
  columns: PreviewColumn[];
}

export interface ImportPreview {
  sheets: PreviewSheet[];
  targets: ImportTarget[];
}

/** Ручное сопоставление: лист → роль и колонки (index → target). */
export type ImportMapping = Record<string, { role: 'intern' | 'mentor'; columns: Record<string, string> }>;

export interface LlmCheck {
  mode: 'offline' | 'llm';
  model: string;
  ok: boolean;
  latency_ms: number | null;
  sample: string;
  analysis: Analysis | null;
  error: string | null;
}

export type Thresholds = Record<string, number>;

/** Точность типа сигнала по решениям HR: (подтверждено + решено) / все решения. */
export interface SignalPrecision {
  type: string;
  title: string;
  confirmed: number;
  false: number;
  resolved: number;
  precision: number | null;
  /** всего решений HR по типу */
  decisions?: number;
  /** решений достаточно (≥ MIN_DECISIONS), чтобы показывать точность */
  enough?: boolean;
  hint?: string;
  suggest?: { field: string; from: number; to: number };
  /** 95% ДИ Клоппера–Пирсона (R4) */
  lo?: number | null;
  hi?: number | null;
}

/** Тема комментариев по месяцам: сколько комментариев с темой и в каких парах. */
export interface ThemeTrend {
  code: string;
  label: string;
  negative: boolean;
  by_period: number[];
  total: number;
  pairs: string[];
}

export interface PrecisionTotal {
  decisions: number;
  precision: number | null;
  enough: boolean;
  lo?: number | null;
  hi?: number | null;
}

/** Эффект пилота: раньше ли система видит проблему и как HR отрабатывает сигналы. */
export interface Effect {
  pairs_flagged_early: number;
  detection_table: { pair: string; ours: number | null; naive: number | null }[];
  verdict_coverage: number | null;
  overdue: number | null;
  overdue_unknown: number;
  active_signals: number;
  decided_signals: number;
}

/** С какого числа решений HR точность показывается (как MinDecisions на бэкенде). */
export const MIN_DECISIONS = 5;

export interface Overview {
  text_review?: {
    total: number;
    unclassified: number;
    rows: { pair: string; period: number; role: 'intern' | 'mentor'; count: number }[];
    critical?: { pair: string; period: number; role: 'intern' | 'mentor'; question: string }[];
  };
  data_origin?: 'synthetic' | 'mixed' | 'submitted' | 'empty';
  report: ProgramReport & { precision: SignalPrecision[]; precision_total?: PrecisionTotal; effect?: Effect };
  analyzer: AnalyzerInfo;
  themes_over_time: ThemeTrend[];
}

export interface BriefSignal {
  id?: string;
  side?: 'intern' | 'mentor' | 'both';
  type: string;
  title: string;
  severity: Severity;
  explanation: string;
  action: string;
  due_days: number;
  seen_at?: string;
  quotes: string[];
}

/** Часть брифа для разговора с одной стороной: её сигналы (id) и тезисы. */
export interface BriefAudience {
  with: 'intern' | 'mentor';
  signals: string[];
  talking_points: string[];
}

/** Бриф к встрече 1:1 по паре. */
export interface Brief {
  pair: string;
  intern: string;
  mentor: string;
  status: Status;
  generated_at: string;
  signals: BriefSignal[];
  questions: { with: 'intern' | 'mentor'; text: string }[];
  talking_points: string[];
  audiences?: { intern: BriefAudience; mentor: BriefAudience };
  disclaimer: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public offline = false,
  ) {
    super(message);
  }
}

/** Адрес API: в dev и docker — тот же origin (прокси), для отдельного хостинга — VITE_API_URL. */
export const API_BASE = (import.meta.env.VITE_API_URL as string | undefined)?.replace(/\/$/, '') ?? '';

export const OFFLINE_MESSAGE = 'Сервер недоступен, повторяем…';
export const STATIC_WRITE_MESSAGE = 'В онлайн-демо только просмотр. Полная версия запускается локально — см. README';

/** Ошибка связи: сеть не ответила (статус 0) или прокси/сервер вернул 5xx без JSON — обычно бэкенд перезапускается. */
export const isOffline = (e: unknown) => e instanceof ApiError && e.offline;

/** Статическое онлайн-демо (Netlify): GET-ответы — из /demo/*.json, запись запрещена, SSE выключен. */
export const isStaticDemo = () => import.meta.env.VITE_STATIC_DEMO === 'true';

// ---------- Токен HR ----------
const TOKEN_KEY = 'pulse.hrToken';
let authRequired = false;
const authListeners = new Set<() => void>();
const emitAuth = () => authListeners.forEach((f) => f());

export const auth = {
  token: (): string => {
    try {
      return sessionStorage.getItem(TOKEN_KEY) ?? '';
    } catch {
      return '';
    }
  },
  setToken(t: string) {
    try {
      if (t) sessionStorage.setItem(TOKEN_KEY, t);
      else sessionStorage.removeItem(TOKEN_KEY);
    } catch {
      /* sessionStorage недоступен — токен не сохранится между вкладками */
    }
    authRequired = false;
    emitAuth();
  },
  /** сервер ответил 401: нужен (другой) токен HR */
  required: () => authRequired,
  markRequired() {
    if (!authRequired) {
      authRequired = true;
      emitAuth();
    }
  },
  subscribe(f: () => void) {
    authListeners.add(f);
    return () => authListeners.delete(f);
  },
};

/** Адрес потока событий: EventSource не умеет заголовки, токен — параметром. */
export const eventsUrl = () => {
  const t = auth.token();
  return API_BASE + '/api/v1/events' + (t ? `?token=${encodeURIComponent(t)}` : '');
};

/** Имя JSON-снимка для статического демо; фильтры state/severity применяются на клиенте. */
export function staticFile(path: string): { file: string; filter?: (x: FlatSignal) => boolean } | null {
  const u = new URL(path, 'http://x');
  const p = u.pathname.replace(/^\/api\/v1/, '');
  if (p === '/overview') return { file: 'overview.json' };
  if (p === '/thresholds') return { file: 'thresholds.json' };
  if (p === '/sources') return { file: 'sources.json' };
  if (p === '/signals') {
    const view = u.searchParams.get('view') === 'all' ? 'all' : 'grouped';
    const state = u.searchParams.get('state') ?? '';
    const sev = u.searchParams.get('severity') ?? '';
    return {
      file: `signals-${view}.json`,
      filter: (x) => (!state || (state === 'active') === x.active) && (!sev || x.severity === sev),
    };
  }
  let m = p.match(/^\/pairs\/([^/]+)\/brief$/);
  if (m) return { file: `brief-${m[1]}.json` };
  m = p.match(/^\/pairs\/([^/]+)$/);
  if (m) return { file: `pair-${m[1]}.json` };
  return null;
}

async function staticRequest<T>(path: string, init?: RequestInit): Promise<T> {
  if ((init?.method ?? 'GET').toUpperCase() !== 'GET') throw new ApiError(403, STATIC_WRITE_MESSAGE);
  const f = staticFile(path);
  if (!f) throw new ApiError(404, 'В онлайн-демо этот раздел недоступен');
  let res: Response;
  try {
    res = await fetch(`${import.meta.env.BASE_URL ?? '/'}demo/${f.file}`);
  } catch {
    throw new ApiError(0, OFFLINE_MESSAGE, true);
  }
  if (!res.ok) throw new ApiError(res.status, res.status === 404 ? 'Такой страницы в онлайн-демо нет' : `Ошибка ${res.status}`);
  const body = await res.json();
  if (f.filter && Array.isArray(body?.signals)) body.signals = body.signals.filter(f.filter);
  return body as T;
}

/** Запрос с токеном HR; сеть недоступна — ApiError offline. */
async function send(path: string, init?: RequestInit): Promise<Response> {
  const isForm = path.startsWith('/api/v1/form/');
  const token = isForm ? '' : auth.token();
  if (token) {
    const headers = new Headers(init?.headers);
    headers.set('Authorization', `Bearer ${token}`);
    init = { ...init, headers };
  }
  try {
    return await fetch(API_BASE + path, init);
  } catch {
    throw new ApiError(0, OFFLINE_MESSAGE, true);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  if (isStaticDemo()) return staticRequest<T>(path, init);
  const isForm = path.startsWith('/api/v1/form/');
  const res = await send(path, init);
  const text = await res.text().catch(() => '');
  let body: unknown = undefined;
  try {
    body = text ? JSON.parse(text) : undefined;
  } catch {
    body = undefined;
  }
  if (!res.ok) {
    if (res.status === 0 || (res.status >= 500 && body === undefined)) throw new ApiError(res.status, OFFLINE_MESSAGE, true);
    if (res.status === 401 && !isForm) auth.markRequired();
    const msg = (body as { error?: { message?: string } } | undefined)?.error?.message;
    throw new ApiError(res.status, msg ?? `Ошибка ${res.status}`);
  }
  return (body ?? {}) as T;
}

const json = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
});

export const api = {
  overview: () => request<Overview>('/api/v1/overview'),
  pair: (id: string) => request<PairDetail>(`/api/v1/pairs/${encodeURIComponent(id)}`),
  brief: (id: string) => request<Brief>(`/api/v1/pairs/${encodeURIComponent(id)}/brief`),
  signals: (view: SignalsView = 'grouped', state = '', severity = '') =>
    request<{ view?: SignalsView; signals: GroupedSignal[]; current: number }>(`/api/v1/signals?view=${view}&state=${state}&severity=${severity}`),
  verdict: (id: string, status: string, comment = '', action = '') =>
    request<Verdict>(`/api/v1/signals/${encodeURIComponent(id)}/verdict`, json('POST', { status, comment, action })),
  thresholds: () => request<{ thresholds: Thresholds; defaults: Thresholds }>('/api/v1/thresholds'),
  saveThresholds: (t: Thresholds) => request<{ thresholds: Thresholds }>('/api/v1/thresholds', json('PUT', t)),
  sources: () => request<Sources>('/api/v1/sources'),
  setFormWindow: (period: number, closed: boolean) =>
    request<{ period: number; closed: boolean }>('/api/v1/survey/window', json('PUT', { period, closed })),
  importFile: (file: File, role: string, mapping?: ImportMapping, remember = false) => {
    const fd = new FormData();
    fd.append('file', file);
    if (role) fd.append('role', role);
    if (mapping) fd.append('mapping', JSON.stringify(mapping));
    if (remember) fd.append('remember', 'true');
    return request<ImportReport>('/api/v1/imports', { method: 'POST', body: fd });
  },
  /** Предпросмотр колонок файла: ничего не сохраняет. */
  previewImport: (file: File, role: string) => {
    const fd = new FormData();
    fd.append('file', file);
    if (role) fd.append('role', role);
    return request<ImportPreview>('/api/v1/imports/preview', { method: 'POST', body: fd });
  },
  /** Текущая структура анкеты с выученными названиями колонок — файлом. */
  surveyExport: async (): Promise<Blob> => {
    if (isStaticDemo()) throw new ApiError(403, STATIC_WRITE_MESSAGE);
    const res = await send('/api/v1/survey/export');
    if (!res.ok) {
      if (res.status === 401) auth.markRequired();
      if (res.status >= 500) throw new ApiError(res.status, OFFLINE_MESSAGE, true);
      const body = await res.json().catch(() => undefined);
      throw new ApiError(res.status, body?.error?.message ?? `Ошибка ${res.status}`);
    }
    return res.blob();
  },
  llmCheck: () => request<LlmCheck>('/api/v1/llm/check', { method: 'POST' }),
  form: (token: string) => request<FormInfo>(`/api/v1/form/${encodeURIComponent(token)}`),
  submitForm: (token: string, period: number, answers: Record<string, string | number>) =>
    request<{ ok: boolean; period: number }>(`/api/v1/form/${encodeURIComponent(token)}`, json('POST', { period, answers })),
  reset: () => request<{ ok: boolean }>('/api/v1/demo/reset', { method: 'POST' }),
};
