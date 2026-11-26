import type { PairReport, Signal } from '../api';
import { reactionDeadline, ROLE_TEXT, SEVERITY_RANK, SEVERITY_TEXT } from '../format';
import { navigate, pairMonthPath, pairPath } from '../router';
import './priority-case.css';

function criticalEvidencePeriod(signal: Signal) {
  return signal.type === 'S10' && signal.evidence[0]?.quote?.trim() ? signal.evidence[0].period : 0;
}

function compareSignals(a: Signal, b: Signal, current: number) {
  return SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]
    || Number(criticalEvidencePeriod(b) === current) - Number(criticalEvidencePeriod(a) === current)
    || Number(Boolean(b.overdue)) - Number(Boolean(a.overdue))
    || Number(a.verdict?.status === 'confirmed') - Number(b.verdict?.status === 'confirmed')
    || reactionDeadline(a).at - reactionDeadline(b).at
    || Number(Boolean(b.primary)) - Number(Boolean(a.primary))
    || a.due_days - b.due_days
    || a.id.localeCompare(b.id, 'ru');
}

export function PriorityCase({ pairs, current }: { pairs: PairReport[]; current: number }) {
  const cases = pairs.flatMap(pair => pair.signals
    .filter(s => s.severity !== 'low' && s.verdict?.status !== 'resolved' && s.verdict?.status !== 'false')
    .map(signal => ({ pair, signal })));
  const first = cases.sort((a, b) => compareSignals(a.signal, b.signal, current))[0];
  if (!first) return null;
  const { pair, signal } = first;
  const confirmed = signal.verdict?.status === 'confirmed';
  const deadline = reactionDeadline(signal);
  // For S10 the first item is the critical comment. Other items corroborate it.
  const quote = signal.type === 'S10' ? signal.evidence[0] : signal.evidence.filter(e => e.quote?.trim()).sort((a, b) => b.period - a.period)[0];
  const open = (section?: 'brief') => navigate(pairPath(pair.pair, section));
  const sourcePath = quote && pairMonthPath(pair.pair, quote.period) +
    (quote.question ? '#comment-' + quote.role + '-' + encodeURIComponent(quote.question) : '#month-' + quote.role);
  return (
    <section className="priority-case" aria-label="Приоритетная пара">
      <div className="priority-case__decision">
        <p className="priority-case__context"><span>{SEVERITY_TEXT[signal.severity]}</span> Пара {pair.pair} · {pair.intern} / {pair.mentor}</p>
        <h2>{signal.title}</h2>
        <p className="priority-case__action">{signal.action}</p>
        <p className="priority-case__due">{signal.overdue && <strong>Срок прошёл · </strong>}{Number.isFinite(deadline.at) && 'Срок реакции '}{deadline.label}</p>
        {confirmed && <p className="priority-case__confirmation">HR подтвердил сигнал{signal.verdict?.action ? ` · ${signal.verdict.action}` : ''}</p>}
        <div className="priority-case__links">
          <button className="btn priority-case__open" onClick={() => open()}>{confirmed ? 'Проверить действие по паре' : 'Разобрать пару'} {pair.pair} <span aria-hidden="true">→</span></button>
          <button className="btn priority-case__brief" onClick={() => open('brief')}>Бриф для встречи</button>
        </div>
      </div>
      <div className="priority-case__basis">
        {quote?.quote ? <>
          <p className="priority-case__source">{ROLE_TEXT[quote.role]} · M{quote.period}</p>
          <blockquote>«{quote.quote}»</blockquote>
          <a className="priority-case__original" href={sourcePath} onClick={event => {
            if (sourcePath && event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) {
              event.preventDefault();
              navigate(sourcePath);
            }
          }}>Открыть исходный ответ <span aria-hidden="true">→</span></a>
        </> : <><p className="priority-case__source">Основание сигнала</p><p>{signal.explanation}</p></>}
      </div>
    </section>
  );
}
