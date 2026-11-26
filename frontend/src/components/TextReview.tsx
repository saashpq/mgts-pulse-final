import { useState } from 'react';
import type { MouseEvent } from 'react';
import type { Overview } from '../api';
import { ROLE_TEXT, plural } from '../format';
import { navigate, pairMonthPath } from '../router';
import './text-review.css';

const pageSize = 4;

function followComment(e: MouseEvent<HTMLAnchorElement>) {
  if (e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
    e.preventDefault();
    navigate(e.currentTarget.pathname + e.currentTarget.hash);
  }
}

export function TextReview({ review }: { review: NonNullable<Overview['text_review']> }) {
  const [page, setPage] = useState(0);
  if (!review.unclassified && !review.critical?.length) return null;
  const last = Math.ceil(review.rows.length / pageSize) - 1;
  const current = Math.min(page, last);
  return (
    <>
    {!!review.critical?.length && <details className="sheet critical-review">
      <summary>Требуют чтения HR ({review.critical.length})</summary>
      <p className="muted">За последние два отчётных месяца в этих ответах отмечены темы конфликта или ухода. Оценок для гибридного сигнала недостаточно. Прочитайте комментарий и решите, стоит ли связаться с участником.</p>
      <ul className="review-rows">
        {review.critical.map(r => {
          const target = pairMonthPath(r.pair, r.period) + '#comment-' + r.role + '-' + encodeURIComponent(r.question);
          return <li key={target}><a href={target} onClick={followComment}>Пара {r.pair} · M{r.period} · {ROLE_TEXT[r.role]}</a></li>;
        })}
      </ul>
    </details>}
    {!!review.unclassified && <section id="text-review" className="sheet" aria-labelledby="text-review-title">
      <h2 id="text-review-title">Комментарии без определённой темы</h2>
      <p className="muted">У {review.unclassified} из {review.total} комментариев тема не определена. Прочитайте исходный ответ, чтобы оценить, нужна ли участнику поддержка.</p>
      <ul className="review-rows">
        {review.rows.slice(current * pageSize, (current + 1) * pageSize).map(r => {
          const target = pairMonthPath(r.pair, r.period) + '#month-' + r.role;
          return <li key={target}><a href={target} onClick={followComment}>Пара {r.pair} · M{r.period} · {ROLE_TEXT[r.role]}</a>
            <small>{r.count} {plural(r.count, 'комментарий', 'комментария', 'комментариев')} без темы</small></li>;
        })}
      </ul>
      {last > 0 && <div className="review-pager">
        <button className="btn btn--ghost" disabled={current === 0} onClick={() => setPage(current - 1)}>Предыдущие</button>
        <span aria-live="polite">Страница {current + 1} из {last + 1}</span>
        <button className="btn btn--ghost" disabled={current === last} onClick={() => setPage(current + 1)}>Следующие</button>
      </div>}
    </section>}
    </>
  );
}
