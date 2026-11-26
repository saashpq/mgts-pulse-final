import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { App } from './App';
import { commaNum, reactionDeadline, fmt, monthScores } from './format';
import { briefPoints } from './views/Brief';
import { RECONNECT_DELAYS } from './App';
import { match } from './router';
import { PriorityCase } from './components/PriorityCase';
import type { PairReport } from './api';

const pair = {
  pair: 'P2', intern: 'I-02', mentor: 'M-02', track: 'Автоматизация', status: 'attention', headline: 'Потеря мотивации стажёра',
  signals: [{ id: 'P2-S7', pair: 'P2', type: 'S7', title: 'Потеря мотивации стажёра', severity: 'high', source: 'hybrid', first_period: 1, last_period: 6, primary: true,
    explanation: 'Интерес упал', action: 'Пересмотреть задачи', due_days: 5, first_seen: 3, seen_at: '2026-11-12T12:00:00Z',
    evidence: [{ response_id: 'P2-intern-M6', role: 'intern', period: 6, question: 'i_comment', quote: 'Всё та же рутина' }] }],
  resolved: [], history: ['', '', 'high', 'high', 'high', 'high'], intern_index: [4.75, 4.75, 4.25, 3.75, 3.25, 3.13],
  mentor_index: [5, 5, 5, 5, 4.5, 4.5], detection: { ours: 3, naive: 0 }, summary: 'Пара P2: потеря мотивации',
};
const pairedClarity = {
  agreement_questions: ['Понятность задач', 'Понимание задач и ожиданий'],
  agreement_intern: [5, 5, 5, 4, 4, 5],
  agreement_mentor: [3, 3, 4, 3, 2, 3],
};
const solvedPair = {
  ...pair, pair: 'P5', intern: 'I-05', mentor: 'M-05', status: 'ok', headline: 'Пропуск анкеты', signals: [], history: ['', '', 'high', 'high', 'low', 'low'],
  detection: { ours: 3, naive: 3 }, summary: 'Пара P5',
};

it('prioritizes current direct critical evidence and quotes its originating comment', () => {
  const base = pair as PairReport;
  const ordinary = { ...base.signals[0], overdue: true };
  const critical = { ...base.signals[0], type: 'S10', id: 'P9-S10', title: 'Сигнал о возможном конфликте',
    evidence: [{ response_id: 'r1', role: 'intern' as const, period: 6, quote: 'Коллеги унижали меня.' }] };
  const old = { ...critical, id: 'P0-S10', last_period: 5, overdue: true, evidence: [
    { ...critical.evidence[0], period: 5, quote: 'На прошлой неделе меня оскорбили.' },
    { ...critical.evidence[0], quote: 'Сроки срываются.' },
  ] };
  const cases = [{ ...base, signals: [ordinary, old, critical] }];
  const { rerender } = render(<PriorityCase pairs={cases} current={6} />);
  expect(screen.getByRole('blockquote')).toHaveTextContent('Коллеги унижали меня.');
  rerender(<PriorityCase pairs={[{ ...base, signals: [old] }]} current={6} />);
  expect(screen.getByRole('blockquote')).toHaveTextContent('На прошлой неделе меня оскорбили.');
});
const pairDetail = {
  report: { ...pair, signals: [...pair.signals,
    { ...pair.signals[0], primary: false, id: 'P2-S1', type: 'S1', title: 'Снижение удовлетворённости', severity: 'medium', source: 'rules', action: 'Встреча 1:1', due_days: 3, seen_at: undefined, evidence: [] },
    { ...pair.signals[0], primary: false, id: 'P2-S9', type: 'S9', title: 'Повтор темы', severity: 'medium', source: 'text', action: 'Встреча 1:1', due_days: 3, seen_at: undefined, evidence: [] },
    { ...pair.signals[0], primary: false, id: 'P2-S4', type: 'S4', title: 'Ложный сигнал', severity: 'low', source: 'rules', action: 'Не показывать', due_days: 1, evidence: [], verdict: { status: 'false', at: 'now' } },
  ] },
  current: 6, periods: 6,
  questions: [
    { code: 'i_overall', role: 'intern', text: 'Как проходит стажировка?', dimension: 'satisfaction', mirror: 'm_overall', values: [5, 5, 4, 3, 3, 2.5] },
    { code: 'm_overall', role: 'mentor', text: 'Как справляется стажёр?', dimension: 'performance', mirror: 'i_overall', values: [5, 5, 5, 5, 4, 4] },
  ],
  comments: [
    { response_id: 'P2-intern-M6', role: 'intern', period: 6, question: 'i_comment', text: 'Всё та же рутина, нового не учусь.', analysis: { sentiment: -1, themes: ['routine'], quote: 'Всё та же рутина', analyzer: 'offline' } },
    { response_id: 'P2-mentor-M6', role: 'mentor', period: 6, question: 'm_comment', text: 'Работает нормально.', analysis: { sentiment: 0, themes: [], quote: '', analyzer: 'offline', masked: ['телефон', 'ФИО'] } },
  ],
  themes: { routine: { label: 'рутина', negative: true } },
};
const flat = (s: object, active = true) => ({ ...pair.signals[0], primary: false, intern: 'I-02', mentor: 'M-02', active, ...s });
const signalsList = [
  flat({ id: 'P3-S1', pair: 'P3', title: 'Средний сигнал', severity: 'medium', due_days: 3 }),
  flat({}),
  flat({ id: 'P5-S1', pair: 'P5', title: 'Снятый сигнал', severity: 'high' }, false),
];
const groupedList = [
  { ...flat({}), primary: true, related: ['P2-S1'], related_signals: [flat({ id: 'P2-S1', type: 'S1', title: 'Снижение удовлетворённости', severity: 'medium', due_days: 3 })] },
  { ...flat({ id: 'P3-S1', pair: 'P3', title: 'Средний сигнал', severity: 'medium', due_days: 3 }), primary: true, related_signals: [] },
  { ...flat({ id: 'P5-S1', pair: 'P5', title: 'Снятый сигнал', severity: 'high' }, false), related_signals: [] },
];
const overview = {
  data_origin: 'synthetic',
  report: { current: 6, periods: 6, pairs: [pair, solvedPair], attention: 1, watch: 0, intern_avg: [4, 4, 3.9, 3.8, 3.7, 3.7], mentor_avg: [4, 4, 4, 4, 3.6, 3.6],
    fill_rate: 1, signals_now: 1, confirmed: 0, false_positive: 0, earlier_or_only: 1, digest: 'Программа стажировок, месяц 6 из 6.',
    precision: [
      { type: 'S1', title: 'Снижение удовлетворённости', confirmed: 3, false: 0, resolved: 2, precision: 1, decisions: 5, enough: true, lo: 0.478, hi: 1 },
      { type: 'S3', title: 'Расхождение восприятия в паре', confirmed: 0, false: 2, resolved: 0, precision: 0, decisions: 2, enough: false,
        hint: 'Ложных срабатываний: 2 — загрубить порог gap_min с 1.00 до 1.25', suggest: { field: 'gap_min', from: 1, to: 1.25 } },
      { type: 'S13', title: 'Формальная анкета', confirmed: 0, false: 0, resolved: 0, precision: null, decisions: 0, enough: false },
    ],
    precision_total: { decisions: 4, precision: 0.5, enough: false },
    effect: {
      pairs_flagged_early: 2,
      detection_table: [{ pair: 'P1', ours: null, naive: null }, { pair: 'P2', ours: 3, naive: null }, { pair: 'P3', ours: 3, naive: 4 }, { pair: 'P5', ours: 3, naive: 3 }],
      verdict_coverage: 0.25, overdue: null, overdue_unknown: 2, active_signals: 8, decided_signals: 2,
    } },
  analyzer: { mode: 'offline', label: 'Словарь', model: '' },
  themes_over_time: [
    { code: 'routine', label: 'рутина, однотипные задачи', negative: true, by_period: [0, 0, 1, 1, 1, 3], total: 6, pairs: ['P2', 'P4'] },
    { code: 'praise', label: 'довольны работой', negative: false, by_period: [4, 3, 0, 0, 0, 0], total: 7, pairs: ['P1'] },
  ],
};
const brief = {
  pair: 'P2', intern: 'I-02', mentor: 'M-02', status: 'attention', generated_at: '2026-11-18T10:00:00Z',
  signals: [
    { id: 'P2-S7', side: 'intern', type: 'S7', title: 'Потеря мотивации стажёра', severity: 'high', explanation: 'Интерес упал', action: 'Пересмотреть задачи', due_days: 5, quotes: ['Всё та же рутина'] },
    { id: 'P2-S3', side: 'both', type: 'S3', title: 'Расхождение в паре', severity: 'high', explanation: 'Разрыв 2.00 балла', action: 'Совместная встреча', due_days: 5, quotes: [] },
    { id: 'P2-S2', side: 'mentor', type: 'S2', title: 'Снижение оценки наставником', severity: 'medium', explanation: 'С 5.00 до 4.50', action: 'Разговор с наставником', due_days: 5, quotes: [] },
  ],
  audiences: {
    intern: { with: 'intern', signals: ['P2-S7', 'P2-S3'], talking_points: ['Индекс стажёра в M6 — 3.13', 'Потеря мотивации стажёра — пересмотреть задачи (в течение 5 дн.)'] },
    mentor: { with: 'mentor', signals: ['P2-S3', 'P2-S2'], talking_points: ['Оценка наставника в M6 — 4.50.'] },
  },
  questions: [{ with: 'intern', text: 'Какие задачи были самыми интересными?' }, { with: 'mentor', text: 'Как ты даёшь обратную связь?' }],
  talking_points: ['Индекс стажёра в M6 — 3.13', 'Потеря мотивации стажёра — пересмотреть задачи (в течение 5 дн.)'],
  disclaimer: 'Бриф собран автоматически, решения принимает HR.',
};
const thresholds = { thresholds: { gap_min: 1, gap_high: 2, persist: 2 }, defaults: { gap_min: 1, gap_high: 2, persist: 2 } };
const sources = {
  links: [{ pair: 'P1', person: 'I-01', role: 'intern', url: 'http://x/form/t1', token: 't1' }],
  events: [], counts: { web: 3 }, analyzer: { mode: 'offline', label: 'Словарь', model: '' }, webhook: { path: '/api/v1/webhooks/yandex-forms', secret_set: true }, demo: true,
};
const form = {
  title: 'Опрос', role: 'intern', person: 'I-01', period: 6, periods: 6,
  questions: [
    { code: 'i_overall', role: 'intern', text: 'Как проходит стажировка?', kind: 'scale', max: 5 },
    { code: 'i_comment', role: 'intern', text: 'Комментарий', kind: 'text' },
  ],
};

