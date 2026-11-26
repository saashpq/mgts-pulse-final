import { useId, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { MIN_DECISIONS, api, isStaticDemo, type Signal, type SignalPrecision } from '../api';
import { VERDICT_KEYS, invalidate } from '../queries';
import { SignalFollowUp } from './FollowUps';
import { ROLE_TEXT, analyzerText, commaNum, SEVERITY_MARK, SEVERITY_TEXT, SOURCE_HINT, SOURCE_TEXT, VERDICT_TEXT, reactionDeadline, fmt, ratioCI } from '../format';

/** Подсказки для поля «что сделано»: последняя — «другое», ставит фокус в поле. */
export const ACTION_CHIPS = ['встреча 1:1 проведена', 'поговорили с наставником', 'сменили задачи'] as const;

/** Ветка срабатывания S1/S2 по тексту объяснения: «z = …» — резкий провал, «балла/мес» — устойчивое снижение. */
export function triggerBranch(s: Pick<Signal, 'type' | 'explanation'>): string {
  if (s.type !== 'S1' && s.type !== 'S2') return '';
  const z = /z\s*=\s*[−-]/.test(s.explanation);
  const slope = /балл\S*\s*\/\s*мес/.test(s.explanation);
  if (z && slope) return 'резкий провал и устойчивое снижение';
  if (z) return 'резкий провал к собственному уровню пары';
  if (slope) return 'устойчивое снижение ниже собственного уровня';
  return '';
}

/** Точность типа сигнала по решениям HR одной строкой. */
export function typePrecisionText(p: SignalPrecision | undefined): string {
  if (!p || p.precision === null) return '';
  const n = p.decisions ?? p.confirmed + p.resolved + p.false;
  if (p.enough === false) return `мало решений (${n} из ${MIN_DECISIONS})`;
  return ratioCI(p.confirmed + p.resolved, n, p.precision, p.lo, p.hi);
}
import { navigate, pairMonthPath, pairPath } from '../router';

/** Карточка сигнала: что случилось, почему (числа, пороги, цитаты), что делать и решение HR. */
export function SignalCard({
  s,
  showPair,
  active = true,
  who,
  analyzers,
}: {
  s: Signal;
  showPair?: boolean;
  active?: boolean;
  who?: string;
  /** кто разобрал комментарий: `${response_id}|${question}` → analyzer (из карточки пары) */
  analyzers?: Record<string, string>;
}) {
  const qc = useQueryClient();
  const [comment, setComment] = useState(s.verdict?.comment ?? '');
  const [action, setAction] = useState(s.verdict?.action ?? '');
  const actionRef = useRef<HTMLInputElement>(null);
  const uid = useId();
  // точность типа — из уже загруженного обзора (общий кэш), отдельного запроса нет
  const ov = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const precision = typePrecisionText(ov.data?.report.precision?.find((p) => p.type === s.type));
  const branch = triggerBranch(s);
  const m = useMutation({
    mutationFn: (status: string) => api.verdict(s.id, status, comment, status ? action.trim() : ''),
    onSuccess: () => invalidate(qc, VERDICT_KEYS),
  });
  const period = s.first_period === s.last_period ? `M${s.last_period}` : `M${s.first_period}–M${s.last_period}`;
  return (
    <article className={`sig sig--${s.severity}${s.verdict?.status === 'false' || !active ? ' sig--muted' : ''}`}>
      <header className="sig__head">
        <span className={`sev sev--${s.severity}`}>
          <span aria-hidden="true">{SEVERITY_MARK[s.severity]}</span> {SEVERITY_TEXT[s.severity]}
        </span>
        <h3 className="sig__title">{s.title}</h3>
        {s.primary && active && (
          <span className="sig__primary" title="Главный сигнал пары: остальные активные сигналы его подтверждают">
            Главное
          </span>
        )}
        {s.overdue && active && <span className="sig__overdue">Срок реакции прошёл</span>}
        {showPair && (
          <button type="button" className="btn btn--link" onClick={() => navigate(pairPath(s.pair))}>
            {s.pair}
            {who ? ` · ${who}` : ''}
          </button>
        )}
      </header>
      {active && (
        <p className="sig__action">
          <b>Что сделать:</b> {s.action} <span className="signal-reaction-time">{reactionDeadline(s).label}</span>
        </p>
      )}
      <p className="sig__why">{commaNum(s.explanation)}</p>
      {(branch || precision) && (
        <p className="sig__facts">
          {branch && (
            <span>
              <span className="muted">Сработало: </span>
              {branch}
            </span>
          )}
          {precision && (
            <span title="Доля сигналов этого типа, которые HR подтвердил или решил; в скобках — диапазон, где с 95% уверенностью лежит настоящая точность">
              <span className="muted">Точность {s.type} по решениям HR: </span>
              {precision}
            </span>
          )}
        </p>
      )}
      {s.evidence.length > 0 && (
        <ul className="sig__ev">
          {s.evidence.map((e, i) => (
            <li key={i}>
              <span className="muted">
                {ROLE_TEXT[e.role]}, M{e.period}
                {e.text && !e.quote ? ` · ${e.text}` : ''}:
              </span>{' '}
              {e.quote ? <q>{e.quote}</q> : <b>{fmt(e.value ?? null, 1)}</b>}
              {e.period > 0 && <a className="signal-source-link" href={pairMonthPath(s.pair, e.period) + (e.question ? '#' + (e.quote ? 'comment' : 'score') + '-' + e.role + '-' + encodeURIComponent(e.question) : '#month-' + e.role)}>Исходный ответ<span className="sr-only"> · {ROLE_TEXT[e.role]}, месяц {e.period}</span></a>}
              {e.quote && quoteAnalyzer(e, analyzers) && (
                <span className={`engine${quoteAnalyzer(e, analyzers).startsWith('llm') ? ' engine--llm' : ''} sig__engine`}>
                  цитату выделил: {analyzerText(quoteAnalyzer(e, analyzers))}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="sig__src">
        <span className={`src src--${s.source}`} title={SOURCE_HINT[s.source]}>
          <span className="sr-only">Источник: </span>
          {SOURCE_TEXT[s.source]}
        </span>
        <span className="sig__src-hint">{SOURCE_HINT[s.source]}</span>
        <button type="button" className="btn btn--link" onClick={() => navigate('/thresholds')}>
          Пороги
        </button>
      </div>
      <p className="sig__meta">
        {s.type} · {period} · впервые замечен в M{s.first_seen}
        {!active && ' · сейчас не срабатывает'}
      </p>
      <div className="sig__verdict" id={`decision-${s.id}`} tabIndex={-1} role="group" aria-label={`Решение HR по сигналу «${s.title}»`}>
        <p className="sig__decision-context">Решение по сигналу «{s.title}»</p>
        {s.verdict ? (
          <span className={`verdict verdict--${s.verdict.status}`}>
            {VERDICT_TEXT[s.verdict.status]}
            {s.verdict.period ? ` в M${s.verdict.period}` : ''}
            {s.verdict.comment ? `: ${s.verdict.comment}` : ''}
            {s.verdict.action ? ` · сделано: ${s.verdict.action}` : ''}
          </span>
        ) : (
          <span className="muted">Решение HR не принято</span>
        )}
        {isStaticDemo() ? <p className="muted">В рабочей версии здесь можно записать итог встречи и решение HR.</p> : <>
        <input
          className="sig__comment"
          placeholder="Комментарий HR"
          aria-label="Комментарий HR"
          maxLength={500}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
        />
        <div className="sig__done">
          <label htmlFor={`${uid}-action`} className="sig__done-label">
            Что сделано
          </label>
          <div className="chips sig__chips" role="group" aria-label="Подсказки: что сделано">
            {ACTION_CHIPS.map((c) => (
              <button key={c} type="button" className={`chip-btn${action === c ? ' chip-btn--on' : ''}`} aria-pressed={action === c} onClick={() => setAction(action === c ? '' : c)}>
                {c}
              </button>
            ))}
            <button
              type="button"
              className="chip-btn"
              onClick={() => {
                if ((ACTION_CHIPS as readonly string[]).includes(action)) setAction('');
                actionRef.current?.focus();
              }}
            >
              другое
            </button>
          </div>
          <input
            id={`${uid}-action`}
            ref={actionRef}
            className="sig__comment"
            placeholder="Например: встреча 1:1 проведена 18.11"
            maxLength={200}
            value={action}
            aria-describedby={`${uid}-action-hint`}
            onChange={(e) => setAction(e.target.value)}
          />
          <p id={`${uid}-action-hint`} className="muted">
            Сохраняется вместе с решением. Через месяц система покажет, как изменился индекс после действия.
          </p>
        </div>
        <div className="sig__buttons">
          <button type="button" className="btn btn--small btn--ghost" disabled={m.isPending} onClick={() => m.mutate('confirmed')}>
            Подтвердить
          </button>
          <button type="button" className="btn btn--small btn--ghost" disabled={m.isPending} onClick={() => m.mutate('resolved')}>
            Решено
          </button>
          <button type="button" className="btn btn--small btn--ghost" disabled={m.isPending} onClick={() => m.mutate('false')}>
            Ложный
          </button>
          {s.verdict && (
            <button type="button" className="btn btn--link" disabled={m.isPending} onClick={() => m.mutate('')}>
              Снять отметку
            </button>
          )}
        </div>
        {m.isError && <p className="error" role="alert">{(m.error as Error).message}</p>}
        </>}
      </div>
      <SignalFollowUp key={s.id + '/' + (s.episode ?? s.first_seen)} signal={s} active={active} />
    </article>
  );
}

function quoteAnalyzer(e: Signal['evidence'][number], map?: Record<string, string>): string {
  return e.analyzer ?? map?.[`${e.response_id}|${e.question ?? ''}`] ?? '';
}
