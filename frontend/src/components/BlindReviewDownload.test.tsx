import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { api, type PairDetail } from '../api';
import sample from '../../public/demo/pair-P2.json';
import { BlindReviewDownload, blindWorksheet } from './BlindReviewDownload';

const pair = sample as PairDetail;

test('бланк видит пары без тревог, сохраняет текст автора и оставляет графы для независимого решения пустыми', () => {
  const comments = [{ ...pair.comments[0], role: 'mentor' as const, period: 1, text: 'Ответ; "точно"\nна двух строках' }];
  const raw = { ...pair, comments, current: 1, questions: [], report: { ...pair.report, pair: '=test', headline: 'Скрытый вывод модели' } };
  const csv = blindWorksheet([raw]);
  expect(csv.charCodeAt(0)).toBe(0xFEFF);
  expect(csv).toContain('"\u0027=test";"1";"Нет ответов за месяц";"Комментарий — Ответ; ""точно""\nна двух строках";"";""\r\n');
  expect(csv).not.toContain('Скрытый вывод модели');
});

test('неполный набор анкет оставляет кнопку повторной загрузки', async () => {
  const fetch = vi.spyOn(api,'pair').mockResolvedValueOnce(pair).mockRejectedValueOnce(new Error('Нет связи'));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  try {
    render(<QueryClientProvider client={client}><BlindReviewDownload pairs={['P1','P2']} /></QueryClientProvider>);
    await userEvent.click(screen.getByRole('button', {name:'Скачать бланк для сверки'}));
    expect(await screen.findByRole('button',{name:'Скачать бланк ещё раз'})).toBeEnabled();
    expect(screen.getByRole('status')).toHaveTextContent('Не удалось собрать все анкеты');
  } finally {
    fetch.mockRestore();
    client.clear();
  }
});