const calls: { url: string; init?: RequestInit }[] = [];
beforeEach(() => {
  calls.length = 0;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    const body = url.includes('/brief') ? brief : url.includes('/thresholds') ? (init?.method === 'PUT' ? { thresholds: JSON.parse(String(init.body)) } : thresholds)
      : url.includes('/overview') ? overview : url.includes('/form/') && init?.method === 'POST' ? { ok: true, period: 6 } : url.includes('/form/') ? form
      : url.includes('/verdict') ? { status: 'confirmed', at: 'now' } : url.includes('/signals') ? (url.includes('view=all') ? { view: 'all', signals: signalsList, current: 6 } : { view: 'grouped', signals: groupedList, current: 6 })
      : url.includes('/pairs/') ? pairDetail : url.includes('/sources') ? sources : {};
    return new Response(JSON.stringify(body), { status: 200 });
  }));
});

const wrap = (ui: ReactNode) => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>);

test('обзор показывает пары, статус и тепловую карту', async () => {
  window.history.pushState(null, '', '/');
  wrap(<App />);
  expect(await screen.findAllByText('Требует вмешательства')).not.toHaveLength(0);
  expect(screen.getAllByText('Потеря мотивации стажёра').length).toBeGreaterThan(0);
  expect(screen.getAllByTitle('M3: Критично')).toHaveLength(2);
  expect(screen.getByRole('link', { name: /Посмотреть результаты программы/ })).toHaveAttribute('href', '/report');
  expect(screen.queryByText('Сводка для руководства')).toBeNull();
  // ячейка тепловой карты: символ + aria-label, клик ведёт в карточку пары
  const cell = screen.getByRole('button', { name: 'P2, M3: Критично' });
  expect(cell).toHaveTextContent('!');
  expect(screen.getByRole('button', { name: 'P5, M5: Наблюдать' })).toHaveTextContent('○');
  expect(screen.getByText(/Критический сигнал в M3–M4/)).toBeInTheDocument();
  expect(screen.getByText('Учебные данные')).toBeInTheDocument();
  cell.focus();
  await userEvent.keyboard('{Enter}');
  expect(window.location.pathname).toBe('/pairs/P2/months/3');
  expect(await screen.findByRole('heading', {name: 'Ответы пары P2 за M3'})).toBeVisible();
});

