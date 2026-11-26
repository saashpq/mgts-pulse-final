import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TextReview } from './TextReview';

test('очередь даёт доступ ко всем анкетам и конкретной роли, кроме пустой очереди', async () => {
  const rows = Array.from({ length: 5 }, (_, i) => ({ pair: `P${i}`, period: 3, role: 'mentor' as const, count: 1 }));
  const ui = render(<TextReview review={{ total: 9, unclassified: 5, rows }} />);
  expect(screen.getAllByRole('link')).toHaveLength(4);
  await userEvent.click(screen.getByRole('button', {name: 'Следующие'}));
  const link = screen.getByRole('link', {name: 'Пара P4 · M3 · Наставник'});
  expect(link).toHaveAttribute('href','/pairs/P4/months/3#month-mentor');
  await userEvent.click(link);
  expect(location.hash).toBe('#month-mentor');
  ui.rerender(<TextReview review={{ total: 0, unclassified: 0, rows: [] }} />);
  expect(screen.queryByRole('link')).toBeNull();
});

test('свежая критическая реплика доступна и при пустой очереди тем', async () => {
  render(<TextReview review={{ total: 1, unclassified: 0, rows: [],
    critical: [{ pair: 'R7', role: 'mentor', period: 4, question: 'm_other' }] }} />);
  await userEvent.click(screen.getByText(/Требуют чтения HR/));
  expect(screen.getByRole('link', {name: /R7.*M4.*Наставник/})).toHaveAttribute('href','/pairs/R7/months/4#comment-mentor-m_other');
});
