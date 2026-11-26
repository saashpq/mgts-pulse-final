import { match, navigate, pairDecisionPath, pairPath } from './router';

test('ссылка на решение сохраняет идентификаторы сигнала и пары, старый бриф ведёт к списку', () => {
  const url = new URL(pairDecisionPath('Пара #1', 'P 1#%'), 'https://demo.test');
  expect(match(url.pathname)).toEqual({view:'pair', id:'Пара #1'});
  expect(decodeURIComponent(url.hash.slice(1))).toBe('decision-P 1#%');
  expect(pairDecisionPath('P2')).toBe('/pairs/P2#sig-title');
});

test('возврат с чистого адреса восстанавливает якорь', async () => {
  history.replaceState(null, '', '/quality#q-ai');
  navigate('/quality');
  expect(location.hash).toBe('');

  const restored = new Promise<void>((resolve) => {
    window.addEventListener('popstate', () => resolve(), { once: true });
  });
  history.back();
  await restored;
  expect(location.pathname + location.hash).toBe('/quality#q-ai');
});

test('смена параметров создаёт запись истории, а повтор адреса с якорем сохраняет длину истории', () => {
  history.replaceState(null, '', '/quality?month=1#q-ai');
  const pushState = vi.spyOn(history, 'pushState');
  try {
    navigate('/quality?month=2#q-ai');
    navigate('/quality?month=2#q-ai');
    expect(location.search).toBe('?month=2');
    expect(pushState).toHaveBeenCalledTimes(1);
  } finally {
    pushState.mockRestore();
  }
});

test.each(['P2', 'Команда 2', 'Стажёр #1', 'Готово 50%', 'P%23'])('переходы пары %s сохраняют полный ID', (id) => {
  for (const section of [undefined, 'brief'] as const) {
    navigate(pairPath(id, section));
    expect(location.hash).toBe('');
    expect(match(location.pathname)).toEqual({ view: section ?? 'pair', id });
  }
});

test.each(['/pairs/%', '/pairs/%zz/brief', '/form/%E2'])('повреждённый адрес %s открывает 404', (path) => {
  expect(match(path)).toEqual({ view: 'not-found' });
});