test('карточка пары: план действий, гантель, шкала раннего обнаружения, комментарии', async () => {
  window.history.pushState(null, '', '/pairs/P2');
  wrap(<App />);
  expect(await screen.findByText('Что сделать сейчас')).toBeInTheDocument();
  expect(screen.getByText('Система видит проблему с M3, средняя оценка — нет')).toBeInTheDocument();
  expect(screen.getByText('сигнал системы')).toBeInTheDocument();
  expect(screen.getByText(/так и не упала ниже 3/)).toBeInTheDocument();
  // действия уникальны, ложный сигнал в план не попадает, ближайший срок первым
  const plan = screen.getByRole('region', { name: 'Что сделать сейчас' });
  expect(within(plan).getAllByText('Встреча 1:1')).toHaveLength(1);
  expect(within(plan).queryByText('Не показывать')).toBeNull();
  const dues = within(plan).getAllByText(/^(до |Срок реакции)/).map((e) => e.textContent);
  expect(dues).toEqual(['до 17 ноября 2026 г.', 'Срок реакции — 3 дня с появления сигнала']);
  // гантель: разрыв ≥ 1 подписан, есть таблица значений
  expect(screen.getByText('Δ 1,5')).toBeInTheDocument();
  const toggles = screen.getAllByRole('button', { name: 'Таблица' });
  await userEvent.click(toggles[1]);
  expect(toggles[1]).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('rowheader', { name: 'Разница (наставник − стажёр)' })).toBeInTheDocument();
  // источник сигнала крупно, критичные раскрыты, остальные свёрнуты
  expect(screen.getAllByText('Оценки и текст').length).toBeGreaterThan(0);
  expect(screen.getByText(/2 сигнала/)).toBeInTheDocument();
  // комментарии: тональность словом и фильтры
  expect(screen.getByText('негатив')).toBeInTheDocument();
  expect(screen.getByText('нейтрально')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Только негатив' }));
  expect(screen.getByText('Показано 1 из 2')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Наставник' }));
  expect(screen.getByText('Нет комментариев с такими фильтрами.')).toBeInTheDocument();
  await userEvent.click(screen.getAllByRole('button', { name: 'Пороги' })[0]);
  expect(window.location.pathname).toBe('/thresholds');
});

test('ссылка из брифа раскрывает нужный сигнал и переводит фокус на его решение', async () => {
  window.history.pushState(null, '', '/pairs/P2#decision-P2-S1');
  wrap(<App />);
  const decision = await screen.findByRole('group', { name: 'Решение HR по сигналу «Снижение удовлетворённости»' });
  expect(decision).toHaveFocus();
  expect(decision).toHaveAttribute('data-linked-decision');
  expect(decision.closest('details')).toHaveAttribute('open');
  expect(within(decision).getByLabelText('Что сделано')).toBeVisible();
});

test('бриф к встрече 1:1: из карточки пары, вопросы обеим сторонам, печать', async () => {
  window.history.pushState(null, '', '/pairs/P2');
  wrap(<App />);
  expect(await screen.findByText('Перед моделью скрыто: телефон, ФИО')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Бриф к встрече 1:1' }));
  expect(window.location.pathname).toBe('/pairs/P2/brief');
  expect(await screen.findByRole('heading', { name: 'Пара P2' })).toBeVisible();
  expect(screen.getByRole('tab', { name: /Стажёр I-02/ })).toHaveAttribute('aria-selected', 'true');
  const intern = screen.getByRole('region', { name: 'Разговор со стажёром I-02' });
  expect(screen.queryByRole('region', {name:/Разговор с наставником/})).toBeNull();
  expect(within(intern).getByRole('link', { name: 'Исходный ответ' })).toHaveAttribute('href', '/pairs/P2/months/6#comment-intern-i_comment');
  expect(within(intern).getByText('«Всё та же рутина»')).toBeVisible();
  const proof = within(intern).getByRole('button', { name: /Основания из анкет/ });
  expect(proof).toHaveAttribute('aria-expanded', 'false');
  await userEvent.click(proof);
  expect(proof).toHaveAttribute('aria-expanded', 'true');
  expect(within(intern).getByText('Разрыв 2,00 балла')).toBeVisible();
  expect(within(intern).getByText('Индекс стажёра в M6 — 3,13')).toBeVisible();
  await userEvent.click(screen.getByRole('tab', { name: /Наставник M-02/ }));
  expect(location.hash).toBe('#brief-mentor');
  const mentor = screen.getByRole('region', { name: 'Разговор с наставником M-02' });
  expect(within(mentor).queryByText(/Всё та же рутина/)).toBeNull();
  expect(within(mentor).queryByText('Какие задачи были самыми интересными?')).toBeNull();
  expect(within(mentor).getByText('Оценка наставника в M6 — 4,50.')).toBeVisible();
  expect(within(mentor).getByRole('link',{name:'Оценки за M6'})).toHaveAttribute('href','/pairs/P2/months/6#index-mentor');
  await userEvent.click(within(mentor).getByRole('button', { name: /Ещё .*вопрос/ }));
  expect(within(mentor).getByText('Как ты даёшь обратную связь?')).toBeVisible();
  await userEvent.click(within(mentor).getByRole('button', { name: /Основания из анкет/ }));
  expect(within(mentor).getByText('Снижение оценки наставником')).toBeVisible();
  expect(within(mentor).getByText('Расхождение в паре')).toBeVisible();
  expect(screen.getByText(/решения принимает HR/)).toBeInTheDocument();
  const print = vi.fn();
  vi.stubGlobal('print', print);
  await userEvent.click(screen.getByRole('button', { name: 'Печать / PDF' }));
  expect(print).toHaveBeenCalled();
  expect(match('/pairs/P2/brief')).toEqual({ view: 'brief', id: 'P2' });
  await userEvent.click(screen.getByRole('tab', { name: /Стажёр I-02/ }));
  await userEvent.click(screen.getByRole('link', {name:'К решению по сигналу'}));
  expect(await screen.findByRole('group', {name:'Решение HR по сигналу «Потеря мотивации стажёра»'})).toHaveFocus();
});

test('результаты программы: матрица тем по месяцам, таблица и точность сигналов', async () => {
  window.history.pushState(null, '', '/report');
  wrap(<App />);
  expect(await screen.findByRole('heading', { name: 'О чём пишут в комментариях' })).toBeInTheDocument();
  const cell = screen.getByRole('img', { name: 'рутина, однотипные задачи, M6: 3 комментария' });
  expect(cell).toHaveTextContent('3');
  expect(cell).toHaveAttribute('title', expect.stringContaining('P2, P4'));
  expect(screen.getByRole('img', { name: 'довольны работой, M1: 4 комментария' })).toHaveTextContent('4');
  expect(screen.getByText('Тревожные темы')).toBeInTheDocument();
  // решений меньше 5 — процент не показываем
  expect(screen.getAllByText('решений HR: 4; для оценки нужно не меньше 5')).toHaveLength(2);
  expect(screen.queryByText('50%')).toBeNull();
  const section = screen.getByRole('region', { name: 'О чём пишут в комментариях' });
  await userEvent.click(within(section).getByRole('button', { name: 'Таблица' }));
  expect(within(section).getByRole('rowheader', { name: /довольны работой/ })).toBeInTheDocument();
  await userEvent.click(within(section).getByRole('button', { name: 'Таблица' }));
  await userEvent.click(within(section).getByRole('button', { name: 'P4' }));
  expect(window.location.pathname).toBe('/pairs/P4');
});

test('пороги: точность по решениям HR и «Применить» сохраняет новый порог', async () => {
  window.history.pushState(null, '', '/thresholds');
  wrap(<App />);
  expect(await screen.findByRole('heading', { name: 'Точность по решениям HR' })).toBeInTheDocument();
  expect(screen.getByText('нет решений')).toBeInTheDocument();
  expect(screen.getByText('5/5 = 100% (95% ДИ 48–100%)')).toBeInTheDocument();
  // 2 решения из 5 — процент не показываем, а подсказка остаётся
  expect(screen.getByText('мало решений')).toBeInTheDocument();
  expect(screen.queryByText('0%')).toBeNull();
  expect(screen.getByText(/загрубить порог gap_min/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /Применить/ }));
  await waitFor(() => expect(calls.some((c) => c.url.endsWith('/api/v1/thresholds') && c.init?.method === 'PUT')).toBe(true));
  const put = calls.find((c) => c.init?.method === 'PUT');
  expect(JSON.parse(String(put?.init?.body))).toEqual({ gap_min: 1.25, gap_high: 2, persist: 2 });
  await waitFor(() => expect(screen.getByLabelText(/^Расхождение стажёра и наставника/)).toHaveValue(1.25));
});

test('сигналы: счётчики в фильтрах и сортировка по критичности и сроку', async () => {
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  await userEvent.click(await screen.findByRole('button', { name: 'Все сигналы' }));
  const active = await screen.findByRole('button', { name: 'Активные 2' });
  expect(active).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('button', { name: 'Снятые 1' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Все 3' })).toBeInTheDocument();
  const titles = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent);
  expect(titles).toEqual(['Потеря мотивации стажёра', 'Средний сигнал']);
  await userEvent.click(screen.getByRole('button', { name: 'Все 3' }));
  expect(screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Потеря мотивации стажёра', 'Средний сигнал', 'Снятый сигнал']);
});

test('HR подтверждает сигнал', async () => {
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  await userEvent.click((await screen.findAllByRole('button', { name: 'Подтвердить' }))[0]);
  await waitFor(() => expect(calls.some((c) => c.url.includes('/signals/P2-S7/verdict') && c.init?.method === 'POST')).toBe(true));
  expect(screen.getAllByText('Всё та же рутина').length).toBeGreaterThan(0);
});

test('анкета: отправка после ответа на шкалы', async () => {
  window.history.pushState(null, '', '/form/tok');
  wrap(<App />);
  const send = await screen.findByRole('button', { name: 'Отправить' });
  expect(send).toBeDisabled();
  await userEvent.click(screen.getByLabelText('4'));
  await userEvent.type(screen.getByRole('textbox'), 'Всё хорошо');
  await userEvent.click(send);
  expect(await screen.findByText('Спасибо!')).toBeInTheDocument();
  const post = calls.find((c) => c.init?.method === 'POST');
  expect(JSON.parse(String(post?.init?.body))).toEqual({ period: 6, answers: { i_overall: 4, i_comment: 'Всё хорошо' } });
});

test('маршруты и форматирование', () => {
  expect(match('/pairs/P3')).toEqual({ view: 'pair', id: 'P3' });
  expect(match('/form/abc')).toEqual({ view: 'form', token: 'abc' });
  expect(match('/signals/')).toEqual({ view: 'signals' });
  expect(match('/x/y')).toEqual({ view: 'not-found' });
  expect(fmt(3.125)).toBe('3,13');
  expect(fmt(null)).toBe('—');
  expect(monthScores([4, null, 3, null], 4)).toEqual([null, 3]);
  expect(monthScores([4, null, 3, null], 3)).toEqual([3, null]);
  expect(monthScores([4, 5, null], 2)).toEqual([5, 4]);
  expect(reactionDeadline({ due_days: 2, seen_at: '2026-11-18T12:00:00Z' }).at).toBe(Date.parse('2026-11-20T12:00:00Z'));
  expect(reactionDeadline({ due_days: 2, seen_at: 'invalid' }).label).toBe('Срок реакции — 2 дня с появления сигнала');
});

test('сервер недоступен: понятное сообщение и «Повторить»', async () => {
  const ok = vi.mocked(fetch).getMockImplementation()!;
  let down = true;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    if (down) throw new TypeError('Failed to fetch');
    return ok(url, init);
  }));
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  expect(await screen.findByText('Сервер недоступен, повторяем…')).toBeInTheDocument();
  down = false;
  await userEvent.click(screen.getByRole('button', { name: 'Повторить' }));
  expect(await screen.findByRole('button', { name: 'Активные 2' })).toBeInTheDocument();
});

test('5xx без JSON — «сервер недоступен», 4xx с JSON — текст ошибки сервера', async () => {
  const { api, isOffline } = await import('./api');
  vi.stubGlobal('fetch', vi.fn(async () => new Response('Bad Gateway', { status: 502 })));
  const e1 = await api.overview().catch((e) => e);
  expect(e1.message).toBe('Сервер недоступен, повторяем…');
  expect(isOffline(e1)).toBe(true);
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { message: 'Порог вне диапазона' } }), { status: 400 })));
  const e2 = await api.overview().catch((e) => e);
  expect(e2.message).toBe('Порог вне диапазона');
  expect(isOffline(e2)).toBe(false);
});

class FakeES {
  static all: FakeES[] = [];
  readyState = 0;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners: Record<string, (() => void)[]> = {};
  constructor(public url: string) {
    FakeES.all.push(this);
  }
  addEventListener(t: string, f: () => void) {
    (this.listeners[t] ??= []).push(f);
  }
  emit(t: string) {
    this.listeners[t]?.forEach((f) => f());
  }
  close() {
    this.readyState = 2;
  }
}

test('живое обновление: потеря связи, переподключение с бэкоффом, одна инвалидация на пачку событий', async () => {
  expect(RECONNECT_DELAYS).toEqual([1000, 2000, 5000, 10000]);
  FakeES.all = [];
  vi.stubGlobal('EventSource', FakeES);
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  await screen.findByRole('button', { name: 'Активные 2' });
  const es = FakeES.all[0];
  act(() => es.onopen?.());
  // пачка событий → один перезапрос сигналов через 300 мс
  const signalsCalls = () => calls.filter((c) => c.url.includes('/api/v1/signals')).length;
  const before = signalsCalls();
  act(() => {
    es.emit('data.updated');
    es.emit('analysis.updated');
    es.emit('data.updated');
  });
  await waitFor(() => expect(signalsCalls()).toBe(before + 1));
  await new Promise((r) => setTimeout(r, 400));
  expect(signalsCalls()).toBe(before + 1);
  // сервер закрыл поток → индикатор в шапке и новый EventSource через 1 с
  act(() => {
    es.readyState = 2;
    es.onerror?.();
  });
  expect(screen.getByText(/Связь с сервером потеряна/)).toHaveAttribute('role', 'status');
  await waitFor(() => expect(FakeES.all).toHaveLength(2), { timeout: 2000 });
  act(() => FakeES.all[1].onopen?.());
  expect(screen.queryByText(/Связь с сервером потеряна/)).toBeNull();
  await waitFor(() => expect(signalsCalls()).toBe(before + 2));
  vi.unstubAllGlobals();
});

