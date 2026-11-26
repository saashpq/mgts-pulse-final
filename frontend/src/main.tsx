import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from './App';
import { isOffline } from './api';
import { STALE_TIME } from './queries';
import '@fontsource-variable/golos-text';
import './styles.css';
import './typography.css';
import './studio.css';

// сервер недоступен (перезапуск) — повторяем дольше; прочие ошибки — один повтор
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: (n, e) => n < (isOffline(e) ? 5 : 1), staleTime: STALE_TIME, refetchOnWindowFocus: false },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
);
