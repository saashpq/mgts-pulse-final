import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { PairDetail } from '../api';
import { match, pairMonthPath } from '../router';
import p2 from '../../public/demo/pair-P2.json';
import { MonthEvidence } from './MonthEvidence';

const sample = p2 as unknown as PairDetail;

test('месяц открывается по прямому адресу с произвольным ID пары', () => {
  const id = 'Отдел #5%';
  expect(match(pairMonthPath(id, 3))).toEqual({ view: 'pair', id, month: 3 });
  expect(match('/pairs/P2/months/0')).toEqual({ view: 'not-found' });
  expect(match('/pairs/P2/months/1.5')).toEqual({ view: 'not-found' });
});

test('за выбранный месяц выводятся вопросы, ответы и цитаты соответствующей стороны', async () => {
  const month = 3;
  render(<MonthEvidence detail={sample} month={month} />);
  for (const role of ['intern','mentor'] as const) {
    const region = screen.getByRole('region', { name: new RegExp(sample.report[role]) });
    const scope = within(region);
    const q = sample.questions.find(item => item.role === role)!;
    expect(scope.getByText(q.text).parentElement).toHaveTextContent(q.values[month - 1]!.toLocaleString('ru-RU'));
    for (const c of sample.comments.filter(item => item.role === role && item.period === month))
      expect(scope.getByText(c.text)).toBeVisible();
  }
  await userEvent.selectOptions(screen.getByRole('combobox', {name: 'Месяц'}), '5');
  expect(match(location.pathname)).toEqual({view:'pair', id:'P2', month:5});
});

test('пустая анкета и будущий месяц показывают свой статус, без соседних оценок', () => {
  const sparse: PairDetail = { ...sample, responses: [], current: 3, comments: [], questions: sample.questions.map(q => ({ ...q, values: [5, null, 2] })) };
  const ui = render(<MonthEvidence detail={sparse} month={2} />);
  expect(screen.getAllByText('Анкета за M2 не поступила.')).toHaveLength(2);
  ui.rerender(<MonthEvidence detail={sparse} month={4} />);
  expect(screen.getByText(/позже текущего отчётного периода/)).toBeVisible();
  expect(screen.queryByText(sample.questions[0].text)).toBeNull();
  ui.rerender(<MonthEvidence detail={sparse} month={99} />);
  expect(screen.getByText(/Выберите месяц из списка/)).toBeVisible();
});