test('смена экрана: заголовок вкладки и фокус на h1; на /signals есть h2', async () => {
  window.history.pushState(null, '', '/');
  wrap(<App />);
  await screen.findByRole('heading', { name: 'Пары «стажёр — наставник»' });
  expect(document.title).toBe('Обзор пар — Пульс стажировки');
  await userEvent.click(screen.getByRole('button', { name: 'Сигналы' }));
  expect(document.title).toBe('Сигналы — Пульс стажировки');
  expect(screen.getByRole('heading', { level: 1, name: 'Сигналы' })).toHaveFocus();
  expect(await screen.findByRole('heading', { level: 2, name: 'Список сигналов' })).toBeInTheDocument();
  await userEvent.click(screen.getAllByRole('button', { name: /^P2/ })[0]);
  expect(document.title).toBe('Карточка пары P2 — Пульс стажировки');
});

test('пороги: «Применить» при несохранённых правках спрашивает и берёт значения из формы', async () => {
  window.history.pushState(null, '', '/thresholds');
  wrap(<App />);
  const gapHigh = await screen.findByLabelText(/^Критичное расхождение/);
  await userEvent.clear(gapHigh);
  await userEvent.type(gapHigh, '3');
  const confirm = vi.fn(() => false);
  vi.stubGlobal('confirm', confirm);
  await userEvent.click(screen.getByRole('button', { name: /Применить/ }));
  expect(confirm).toHaveBeenCalledWith(expect.stringContaining('Есть несохранённые правки'));
  expect(calls.some((c) => c.init?.method === 'PUT')).toBe(false);
  confirm.mockReturnValue(true);
  await userEvent.click(screen.getByRole('button', { name: /Применить/ }));
  await waitFor(() => expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true));
  const put = calls.find((c) => c.init?.method === 'PUT');
  expect(JSON.parse(String(put?.init?.body))).toEqual({ gap_min: 1.25, gap_high: 3, persist: 2 });
});

test('пороги: пустое поле — ошибка у поля, сохранить нельзя', async () => {
  window.history.pushState(null, '', '/thresholds');
  wrap(<App />);
  const persist = await screen.findByLabelText(/^Сколько месяцев подряд/);
  await userEvent.clear(persist);
  expect(screen.getByRole('alert')).toHaveTextContent('Введите число');
  expect(persist).toHaveAttribute('aria-invalid', 'true');
  expect(persist).toHaveAccessibleDescription('Введите число');
  const save = screen.getByRole('button', { name: 'Сохранить и пересчитать' });
  expect(save).toBeDisabled();
  await userEvent.type(persist, '3');
  expect(save).toBeEnabled();
  await userEvent.click(save);
  await waitFor(() => expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true));
  expect(JSON.parse(String(calls.find((c) => c.init?.method === 'PUT')?.init?.body))).toEqual({ gap_min: 1, gap_high: 2, persist: 3 });
});

test('«Скопировать» объявляет результат, «Сбросить демо» спрашивает подтверждение', async () => {
  const writeText = vi.fn(async () => {});
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
  window.history.pushState(null, '', '/report');
  wrap(<App />);
  // fireEvent: userEvent подменяет navigator.clipboard своей заглушкой
  fireEvent.click(await screen.findByRole('button', { name: 'Скопировать' }));
  expect(writeText).toHaveBeenCalledWith('Учебные данные. Программа стажировок, месяц 6 из 6.');
  expect(await screen.findByText('Скопировано')).toHaveAttribute('aria-live', 'polite');
  writeText.mockRejectedValueOnce(new Error('denied'));
  fireEvent.click(screen.getByRole('button', { name: 'Скопировать' }));
  expect(await screen.findByText('Не удалось скопировать')).toBeInTheDocument();

  await userEvent.click(screen.getByRole('button', { name: 'Источники данных' }));
  const confirm = vi.fn(() => false);
  vi.stubGlobal('confirm', confirm);
  await userEvent.click(await screen.findByRole('button', { name: 'Сбросить демо к синтетическим данным' }));
  expect(confirm).toHaveBeenCalled();
  expect(calls.some((c) => c.url.includes('/demo/reset'))).toBe(false);
});

test('анкета: ошибка отправки объявляется (role=alert)', async () => {
  const ok = vi.mocked(fetch).getMockImplementation()!;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) =>
    init?.method === 'POST' ? new Response(JSON.stringify({ error: { message: 'Анкета закрыта' } }), { status: 409 }) : ok(url, init)));
  window.history.pushState(null, '', '/form/tok');
  wrap(<App />);
  await userEvent.click(await screen.findByLabelText('4'));
  expect(document.title).toBe('Опрос — анкета стажировки');
  await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Анкета закрыта');
});

test('после закрытия или смены месяца устаревшую анкету можно перечитать', async () => {
  const original = vi.mocked(fetch).getMockImplementation()!;
  let closed = false;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    if (!url.includes('/form/')) return original(url, init);
    if (init?.method === 'POST') {
      closed = true;
      return new Response(JSON.stringify({ error: { message: 'Приём закрыт' } }), { status: 409 });
    }
    return new Response(JSON.stringify({ ...form, closed }));
  }));
  window.history.pushState(null, '', '/form/tok');
  wrap(<App />);
  await userEvent.click(await screen.findByLabelText('4'));
  await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
  await userEvent.click(await screen.findByRole('button', { name: 'Проверить текущий месяц' }));
  expect(await screen.findByRole('heading', { name: 'Приём ответов закрыт' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Отправить' })).toBeNull();
});

test('HR закрывает приём и открывает следующий месяц для тех же ссылок', async () => {
  const original = vi.mocked(fetch).getMockImplementation()!;
  let current = { period: 1, closed: false };
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/survey/window') && init?.method === 'PUT') {
      current = JSON.parse(String(init.body));
      return new Response(JSON.stringify(current));
    }
    if (url.endsWith('/sources')) return new Response(JSON.stringify({ ...sources, periods: 3, form_window: current }));
    return original(url, init);
  }));
  window.history.pushState(null, '', '/sources');
  wrap(<App />);
  await userEvent.click(await screen.findByRole('button', { name: 'Закрыть приём за месяц 1' }));
  expect(await screen.findByText('Приём ответов закрыт.', { exact: false })).toBeInTheDocument();
  await userEvent.selectOptions(screen.getByLabelText('Месяц анкеты'), '2');
  await userEvent.click(screen.getByRole('button', { name: 'Открыть месяц 2' }));
  expect(await screen.findByRole('button', { name: 'Закрыть приём за месяц 2' })).toBeEnabled();
  expect(current).toEqual({ period: 2, closed: false });
});

test('десятичная запятая в текстах и тезисы без повтора действий', () => {
  expect(commaNum('с 4.75 (M1) до 3.12 (M6), порог 0.25.')).toBe('с 4,75 (M1) до 3,12 (M6), порог 0,25.');
  expect(commaNum('дата 18.11.2026, версия 1.2.3, модель qwen2.5')).toBe('дата 18.11.2026, версия 1.2.3, модель qwen2.5');
  expect(commaNum('Индекс — 3.13.')).toBe('Индекс — 3,13.');
  expect(
    briefPoints({
      signals: [{ type: 'S1', title: 'T', severity: 'high', explanation: '', action: 'Срочно: встреча HR', due_days: 2, quotes: [] }],
      talking_points: ['T — срочно: встреча HR (в течение 2 дн.)', 'Индекс 3.13'],
    }),
  ).toEqual(['Индекс 3.13']);
});

test('сигналы по парам: главный сигнал и свёрнутые подтверждающие; переключение на «Все сигналы»', async () => {
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  const byPairs = await screen.findByRole('button', { name: 'По парам' });
  expect(byPairs).toHaveAttribute('aria-pressed', 'true');
  expect(calls.some((c) => c.url.includes('/api/v1/signals?view=grouped'))).toBe(true);
  // счётчики по виду: главных сигналов 2 активных
  expect(await screen.findByRole('button', { name: 'Активные 2' })).toBeInTheDocument();
  expect(screen.getAllByText('Главное')).toHaveLength(2);
  const rel = screen.getByText('Подтверждающие сигналы (1)').closest('details')!;
  expect(rel).not.toHaveAttribute('open');
  await userEvent.click(screen.getByText('Подтверждающие сигналы (1)'));
  expect(rel).toHaveAttribute('open');
  const row = within(rel).getAllByText('Снижение удовлетворённости').find((e) => e.closest('.sig-rel__row'))!;
  await userEvent.click(row);
  expect(row.closest('details')).toHaveAttribute('open');
  expect(within(row.closest('details')!).getByRole('heading', { level: 3, name: 'Снижение удовлетворённости' })).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Все сигналы' }));
  expect(await screen.findByRole('button', { name: 'Все 3' })).toBeInTheDocument();
  expect(calls.some((c) => c.url.includes('view=all'))).toBe(true);
  expect(screen.queryByText(/Подтверждающие сигналы/)).toBeNull();
});

