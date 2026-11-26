import { useEffect, useRef, useState } from 'react';
import { isOffline } from '../api';

export const Loading = () => <p role="status" className="sheet sheet--state">Загружаем…</p>;

/** Ошибка загрузки экрана с кнопкой повтора. Если сервер недоступен — запросы повторяются и сами. */
export const Failure = ({ error, onRetry }: { error: Error; onRetry?: () => void }) => (
  <div className="sheet sheet--state failure" role="alert">
    <p className={isOffline(error) ? undefined : 'error'}>{error.message}</p>
    {onRetry && (
      <button type="button" className="btn btn--ghost btn--small" onClick={onRetry}>
        Повторить
      </button>
    )}
  </div>
);

/** Кнопка копирования с объявлением результата для скринридера. */
export function CopyButton({ text, label, className = 'btn btn--small btn--ghost' }: { text: string; label: string; className?: string }) {
  const [status, setStatus] = useState<'' | 'ok' | 'fail'>('');
  const timer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    let ok = false;
    try {
      await navigator.clipboard.writeText(text);
      ok = true;
    } catch {
      ok = false;
    }
    setStatus(ok ? 'ok' : 'fail');
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setStatus(''), 4000);
  };
  return (
    <span className="copy">
      <button type="button" className={className} onClick={copy}>
        {label}
      </button>
      <span className={`copy__status${status === 'fail' ? ' copy__status--fail' : ''}`} role="status" aria-live="polite">
        {status === 'ok' ? 'Скопировано' : status === 'fail' ? 'Не удалось скопировать' : ''}
      </span>
    </span>
  );
}
