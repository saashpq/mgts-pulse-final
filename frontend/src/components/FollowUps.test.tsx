import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { ApiError, api, type FollowUp, type FollowUpCommand, type Signal } from '../api';
import { FollowUpQueue, SignalFollowUp } from './FollowUps';

const today = '2026-11-26';
const signal: Signal = {
  id: 'P2-S7', pair: 'P2', title: 'Снижение интереса', type: 'S7', severity: 'high', source: 'rules',
  first_period: 1, last_period: 6, first_seen: 2, episode: 5, due_days: 3, explanation: '', action: '', evidence: [],
};
const plan: FollowUp = {
  signal_id: signal.id, pair: signal.pair, title: signal.title, episode: 5,
  status: 'open', timing: 'today', revision: 1, due_date: today, agreed_action: 'Разобрать одну задачу вместе', owner: 'HR', checks: [],
};
function mount(children: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
  return client;
}
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllEnvs(); });

it('plans the current episode from a signal, then records execution and participant feedback independently', async () => {
  let stored: FollowUp | undefined;
  vi.spyOn(api, 'followUps').mockImplementation(async () => ({ today, items: stored ? [stored] : [] }));
  const update = vi.spyOn(api, 'updateFollowUp').mockImplementation(async (_signal, _episode, command) => {
    if (command.operation === 'plan') stored = plan;
    else stored = { ...plan, status: 'closed', timing: 'closed', revision: 2, checks: [{ at: today + 'T11:00:00Z',
      due_date: today, owner: plan.owner, agreed_action: plan.agreed_action, delivery: command.delivery, outcome: command.outcome, note: command.note }] };
    return stored;
  });
  mount(<><SignalFollowUp signal={signal} active /><FollowUpQueue /></>);
  const queue = within(screen.getByRole('region', { name: 'Повторные контакты' }));
  expect(await queue.findByText(/Повторных контактов в плане нет/)).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Договорённость и повторный контакт' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Назначить повторный контакт' }));
  fireEvent.change(screen.getByLabelText('О чём договорились'), { target: { value: plan.agreed_action } });
  fireEvent.change(screen.getByLabelText('Дата повторного контакта'), { target: { value: today } });
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить договорённость' }));
  expect(await queue.findByText(plan.agreed_action)).toBeVisible();
  expect(update).toHaveBeenCalledWith(signal.id, 5, { operation: 'plan', revision: 0, due_date: today, agreed_action: plan.agreed_action, owner: 'HR' });
  fireEvent.click(queue.getByRole('button', { name: 'Записать результат' }));
  expect(queue.getByLabelText('Что сказал участник')).toHaveValue('');
  fireEvent.change(queue.getByLabelText('Выполнение договорённости'), { target: { value: 'done' } });
  fireEvent.change(queue.getByLabelText('Что сказал участник'), { target: { value: 'no_change' } });
  expect(queue.getByRole('button', { name: 'Сохранить результат' })).toBeDisabled();
  fireEvent.click(queue.getByRole('radio', { name: 'Завершить без следующего контакта' }));
  fireEvent.click(queue.getByRole('button', { name: 'Сохранить результат' }));
  expect(await queue.findByText(/Результат записан/)).toHaveTextContent('Контроль завершён');
  fireEvent.click(queue.getByText('Завершённые контакты (1)'));
  expect(queue.getByText(/Последний контакт/)).toHaveTextContent('Действие — выполнено. Ответ участника — без улучшения');
  expect(queue.queryByRole('button', { name: 'Записать результат' })).not.toBeInTheDocument();
});