test('результаты программы: блок «Сравнение с порогом оценки»; карточка пары помечает главный сигнал', async () => {
  window.history.pushState(null, '', '/report');
  wrap(<App />);
  const effect = await screen.findByRole('region', { name: 'Сравнение с порогом оценки' });
  expect(within(effect).getByText('видит только система')).toBeInTheDocument();
  expect(within(effect).getByText('раньше на 1 месяц')).toBeInTheDocument();
  expect(within(effect).getByText('одновременно')).toBeInTheDocument();
  expect(within(effect).getByText('сигналов не было')).toBeInTheDocument();
  expect(within(effect).getByText('25%')).toBeInTheDocument();
  expect(within(effect).getByText('нет данных о датах')).toBeInTheDocument();
  await userEvent.click(within(effect).getByRole('button', { name: 'P2' }));
  expect(await screen.findByText('Главное')).toBeInTheDocument();
});

test('токен HR: 401 → экран входа, токен в заголовке и в sessionStorage, «Выйти»', async () => {
  sessionStorage.clear();
  const ok = vi.mocked(fetch).getMockImplementation()!;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const authz = new Headers(init?.headers).get('Authorization');
    calls.push({ url, init });
    if (!url.includes('/form/') && authz !== 'Bearer s3cret')
      return new Response(JSON.stringify({ error: { message: 'нужен токен HR' } }), { status: 401 });
    return ok(url, init);
  }));
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  const input = await screen.findByLabelText('Токен HR');
  expect(document.title).toBe('Вход — Пульс стажировки');
  await userEvent.type(input, 's3cret');
  await userEvent.click(screen.getByRole('button', { name: 'Войти' }));
  expect(await screen.findByRole('button', { name: 'Активные 2' })).toBeInTheDocument();
  expect(sessionStorage.getItem('pulse.hrToken')).toBe('s3cret');
  await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
  expect(await screen.findByLabelText('Токен HR')).toBeInTheDocument();
  expect(sessionStorage.getItem('pulse.hrToken')).toBeNull();
  // анкета — без токена
  const { api, auth, eventsUrl } = await import('./api');
  act(() => auth.setToken('s3cret'));
  expect(eventsUrl()).toBe('/api/v1/events?token=s3cret');
  calls.length = 0;
  await api.form('tok');
  expect(new Headers(calls[0].init?.headers).get('Authorization')).toBeNull();
  act(() => auth.setToken(''));
});

test('статическое онлайн-демо: чтение из /demo/*.json, запись — понятная ошибка, баннер', async () => {
  vi.stubEnv('VITE_STATIC_DEMO', 'true');
  try {
    const { api, staticFile } = await import('./api');
    expect(staticFile('/api/v1/pairs/P2/brief')?.file).toBe('brief-P2.json');
    expect(staticFile('/api/v1/signals?view=all&state=&severity=')?.file).toBe('signals-all.json');
    const res = await api.signals('all', 'past');
    expect(calls.at(-1)?.url).toBe('/demo/signals-all.json');
    expect(res.signals.map((x) => x.id)).toEqual(['P5-S1']);
    const err = await api.verdict('P2-S7', 'confirmed').catch((e) => e);
    expect(err.message).toBe('В онлайн-демо только просмотр. Полная версия запускается локально — см. README');
    expect(calls.some((c) => c.url.includes('/verdict'))).toBe(false);

    window.history.pushState(null, '', '/signals');
    wrap(<App />);
    expect(await screen.findByText('Онлайн-демо: только просмотр, данные синтетические')).toBeInTheDocument();
    expect((await screen.findAllByText(/В рабочей версии здесь можно записать/)).length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Подтвердить' })).toBeNull();
  } finally {
    vi.unstubAllEnvs();
  }
});

test('статическое демо: анкета — заглушка с объяснением', async () => {
  vi.stubEnv('VITE_STATIC_DEMO', 'true');
  try {
    window.history.pushState(null, '', '/form/tok');
    wrap(<App />);
    expect(await screen.findByRole('heading', { name: 'Анкета в онлайн-демо не принимает ответы' })).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  } finally {
    vi.unstubAllEnvs();
  }
});

// ---------- Раунд 3: мастер импорта, QR, проверка модели ----------
const preview = {
  sheets: [{
    name: 'Лист1', role: '', rows: 30,
    columns: [
      { index: 0, title: 'Код практиканта', target: null, samples: ['I-01', 'I-02', 'I-03'] },
      { index: 1, title: 'Номер месяца', target: null, samples: ['1', '2', '3'] },
      { index: 2, title: 'Оценка стажировки', target: 'i_overall', samples: ['5', '4', '4'] },
      { index: 3, title: 'Отзыв', target: null, samples: ['Всё отлично'] },
    ],
  }],
  targets: [
    { value: 'intern_id', label: 'ID стажёра', role: 'any' }, { value: 'mentor_id', label: 'ID наставника', role: 'any' },
    { value: 'period', label: 'Месяц', role: 'any' }, { value: 'skip', label: 'Не загружать', role: 'any' },
    { value: 'i_overall', label: 'Как проходит стажировка?', role: 'intern' }, { value: 'i_comment', label: 'Комментарий стажёра', role: 'intern' },
    { value: 'm_overall', label: 'Как справляется стажёр?', role: 'mentor' },
  ],
};
const importReport = { report: { sheets: [{ name: 'Лист1', role: 'intern', mapped: ['Код практиканта'], unmapped: null, accepted: 30 }], accepted: 30, rejected: 0 }, changed: 30, unchanged: 0, remembered: 3 };
const llmOk = { mode: 'llm', model: 'qwen2.5:7b', ok: true, latency_ms: 1840, sample: 'Задачи однотипные, наставник не отвечает в чате, думаю, стоит ли продолжать.',
  analysis: { sentiment: -0.8, themes: ['routine'], quote: 'наставник не отвечает в чате', analyzer: 'llm:qwen2.5:7b' }, error: null };
const llmOff = { mode: 'offline', model: '', ok: false, latency_ms: null, sample: llmOk.sample, analysis: null, error: 'Модель не подключена: задайте LLM_BASE_URL, LLM_MODEL, LLM_API_KEY' };

function withRoutes(routes: Record<string, unknown>) {
  const ok = vi.mocked(fetch).getMockImplementation()!;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const hit = Object.keys(routes).find((k) => url.endsWith(k));
    if (hit) {
      calls.push({ url, init });
      return new Response(JSON.stringify(routes[hit]), { status: 200 });
    }
    return ok(url, init);
  }));
}

test('во время проверки книги файл и роль заблокированы, смена роли сбрасывает сопоставление', async () => {
  const { api } = await import('./api');
  const { ImportSection } = await import('./views/ImportWizard');
  let finish!: (value: Awaited<ReturnType<typeof api.previewImport>>) => void;
  const pending = vi.spyOn(api, 'previewImport').mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  try {
    wrap(<ImportSection />);
    const file = screen.getByLabelText('Файл выгрузки');
    const role = screen.getByLabelText('Чьи ответы');
    await userEvent.upload(file, new File(['A'], 'A.csv', { type: 'text/csv' }));
    await userEvent.click(screen.getByRole('button', { name: 'Проверить колонки' }));
    expect(file).toBeDisabled();
    expect(role).toBeDisabled();
    await act(async () => finish({ sheets: [], targets: [] }));
    expect(await screen.findByRole('group', { name: 'Сопоставление колонок' })).toBeInTheDocument();
    await userEvent.selectOptions(role, 'mentor');
    expect(screen.queryByRole('group', { name: 'Сопоставление колонок' })).toBeNull();
    expect(file).toBeEnabled();
  } finally {
    pending.mockRestore();
  }
});

