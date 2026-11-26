import { useEffect } from 'react';
import { isStaticDemo, type PairDetail } from '../api';
import { ROLE_TEXT } from '../format';
import { navigate, pairMonthPath, pairPath } from '../router';
import './month-evidence.css';

export function MonthEvidence({ detail, month }: { detail: PairDetail; month: number }) {
  const { report, periods, current, questions, comments } = detail;
  const available = month <= periods;
  const future = month > current;
  useEffect(() => {
    let answer: HTMLElement | null = null;
    const focusLinkedAnswer = () => {
      answer?.removeAttribute('data-linked-answer');
      const id = window.location.hash.slice(1);
      answer = document.getElementById(id);
      if (!answer?.matches('.month-evidence__role [tabindex="-1"]')) return;
      // The API may render the answer after the browser has resolved the fragment.
      answer.setAttribute('data-linked-answer', '');
      answer.focus();
    };
    focusLinkedAnswer();
    window.addEventListener('hashchange', focusLinkedAnswer);
    return () => {
      window.removeEventListener('hashchange', focusLinkedAnswer);
      answer?.removeAttribute('data-linked-answer');
    };
  }, [month, report.pair, available, future]);
  return (
    <>
      <div className="pair-route">
        <button type="button" className="back-link btn btn--link" onClick={() => navigate(pairPath(report.pair))}>← Карточка пары {report.pair}</button>
        <button type="button" className="btn btn--ghost btn--small" onClick={() => navigate(pairPath(report.pair, 'brief'))}>Бриф к встрече 1:1</button>
      </div>
      <section className="sheet month-evidence" aria-labelledby="month-title">
        <div className="sheet__head">
          <h2 id="month-title">Ответы пары {report.pair} за M{month}</h2>
          <div className="month-controls">
          <button type="button" aria-label="Предыдущий месяц" disabled={!available || month <= 1} onClick={() => navigate(pairMonthPath(report.pair, month - 1))}>‹</button>
          <label className="month-picker">Месяц
            <select value={available ? month : ''} onChange={e => navigate(pairMonthPath(report.pair, Number(e.target.value)))}>
              {!available && <option value="" disabled>Выберите месяц</option>}
              {Array.from({ length: periods }, (_, i) => <option key={i} value={i + 1}>M{i + 1}</option>)}
            </select>
          </label>
          <button type="button" aria-label="Следующий месяц" disabled={!available || month >= periods} onClick={() => navigate(pairMonthPath(report.pair, month + 1))}>›</button>
          </div>
        </div>
        {!available ? <p>В программе {periods} мес. Выберите месяц из списка.</p> :
          future ? <p>Этот месяц позже текущего отчётного периода. Ответы за него пока не показаны.</p> :
          <div className="month-evidence__roles">
            {(['intern', 'mentor'] as const).map(role => {
              const scores = questions.filter(q => q.role === role);
              const indexValue = report[role === 'intern' ? 'intern_index' : 'mentor_index']?.[month - 1];
              const quotes = comments.filter(c => c.role === role && c.period === month);
              const hasData = scores.some(q => q.values[month - 1] != null) || quotes.length > 0;
              return (
                <section className={`month-evidence__role month-evidence__role--${role}`} key={role} aria-labelledby={`month-${role}`}>
                  <h3 id={`month-${role}`} tabIndex={-1}>{ROLE_TEXT[role]} {report[role]}</h3>
                  {hasData && <p className="muted month-perspective">{role === 'intern' ? 'Как стажёр оценивает свою стажировку' : 'Как наставник оценивает работу стажёра'} · шкала 1–5</p>}
                  {hasData && indexValue != null && <p id={`index-${role}`} className="month-score" tabIndex={-1}>
                    Сводная оценка за M{month} — <strong>{indexValue.toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</strong>
                  </p>}
                  {!hasData ? <p className="muted">За M{month} нет оценок и комментариев.</p> : <>
                    {isStaticDemo() && scores.some(q => q.values[month - 1] != null && !Number.isInteger(q.values[month - 1])) && <p className="muted">В учебном наборе есть дробные оценки. В персональной анкете участник выбирает целое число от 1 до 5.</p>}
                    <dl className="month-scores">
                      {scores.map(q => <div key={q.code} id={`score-${role}-${encodeURIComponent(q.code)}`} tabIndex={-1}>
                        <dt>{q.text}</dt>
                        <dd>{q.values[month - 1]?.toLocaleString('ru-RU') ?? 'Нет ответа'}</dd>
                      </div>)}
                    </dl>
                    {quotes.length > 0 ? <>
                      <h4>Из анкеты</h4>
                      {quotes.map(c => <div key={c.response_id + '/' + c.question}>
                        <blockquote id={`comment-${role}-${encodeURIComponent(c.question)}`} tabIndex={-1}>{c.text}</blockquote>
                        {!c.analysis?.themes?.length && <small className="muted">Тема не определена</small>}
                      </div>)}
                    </> : <p className="muted">Комментарий отсутствует.</p>}
                  </>}
                </section>
              );
            })}
          </div>}
      </section>
    </>
  );
}