it('keeps an unanswered contact open when a new date is requested', async () => {
  let stored = plan;
  vi.spyOn(api, 'followUps').mockImplementation(async () => ({ today, items: [stored] }));
  const update = vi.spyOn(api, 'updateFollowUp').mockImplementation(async () => {
    stored = { ...plan, due_date: '2026-12-02', timing: 'upcoming', revision: 2 };
    return stored;
  });
  mount(<FollowUpQueue />);
  fireEvent.click(await screen.findByRole('button', { name: 'Записать результат' }));
  fireEvent.change(screen.getByLabelText('Выполнение договорённости'), { target: { value: 'unknown' } });
  fireEvent.change(screen.getByLabelText('Что сказал участник'), { target: { value: 'unknown' } });
  expect(screen.getByRole('button', { name: 'Сохранить результат' })).toBeDisabled();
  expect(screen.getByLabelText('Назначить ещё один контакт')).not.toBeChecked();
  fireEvent.click(screen.getByLabelText('Назначить ещё один контакт'));
  expect(screen.getByLabelText('Дата следующего контакта')).toHaveAttribute('min', today);
  fireEvent.change(screen.getByLabelText('Дата следующего контакта'), { target: { value: '2026-12-02' } });
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить результат' }));
  await waitFor(() => expect(update).toHaveBeenCalledWith(signal.id, 5, {
    operation: 'check', revision: 1, delivery: 'unknown', outcome: 'unknown', note: '', next_due_date: '2026-12-02',
  } satisfies FollowUpCommand));
  expect(await screen.findByText('Запланирован')).toBeVisible();
});

it('shows read errors separately from empty and recovers on retry', async () => {
  vi.spyOn(api, 'followUps').mockRejectedValueOnce(new Error('off')).mockResolvedValue({ today, items: [] });
  mount(<FollowUpQueue />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось загрузить');
  expect(screen.queryByText(/Повторных контактов в плане нет/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Повторить загрузку' }));
  expect(await screen.findByText(/Повторных контактов в плане нет/)).toBeVisible();
});

it.each(['queue', 'signal'])('keeps the %s draft and marks stale data on a background read error', async (surface) => {
  const read = vi.spyOn(api, 'followUps').mockResolvedValue({ today, items: [plan] });
  const client = mount(surface === 'queue' ? <FollowUpQueue /> : <SignalFollowUp signal={signal} active />);
  if (surface === 'signal') fireEvent.click(screen.getByRole('button', { name: 'Договорённость и повторный контакт' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Изменить договорённость' }));
  fireEvent.change(screen.getByLabelText('О чём договорились'), { target: { value: 'Несохранённый план' } });
  read.mockRejectedValueOnce(new Error('offline'));
  await act(async () => { await client.invalidateQueries({ queryKey: ['follow-ups'] }); });
  expect(await screen.findByRole('alert')).toHaveTextContent('последние загруженные данные');
  expect(screen.getByLabelText('О чём договорились')).toHaveValue('Несохранённый план');
  fireEvent.click(screen.getByRole('button', { name: 'Повторить загрузку' }));
  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
  expect(screen.getByLabelText('О чём договорились')).toHaveValue('Несохранённый план');
});

it('keeps a stale draft from overwriting a concurrent update', async () => {
  let stored = plan;
  vi.spyOn(api, 'followUps').mockImplementation(async () => ({ today, items: [stored] }));
  const update = vi.spyOn(api, 'updateFollowUp').mockImplementation(async () => {
    stored = { ...plan, revision: 2, agreed_action: 'Уточнённое действие' };
    throw new ApiError(409, 'изменено');
  });
  mount(<FollowUpQueue />);
  fireEvent.click(await screen.findByRole('button', { name: 'Изменить договорённость' }));
  fireEvent.change(screen.getByLabelText('О чём договорились'), { target: { value: 'Мой черновик' } });
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить договорённость' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('уже изменилась');
  expect(screen.queryByRole('button', { name: 'Сохранить договорённость' })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Закрыть форму' }));
  fireEvent.click(screen.getByRole('button', { name: 'Изменить договорённость' }));
  expect(screen.getByLabelText('О чём договорились')).toHaveValue('Уточнённое действие');
  expect(update).toHaveBeenCalledTimes(1);
});

it('allows a static visitor to inspect a synthetic case without write controls', async () => {
  vi.stubEnv('VITE_STATIC_DEMO', 'true');
  const mockFetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ report: { pairs: [{ signals: [signal] }] } })));
  mount(<FollowUpQueue />);
  expect(await screen.findByText(/Стажёр и наставник сверят ожидания/)).toBeVisible();
  expect(screen.getByText(/Демонстрационный сценарий доступен для просмотра/)).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Записать результат' })).not.toBeInTheDocument();
  expect(mockFetch).toHaveBeenCalledWith('/demo/overview.json');
  await expect(api.updateFollowUp(signal.id, 5, { operation: 'plan', revision: 0, agreed_action: 'test', owner: 'HR', due_date: today })).rejects.toMatchObject({ status: 403 });
  expect(mockFetch).toHaveBeenCalledTimes(1);
});