test('мастер импорта: нераспознанные колонки, обязательные ID и месяц, дубликаты, отправка mapping', async () => {
  withRoutes({ '/api/v1/imports/preview': preview, '/api/v1/imports': importReport });
  window.history.pushState(null, '', '/sources');
  wrap(<App />);
  const file = new File(['Код практиканта;Номер месяца'], 'чужие.csv', { type: 'text/csv' });
  await userEvent.upload(await screen.findByLabelText('Файл выгрузки'), file);
  await userEvent.click(screen.getByRole('button', { name: 'Проверить колонки' }));
  expect(await screen.findByText('Не распознано колонок: 3')).toBeInTheDocument();
  expect(screen.getAllByText('не распознано')).toHaveLength(3);
  expect(screen.getByText('I-02')).toBeInTheDocument();
  const upload = screen.getByRole('button', { name: 'Загрузить' });
  expect(upload).toBeDisabled();
  expect(screen.getByText(/Укажите, чьи ответы/)).toBeInTheDocument();
  expect(screen.getByText(/Не выбрана колонка с ID участника/)).toBeInTheDocument();
  expect(screen.getByText(/Не выбрана колонка «Месяц»/)).toBeInTheDocument();

  await userEvent.selectOptions(screen.getByLabelText('Чьи ответы', { selector: '.map-sheet select' }), 'intern');
  await userEvent.selectOptions(screen.getByLabelText('Куда загрузить колонку «Код практиканта»'), 'intern_id');
  await userEvent.selectOptions(screen.getByLabelText('Куда загрузить колонку «Номер месяца»'), 'period');
  // один вопрос на две колонки — ошибка у обеих
  const review = screen.getByLabelText('Куда загрузить колонку «Отзыв»');
  await userEvent.selectOptions(review, 'i_overall');
  expect(screen.getByText(/«Как проходит стажировка\?» выбрано для нескольких колонок/)).toBeInTheDocument();
  expect(review).toHaveAttribute('aria-invalid', 'true');
  expect(screen.getByLabelText('Куда загрузить колонку «Оценка стажировки»')).toHaveAttribute('aria-invalid', 'true');
  expect(upload).toBeDisabled();
  await userEvent.selectOptions(review, 'i_comment');
  expect(screen.queryByText(/выбрано для нескольких колонок/)).toBeNull();
  // вопросы наставника на листе стажёров не предлагаются
  expect(within(review).queryByRole('option', { name: 'Как справляется стажёр?' })).toBeNull();
  expect(screen.getByRole('checkbox', { name: 'Запомнить названия колонок для следующих загрузок' })).toBeChecked();
  expect(upload).toBeEnabled();
  await userEvent.click(upload);
  expect(await screen.findByText(/Принято строк: 30/)).toBeInTheDocument();
  expect(screen.getByText(/Запомнено названий колонок: 3./)).toBeInTheDocument();
  const sent = calls.find((c) => c.url.endsWith('/api/v1/imports'))!.init!.body as FormData;
  expect(JSON.parse(String(sent.get('mapping')))).toEqual({
    'Лист1': { role: 'intern', columns: { 0: 'intern_id', 1: 'period', 2: 'i_overall', 3: 'i_comment' } },
  });
  expect(sent.get('remember')).toBe('true');
  expect(sent.get('file')).toBeInstanceOf(File);
  // мастер закрыт после загрузки
  expect(screen.queryByRole('button', { name: 'Загрузить' })).toBeNull();
});

test('мастер импорта: «Загрузить без проверки» — прежний путь без mapping; без «Запомнить» remember не уходит', async () => {
  withRoutes({ '/api/v1/imports/preview': preview, '/api/v1/imports': importReport });
  const { buildMapping, sheetIssues } = await import('./views/ImportWizard');
  // лист без выбранных колонок не загружается, но передаётся пустым — иначе сервер разобрал бы его сам
  expect(buildMapping(preview.sheets as never, [{ role: '', columns: { 0: '', 1: 'skip' } }], 'mentor')).toEqual({ 'Лист1': { role: 'mentor', columns: { 1: 'skip' } } });
  expect(sheetIssues({ role: 'mentor', columns: { 0: 'mentor_id', 1: 'period', 2: 'i_overall' } }, preview.targets as never).errors).toEqual([
    'Вопрос «Как проходит стажировка?» из анкеты стажёра, а лист — наставников.',
  ]);
  window.history.pushState(null, '', '/sources');
  wrap(<App />);
  await userEvent.upload(await screen.findByLabelText('Файл выгрузки'), new File(['x'], 'a.csv'));
  await userEvent.click(screen.getByRole('button', { name: 'Загрузить без проверки' }));
  expect(await screen.findByText(/Принято строк: 30/)).toBeInTheDocument();
  const sent = calls.find((c) => c.url.endsWith('/api/v1/imports'))!.init!.body as FormData;
  expect(sent.get('mapping')).toBeNull();
  expect(sent.get('remember')).toBeNull();
});

test('QR-код анкеты: модалка, подсказка про localhost, Esc закрывает и возвращает фокус', async () => {
  const { isLoopback, absoluteUrl } = await import('./components/QrDialog');
  expect(isLoopback('http://localhost:5173/form/t')).toBe(true);
  expect(isLoopback('http://127.0.0.1:8080/form/t')).toBe(true);
  expect(isLoopback('http://192.168.1.20:8080/form/t')).toBe(false);
  expect(isLoopback('http://localhost.example.ru/form/t')).toBe(false);
  expect(absoluteUrl('/form/t1')).toBe(`${window.location.origin}/form/t1`);
  withRoutes({ '/api/v1/sources': { ...sources, links: [...sources.links, { pair: 'P2', person: 'M-02', role: 'mentor', url: 'http://localhost:5173/form/t2', token: 't2' }] } });
  window.history.pushState(null, '', '/sources');
  wrap(<App />);
  const btn = await screen.findByRole('button', { name: 'QR-код анкеты I-01' });
  await userEvent.click(btn);
  const dlg = screen.getByRole('dialog', { name: 'Анкета: стажёр I-01, пара P1' });
  expect(within(dlg).getByText('Для жюри: сканируйте телефоном')).toBeInTheDocument();
  expect(within(dlg).getByRole('img', { name: 'QR-код ссылки http://x/form/t1' }).querySelector('path')?.getAttribute('d')).toMatch(/^M\d/);
  expect(within(dlg).queryByText(/Телефон не откроет localhost/)).toBeNull();
  expect(within(dlg).getByRole('button', { name: 'Закрыть' })).toHaveFocus();
  await userEvent.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(btn).toHaveFocus();
  await userEvent.click(screen.getByRole('button', { name: 'QR-код анкеты M-02' }));
  expect(screen.getByRole('dialog')).toHaveTextContent('Телефон не откроет localhost — задайте PUBLIC_URL=http://<IP ноутбука>:8080');
  await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }));
  expect(screen.queryByRole('dialog')).toBeNull();
});

test('«Проверить модель»: offline — инструкция и docs/LLM.md; llm — задержка, темы, цитата', async () => {
  withRoutes({ '/api/v1/llm/check': llmOff });
  window.history.pushState(null, '', '/sources');
  wrap(<App />);
  await userEvent.click(await screen.findByRole('button', { name: 'Проверить модель' }));
  expect(await screen.findByText('Модель не подключена — работает словарь')).toBeInTheDocument();
  expect(screen.getByText(/Модель не подключена: задайте LLM_BASE_URL/)).toBeInTheDocument();
  expect(screen.getByText('docs/LLM.md')).toBeInTheDocument();
  expect(calls.find((c) => c.url.endsWith('/llm/check'))?.init?.method).toBe('POST');

  withRoutes({ '/api/v1/llm/check': llmOk });
  await userEvent.click(screen.getByRole('button', { name: 'Проверить модель' }));
  expect(await screen.findByText('Модель распознала темы контрольного примера')).toBeInTheDocument();
  expect(screen.getByText('qwen2.5:7b')).toBeInTheDocument();
  expect(screen.getByText('1 840 мс')).toBeInTheDocument();
  expect(screen.getByText('«наставник не отвечает в чате»')).toBeInTheDocument();
  expect(screen.getByText('негатив')).toBeInTheDocument();
  expect(screen.queryByText('docs/LLM.md')).toBeNull();
});

test('карточка пары: бейдж «Модель»/«Словарь» и причина отклонения ответа модели', async () => {
  const detail = {
    ...pairDetail,
    comments: [
      { ...pairDetail.comments[0], analysis: { ...pairDetail.comments[0].analysis, analyzer: 'llm:qwen2.5:7b' } },
      { ...pairDetail.comments[1], analysis: { ...pairDetail.comments[1].analysis, note: 'ответ модели отклонён: цитата не найдена в комментарии' } },
    ],
  };
  withRoutes({ '/api/v1/pairs/P2': detail });
  window.history.pushState(null, '', '/pairs/P2');
  wrap(<App />);
  expect(await screen.findByText('Модель')).toHaveAttribute('title', 'Разбор моделью qwen2.5:7b');
  expect(screen.getByText('Словарь')).toBeInTheDocument();
  expect(screen.getByText('ответ модели отклонён: цитата не найдена в комментарии, использован словарь')).toBeInTheDocument();
});

test('статическое демо: список участников доступен, действия локальной версии скрыты', async () => {
  vi.stubEnv('VITE_STATIC_DEMO', 'true');
  try {
    window.history.pushState(null, '', '/sources');
    wrap(<App />);
    expect(await screen.findByText(/Чтобы загрузить свой файл/)).toBeInTheDocument();
    expect(screen.queryByLabelText('Файл выгрузки')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Проверить модель' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'QR-код анкеты I-01' })).toBeNull();
    await userEvent.type(screen.getByLabelText('Поиск участника'), 'I-01');
    expect(screen.getByRole('table')).toHaveTextContent('I-01');
    expect(calls.some((c) => c.url.includes('/imports') || c.url.includes('/llm/'))).toBe(false);
  } finally {
    vi.unstubAllEnvs();
  }
});

// ---------- R4: качество системы, интервалы, действия HR ----------

/** Подменяет ответы поверх базового мока (запросы по-прежнему пишутся в calls). */
function patchResponses(patch: (url: string, body: any, init?: RequestInit) => any) {
  const orig = fetch as unknown as (u: string, i?: RequestInit) => Promise<Response>;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const res = await orig(url, init);
    const body = await res.json();
    return new Response(JSON.stringify(patch(url, body, init) ?? body), { status: 200 });
  }));
}

const quality = {
  ground_truth: 'synthetic',
  evaluation: {
    episodes: 5, found: 4, recall: { value: 0.8, lo: 0.284, hi: 0.995 }, alerts: 6, true_alerts: 5, precision: { value: 0.833, lo: 0.359, hi: 0.996 },
    median_delay: 0, naive: { found: 2, median_delay: 1 }, earlier: 2, same: 1, later: 0, only_ours: 1, only_naive: 0, false_pairs: ['P1'],
  },
  alert_load: { active_cards: 3, per_10_pairs: 6, attention_share: 0.6, flapping: ['P5-S12'] },
  scales: { noise_sigma: 0.31, ceiling: [{ question: 'i_overall', share: 0.34 }, { question: 'm_overall', share: 0.1 }], ceiling_threshold: 0.15, flat_share: 0.08 },
  agreement: { bias: 0.03, sd: 1.2, loa_lo: -2.35, loa_hi: 2.42, n: 30, method: 'повторные измерения' },
  icc_note: 'ICC не считаем: нужно не меньше 30 пар.',
  text_ai: { analyzer: 'llm:qwen2.5', model: 'qwen2.5', prompt_hash: 'a1b2c3d4e5f6', total: 20, rejected: 2, reasons: { 'тема вне списка': 1, 'цитаты нет в тексте': 1 },
    agreement: [{ theme: 'рутина', kappa: 0.72, both: 5, only_offline: 1, only_llm: 1 }, { theme: 'конфликт', kappa: 0.2, both: 1, only_offline: 2, only_llm: 1 }] },
  goldset: null,
  action_effect: { with_action: 2, measured: 1, median_delta: 0.8, note: 'Без контрольной группы это наблюдение, а не доказательство.' },
  caveat: 'Метрики на 5 парах; интервалы широкие. Сигнал — повод для разговора, решение принимает HR.',
};
const actions = [
  { signal: 'P2-S7', pair: 'P2', type: 'S7', title: 'Потеря мотивации стажёра', status: 'confirmed', action: 'встреча 1:1 проведена', period: 4, role: 'intern', before: 3.75, after: 4.55, delta: 0.8, months: 1 },
  { signal: 'P2-S1', pair: 'P2', type: 'S1', title: 'Снижение удовлетворённости', status: 'resolved', action: 'сменили задачи', period: 6, role: 'intern', before: 3.13, after: null, delta: null, months: 0 },
];

test('интервалы точности и формулировки: «4/5 = 80% (95% ДИ 28–99%)», смещение и κ словами', async () => {
  const { pctCI, ratioCI, signed, biasText, kappaText } = await import('./format');
  expect(ratioCI(4, 5, 0.8, 0.284, 0.995)).toBe('4/5 = 80% (95% ДИ 28–99%)');
  expect(ratioCI(5, 5, 1, 0.478, 1)).toBe('5/5 = 100% (95% ДИ 48–100%)');
  expect(ratioCI(0, 5, 0, 0, 0.522)).toBe('0/5 = 0% (95% ДИ 0–52%)');
  // граница 0,996 не округляется до «100%», 0,003 — до «0%»
  expect(pctCI(0.5, 0.003, 0.996)).toBe('50% (95% ДИ 1–99%)');
  // старый сервер без границ — просто процент
  expect(ratioCI(4, 5, 0.8)).toBe('4/5 = 80%');
  expect(pctCI(null)).toBe('—');
  expect(signed(0.8)).toBe('+0,8');
  expect(signed(-0.42, 2)).toBe('−0,42');
  expect(signed(0)).toBe('0,0');
  expect(biasText(1.75)).toBe('стажёр оценивает в среднем на 1,75 выше наставника');
  expect(biasText(-0.5)).toBe('наставник оценивает в среднем на 0,50 выше стажёра');
  expect(biasText(0.03)).toBe('в среднем стажёр и наставник оценивают одинаково');
  expect(kappaText(0.72)).toBe('существенное');
  expect(kappaText(null)).toBe('не посчитать');
});

test.each([
  ['/report', 'Средняя < 3,5'],
  ['/quality', 'Правило «средняя < 3,5»'],
  ['/pairs/P2', 'Средняя так и не упала ниже 3,5: по ней проблему не видно.'],
])('изменённый порог сравнения на %s', async (path, label) => {
  patchResponses((url, body) => url.includes('/overview')
    ? { ...body, report: { ...body.report, quality, low_score: 3.5 } }
    : url.includes('/pairs/') ? { ...body, low_score: 3.5 } : undefined);
  window.history.pushState(null, '', path);
  wrap(<App />);
  expect(await screen.findByText(label)).toBeInTheDocument();
});

test('экран «Качество системы»: метрики с интервалами, сравнение с правилом, шкалы, согласие, ИИ, действия; оговорка видна', async () => {
  patchResponses((url, body) => (url.includes('/overview') ? {
    ...body, report: { ...body.report, quality, actions, pairs: body.report.pairs.map((p: PairReport) => ({ ...p, ...pairedClarity })) },
  } : undefined));
  window.history.pushState(null, '', '/');
  wrap(<App />);
  await userEvent.click(await screen.findByRole('button', { name: 'Качество системы' }));
  expect(window.location.pathname).toBe('/quality');
  expect(await screen.findByRole('note', { name: 'Оговорка' })).toHaveTextContent('Метрики на 5 парах; интервалы широкие');
  expect(screen.getByText('По учебным примерам.')).toBeInTheDocument();
  // раннее предупреждение
  const early = screen.getByRole('region', { name: 'Раннее предупреждение' });
  expect(early.querySelector('details')).not.toHaveAttribute('open');
  await userEvent.click(early.querySelector('summary')!);
  expect(within(early).getByText('4/5 = 80% (95% ДИ 28–99%)')).toBeInTheDocument();
  expect(within(early).getByText('5/6 = 83% (95% ДИ 36–99%)')).toBeInTheDocument();
  expect(within(early).getAllByText('0 мес.')).toHaveLength(2);
  expect(within(early).getByText('1 мес.')).toBeInTheDocument();
  expect(within(early).getByText('2 из 5')).toBeInTheDocument();
  expect(within(early).getByText('Только наша система')).toBeVisible();
  expect(within(early).getByText(/Какую долю известных проблем/)).toBeInTheDocument();
  // нагрузка и шкалы
  const load = screen.getByRole('region', { name: 'Нагрузка на HR' });
  await userEvent.click(load.querySelector('summary')!);
  expect(within(load).getByText('P5-S12')).toBeInTheDocument();
  expect(within(load).getByText('60%')).toBeInTheDocument();
  const scales = screen.getByRole('region', { name: 'Данные и шкалы' });
  await userEvent.click(scales.querySelector('summary')!);
  expect(within(scales).getByText('0,31 балла')).toBeInTheDocument();
  expect(within(scales).getByRole('list', { name: 'Доля максимальных оценок по вопросам, порог 15%' })).toBeInTheDocument();
  expect(within(scales).getAllByText(/выше порога/)).toHaveLength(1);
  // согласие сторон: смещение словами, мини-график и таблица
  const agree = screen.getByRole('region', { name: 'Согласие сторон' });
  await userEvent.click(agree.querySelector('summary')!);
  expect(within(agree).getByText('+0,03')).toBeInTheDocument();
  expect(within(agree).getByText('−2,35 … +2,42')).toBeInTheDocument();
  expect(within(agree).getByText(/ICC не считаем/)).toBeInTheDocument();
  expect(within(agree).getByRole('img', { name: /12 месяцев с оценками обеих сторон/ })).toBeInTheDocument();
  await userEvent.click(within(agree).getByRole('button', { name: 'Таблица' }));
  expect(within(agree).getByRole('rowheader', { name: 'P2, M6' })).toBeInTheDocument();
  // ИИ-разбор: причины, κ словами, gold-set не проводился
  const ai = screen.getByRole('region', { name: 'ИИ-разбор комментариев' });
  await userEvent.click(within(ai).getByText('В этой программе'));
  expect(within(ai).getByText('2 из 20 (10%)')).toBeInTheDocument();
  expect(within(ai).getByText('тема вне списка')).toBeInTheDocument();
  expect(within(ai).getByText('существенное')).toBeInTheDocument();
  expect(within(ai).getByText('слабое')).toHaveClass('prec__low');
  expect(within(ai).getByText(/Не проводилась для текущей программы/)).toBeInTheDocument();
  await userEvent.click(within(ai).getByText('Отдельный эксперимент'));
  expect(within(ai).getByText('Пропущен риск ухода')).toBeVisible();
  // эффект действий: медиана, оговорка, журнал
  const act = screen.getByRole('region', { name: 'Эффект действий HR' });
  await userEvent.click(act.querySelector('summary')!);
  expect(within(act).getByText('+0,8 балла')).toBeInTheDocument();
  expect(within(act).getByText('Без контрольной группы это наблюдение, а не доказательство.')).toBeInTheDocument();
  expect(within(act).getByText('после действия индекс стажёра +0,8 за 1 мес.')).toBeInTheDocument();
  expect(within(act).getByText('эффект пока не измерить: после M6 анкет ещё нет')).toBeInTheDocument();
});

test('экран качества на старом снимке без quality: честное «нет данных», оговорка по числу пар, без падения', async () => {
  window.history.pushState(null, '', '/quality');
  wrap(<App />);
  expect(await screen.findByRole('note', { name: 'Оговорка' })).toHaveTextContent(
    'Метрики на 2 парах; интервалы широкие. Сигнал — повод для разговора, решение принимает HR.',
  );
  expect(screen.getByText(/В этом отчёте доступны только отдельные показатели/)).toBeInTheDocument();
  expect(screen.getByText('В отчёте это не указано.')).toBeInTheDocument();
  expect(screen.getByText(/В отчёте нет данных о том, сколько проблем система обнаружила/)).toBeInTheDocument();
  // то, что можно показать из текущих данных, остаётся
  expect(screen.getAllByText('мало решений (4 из 5)').length).toBeGreaterThan(0);
  expect(screen.getByText('25%')).toBeInTheDocument();
  expect(screen.getByText(/Не проводилась/)).toBeInTheDocument();
  expect(screen.getByText(/отмечайте «Что сделано»/)).toBeInTheDocument();
});

test('экран качества: истина по решениям HR без эпизодов — объяснение, как получить оценку', async () => {
  patchResponses((url, body) =>
    url.includes('/overview') ? { ...body, report: { ...body.report, quality: { ground_truth: 'hr_verdicts', evaluation: null, caveat: '' } } } : undefined,
  );
  window.history.pushState(null, '', '/quality');
  wrap(<App />);
  expect(await screen.findByText('По решениям HR.')).toBeInTheDocument();
  expect(screen.getByText(/независимая проверка пар/)).toBeInTheDocument();
  expect(screen.getByText(/HR ещё не подтвердил ни одного сигнала/)).toBeInTheDocument();
  expect(screen.getByRole('note', { name: 'Оговорка' })).toHaveTextContent('Метрики на 2 парах');
});

test('самоотобранные решения HR не дают независимой оценки полноты', async () => {
  patchResponses((url, body) => url.includes('/overview') ? { ...body, report: { ...body.report,
    quality: { ground_truth: 'hr_verdicts', evaluation: { episodes: 3 }, caveat: '' } } } : undefined);
  window.history.pushState(null, '', '/quality');
  wrap(<App />);
  expect(await screen.findByText(/известно эпизодов: 3/)).toBeInTheDocument();
  expect(screen.queryByText('Полнота: найдено проблем')).toBeNull();
  expect(screen.queryByText(/По каждой проблеме: кто заметил первым/)).toBeNull();
});

test('вердикт: поле «что сделано» с подсказками уходит в POST; в карточке — действие и месяц', async () => {
  window.history.pushState(null, '', '/signals');
  wrap(<App />);
  const card = (await screen.findAllByRole('article'))[0];
  const chip = within(card).getByRole('button', { name: 'встреча 1:1 проведена' });
  await userEvent.click(chip);
  expect(chip).toHaveAttribute('aria-pressed', 'true');
  expect(within(card).getByLabelText('Что сделано')).toHaveValue('встреча 1:1 проведена');
  // «другое» очищает подсказку и ставит фокус в поле
  await userEvent.click(within(card).getByRole('button', { name: 'другое' }));
  expect(within(card).getByLabelText('Что сделано')).toHaveValue('');
  expect(within(card).getByLabelText('Что сделано')).toHaveFocus();
  await userEvent.type(within(card).getByLabelText('Что сделано'), 'поговорили с родителями');
  await userEvent.click(within(card).getByRole('button', { name: 'Подтвердить' }));
  await waitFor(() => expect(calls.some((c) => c.url.includes('/verdict'))).toBe(true));
  const post = calls.find((c) => c.url.includes('/verdict'));
  expect(JSON.parse(String(post?.init?.body))).toEqual({ status: 'confirmed', comment: '', action: 'поговорили с родителями' });

  const { SignalCard } = await import('./components/SignalCard');
  wrap(<SignalCard s={{ ...pair.signals[0], verdict: { status: 'resolved', at: 'now', action: 'сменили задачи', period: 4 } } as any} />);
  expect(screen.getByText('Решён в M4 · сделано: сменили задачи')).toBeInTheDocument();
});

test('карточка пары: согласие сторон со знаком и словами, история действий с delta; ветка S1 и анализатор цитаты', async () => {
  patchResponses((url, body) => {
    if (url.includes('/overview')) return { ...body, report: { ...body.report, actions } };
    if (url.includes('/pairs/') && !url.includes('/brief')) {
      const s1 = { ...body.report.signals[1], explanation: 'z = −3.4 к собственной базе 4.5', evidence: [] };
      const s7 = { ...body.report.signals[0], evidence: [{ ...body.report.signals[0].evidence[0], analyzer: 'llm:qwen2.5' }] };
      return { ...body, report: { ...body.report, ...pairedClarity, signals: [s7, s1, ...body.report.signals.slice(2)], agreement: { bias: 1.75, sd: 0.4, loa_lo: 0.97, loa_hi: 2.53, n: 6 } } };
    }
    return undefined;
  });
  window.history.pushState(null, '', '/pairs/P2');
  wrap(<App />);
  const agree = await screen.findByRole('region', { name: 'Согласие сторон' });
  expect(within(agree).getByText('Смещение +1,75')).toBeInTheDocument();
  expect(within(agree).getByText(/стажёр оценивает в среднем на 1,75 выше наставника/)).toBeInTheDocument();
  expect(within(agree).getByText(/\+0,97…\+2,53 балла · 6 месяцев/)).toBeInTheDocument();
  const hist = await screen.findByRole('region', { name: 'Что уже сделано и что изменилось' });
  expect(within(hist).getByText('после действия индекс стажёра +0,8 за 1 мес.')).toBeInTheDocument();
  expect(within(hist).getByText('(3,75 → 4,55)')).toBeInTheDocument();
  expect(within(hist).getByText('эффект пока не измерить: после M6 анкет ещё нет')).toBeInTheDocument();
  // ветка срабатывания S1 и точность типа с интервалом
  expect(screen.getByText('резкий провал к собственному уровню пары')).toBeInTheDocument();
  expect(screen.getAllByText('5/5 = 100% (95% ДИ 48–100%)').length).toBeGreaterThan(0);
  // цитату выделила модель (поле evidence.analyzer)
  expect(screen.getByText('цитату выделил: модель qwen2.5')).toBeInTheDocument();
});

test('карточка пары без agreement и actions (старый снимок): «нет данных», истории нет; анализатор цитаты — из комментариев', async () => {
  window.history.pushState(null, '', '/pairs/P2');
  wrap(<App />);
  const agree = await screen.findByRole('region', { name: 'Согласие сторон' });
  expect(within(agree).getByText(/Для расчёта нужны ответы на сопоставимые вопросы/)).toBeInTheDocument();
  expect(screen.queryByRole('region', { name: 'Что уже сделано и что изменилось' })).toBeNull();
  expect(screen.getAllByText('цитату выделил: словарь тем').length).toBeGreaterThan(0);
});

test('пороги R4: z_drop, decline_from_base, cost_ratio с подписями и диапазонами; decline_from_peak не показывается', async () => {
  const thr = { z_drop: 3, decline_slope: -0.25, decline_from_base: 0.5, cost_ratio: 10, decline_from_peak: 1, gap_min: 1 };
  patchResponses((url, _body, init) => (url.includes('/thresholds') && init?.method !== 'PUT' ? { thresholds: thr, defaults: thr } : undefined));
  window.history.pushState(null, '', '/thresholds');
  wrap(<App />);
  const cost = await screen.findByLabelText(/^Сколько лишних разговоров HR готов провести, чтобы не пропустить одного стажёра/);
  expect(cost).toHaveValue(10);
  expect(cost).toHaveAccessibleDescription(/для ручного выбора порогов/);
  expect(cost).toHaveAccessibleDescription(/в имитации при цене 10/i);
  expect(screen.queryByText(/вероятность проблемы от/)).not.toBeInTheDocument();
  const z = screen.getByLabelText(/^Резкий провал/);
  expect(z).toHaveValue(3);
  expect(screen.getByLabelText(/обычного уровня пары не меньше чем на/)).toHaveValue(0.5);
  expect(screen.getByLabelText(/^Устойчивое снижение: наклон за 4 месяца/)).toHaveValue(-0.25);
  expect(screen.queryByLabelText(/собственного максимума/)).toBeNull();
  // вне диапазона — ошибка у поля, сохранить нельзя
  await userEvent.clear(z);
  await userEvent.type(z, '7');
  expect(screen.getByRole('alert')).toHaveTextContent('Допустимо от 1 до 5');
  expect(z).toHaveAttribute('aria-invalid', 'true');
  const save = screen.getByRole('button', { name: 'Сохранить и пересчитать' });
  expect(save).toBeDisabled();
  await userEvent.clear(z);
  await userEvent.type(z, '2.5');
  await userEvent.clear(cost);
  await userEvent.type(cost, '0.2');
  expect(screen.getByRole('alert')).toHaveTextContent('Допустимо от 0,5 до 100');
  await userEvent.clear(cost);
  await userEvent.type(cost, '30');
  expect(save).toBeEnabled();
  await userEvent.click(save);
  await waitFor(() => expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true));
  expect(JSON.parse(String(calls.find((c) => c.init?.method === 'PUT')?.init?.body))).toMatchObject({ z_drop: 2.5, cost_ratio: 30, decline_from_base: 0.5 });
});
