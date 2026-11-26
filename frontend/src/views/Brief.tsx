import { useId, useState, useSyncExternalStore, type KeyboardEvent, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, isStaticDemo, type Brief } from '../api';
import { IconArrow } from '../components/icons';
import { ROLE_TEXT, STATUS_TEXT, commaNum, plural } from '../format';
import { actionForAudience, briefParts, contactDate, type BriefPart, type MeetingQuote, type MeetingRole } from '../meetingPlan';
import { navigate, pairDecisionPath, pairPath } from '../router';
import { Failure, Loading } from './common';
import './meeting-brief.css';

export { briefPoints, briefParts } from '../meetingPlan';

const genitive = { intern: 'стажёра', mentor: 'наставника' };
const withRole = { intern: 'со стажёром', mentor: 'с наставником' };

function LocalLink({ href, className, children }: { href: string; className?: string; children: ReactNode }) {
  return <a href={href} className={className} onClick={event => {
    if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    navigate(href);
  }}>{children}</a>;
}

function subscribeToRole(listener: () => void) {
  window.addEventListener('hashchange', listener);
  window.addEventListener('popstate', listener);
  return () => {
    window.removeEventListener('hashchange', listener);
    window.removeEventListener('popstate', listener);
  };
}

function selectRole(role: MeetingRole) {
  const hash = '#brief-' + role;
  if (window.location.hash === hash) return;
  window.history.pushState(null, '', hash);
  window.dispatchEvent(new PopStateEvent('popstate'));
}

function roleKeys(event: KeyboardEvent, index: number, parts: BriefPart[]) {
  let next = index;
  switch (event.key) {
    case 'ArrowLeft': next = (index - 1 + parts.length) % parts.length; break;
    case 'ArrowRight': next = (index + 1) % parts.length; break;
    case 'Home': next = 0; break;
    case 'End': next = parts.length - 1; break;
    default: return;
  }
  event.preventDefault();
  const role = parts[next].role;
  selectRole(role);
  document.getElementById('tab-' + role)?.focus();
}

function PrintIcon() {
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
    <path d="M7 9V3.5h10V9M7 16H4.5V9h15v7H17M7 14h10v6.5H7z" strokeLinejoin="round" />
    <circle cx="16.5" cy="11.5" r=".85" fill="currentColor" stroke="none" />
  </svg>;
}

function PrintButton() {
  return <button type="button" className="meeting-print" onClick={() => window.print()} title="Печать полного брифа для куратора">
    <PrintIcon /> Печать / PDF
  </button>;
}

export function BriefView({ id }: { id: string }) {
  const brief = useQuery({ queryKey: ['brief', id], queryFn: () => api.brief(id) });
  const source = useQuery({ queryKey: ['pair', id], queryFn: () => api.pair(id) });
  if (brief.isPending) return <Loading />;
  if (brief.isError) return <Failure error={brief.error} onRetry={() => brief.refetch()} />;
  return <BriefDocument key={id} brief={brief.data} parts={briefParts(brief.data, source.data)} />;
}

function BriefDocument({ brief: b, parts }: { brief: Brief; parts: BriefPart[] }) {
  const hash = useSyncExternalStore(subscribeToRole, () => window.location.hash);
  const active = parts.find(p => '#brief-' + p.role === hash)?.role ?? parts[0].role;
  const generated = Date.parse(b.generated_at);
  const demo = isStaticDemo();
  return <article className="meeting-document" aria-labelledby="brief-title">
    <header className="meeting-top">
      <div className="meeting-tools no-print">
        <LocalLink href={pairPath(b.pair)} className="meeting-back"><IconArrow /> Карточка пары {b.pair}</LocalLink>
        <PrintButton />
      </div>
      <p className="meeting-paper-only meeting-paper-title">Бриф к встрече 1:1 · Для куратора</p>
      <div className="meeting-heading">
        <h2 id="brief-title">Пара {b.pair}</h2>
        {STATUS_TEXT[b.status] && <span className={`meeting-status meeting-status--${b.status}`}>{STATUS_TEXT[b.status]}</span>}
      </div>
      <p className="meeting-privacy">
        <svg width="17" height="17" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
          <rect x="4.5" y="8.5" width="11" height="8" rx="1.8" /><path d="M7 8.5v-2a3 3 0 0 1 6 0v2m-3 3v2.5" />
        </svg>
        <span>Распечатка обеих встреч — для куратора. Каждую цитату обсуждайте только с её автором.</span>
      </p>
    </header>

    <div role="tablist" aria-label="С кем провести личную встречу" className="meeting-tabs no-print">
      {parts.map((part, index) => <button key={part.role} type="button" role="tab" id={'tab-' + part.role}
        aria-selected={active === part.role} aria-controls={'brief-' + part.role}
        tabIndex={active === part.role ? 0 : -1} onClick={() => selectRole(part.role)}
        onKeyDown={event => roleKeys(event, index, parts)} className="meeting-tab">
        <span className="meeting-tab__name">{ROLE_TEXT[part.role]} <strong>{b[part.role]}</strong></span>
        <span className="meeting-tab__when">{part.deadline && (part.deadline.at === undefined ? 'Срок — ' : 'Связаться ')}{contactDate(part.deadline)}</span>
      </button>)}
    </div>

    {parts.map(part => <div key={part.role} id={'brief-' + part.role} role="tabpanel" aria-labelledby={'tab-' + part.role}
      className="meeting-panel" hidden={active !== part.role} tabIndex={0}>
      <Conversation part={part} brief={b} demo={demo} />
    </div>)}

    <footer className="meeting-footer">
      {b.disclaimer && <p>{b.disclaimer}</p>}
      <p>{Number.isFinite(generated) ? <>Сформирован <time dateTime={b.generated_at}>{new Date(generated).toLocaleDateString('ru-RU', {
        day: 'numeric', month: 'long', year: 'numeric',
      })}</time></> : 'Дата формирования не указана'}{demo && ' · Данные синтетические'}</p>
    </footer>
  </article>;
}

function Conversation({ part, brief, demo }: { part: BriefPart; brief: Brief; demo: boolean }) {
  const { role, points, leadQuote, rows } = part;
  const point = !leadQuote ? part.scorePoint : undefined;
  const name = ('Разговор ' + withRole[role] + ' ' + (brief[role] || '')).trim();
  return <section aria-label={name} className="meeting-conversation">
    <div className="meeting-paper-only meeting-paper-heading">
      <h3>{name}</h3>
      <span>{part.deadline ? 'Срок контакта — ' : ''}{contactDate(part.deadline, true)}</span>
    </div>
    <div className="meeting-conversation__start">
      <div className="meeting-prompts">
        <h3 className="meeting-label">Вопрос для начала</h3>
        <p className="meeting-question">{part.questions[0]}</p>
        {part.questions.length > 1 && <Disclosure title={'Ещё ' + part.questions.slice(1).length + ' ' + plural(part.questions.length - 1, 'вопрос', 'вопроса', 'вопросов')} kind="questions">
          <ul className="meeting-questions">{part.questions.slice(1).map(q => <li key={q}>{q}</li>)}</ul>
        </Disclosure>}
      </div>
      {(leadQuote || point) && <aside className="meeting-source" aria-label="Основание для куратора">
        {leadQuote ? <>
          <span className="meeting-label">Из анкеты {genitive[role]}{leadQuote.period && ' · M' + leadQuote.period}</span>
          <blockquote>«{leadQuote.text}»</blockquote>
          {leadQuote.href && <LocalLink className="meeting-source-link" href={leadQuote.href}>Исходный ответ <IconArrow /></LocalLink>}
        </> : point && <>
          <span className="meeting-label">По анкете {genitive[role]}</span>
          <p className="meeting-source__point">{commaNum(point)}</p>
          {part.scoreSource && <LocalLink className="meeting-source-link" href={part.scoreSource.href}>Оценки за M{part.scoreSource.month} <IconArrow /></LocalLink>}
        </>}
      </aside>}
    </div>

    <div className="meeting-agreement">
      <h3>О чём договориться</h3>
      <p>{part.agreement}</p>
    </div>
    <div className="meeting-print-notes meeting-paper-only">
      <div>Следующий шаг <span /></div>
      <div>Кто отвечает <span /> Дата проверки <span /></div>
    </div>

    {(rows.length > 0 || points.some(p => p !== point)) ? <Disclosure kind="facts" title="Основания из анкет" count={rows.length}>
      {points.some(p => p !== point) && <ul className="meeting-points">
        {points.filter(p => p !== point).map(p => <li key={p}>{commaNum(p)}</li>)}
      </ul>}
      <ul className="meeting-evidence-list">
        {rows.map(({ signal, quotes, due, sameMeeting }) => <li key={signal.id ?? signal.type}>
          <h4>{signal.title}</h4>
          {signal.explanation && <p>{commaNum(signal.explanation)}</p>}
          {quotes.filter(q => signal !== part.leadSignal || q !== leadQuote).map((q, index) =>
            <Citation key={index} quote={q} role={role} />)}
          {sameMeeting ? <p className="meeting-evidence-action">Обсудите на той же встрече{part.deadline?.at !== undefined && ' ' + contactDate(part.deadline)}</p> : <>
            {signal.action && <p className="meeting-evidence-action"><strong>Возможное действие</strong> · {actionForAudience(signal)}</p>}
            {due && <p className="meeting-evidence-date">Срок реакции — {contactDate(due, true)}</p>}
          </>}
        </li>)}
      </ul>
    </Disclosure> : <p className="meeting-no-signals">Для этой встречи активных сигналов нет.</p>}

    <div className="meeting-outcome no-print">
      <div>
        <h3>После встречи</h3>
        <p>{!part.decision ? 'Для плановой встречи запишите договорённость на распечатке.' : demo
          ? 'В рабочей версии итог сохраняют в карточке сигнала. В онлайн-демо запишите договорённость на распечатке.'
          : 'В карточке сигнала заполните «Что сделано» и «Комментарий HR». Выберите решение, чтобы сохранить итог.'}</p>
      </div>
      {part.decision ? <LocalLink className="meeting-outcome-link" href={pairDecisionPath(brief.pair, part.decision.id)}>
        {demo ? 'Карточка сигнала' : 'К решению по сигналу'} <IconArrow />
      </LocalLink> : <PrintButton />}
    </div>
  </section>;
}

function Citation({ quote, role }: { quote: MeetingQuote; role: MeetingRole }) {
  return <div className="meeting-evidence-quote">
    <span>Анкета {genitive[role]}{quote.period && ' · M' + quote.period}</span>{' '}
    <q>{quote.text}</q>{' '}
    {quote.href && <LocalLink className="meeting-citation-link" href={quote.href}>Исходный ответ</LocalLink>}
  </div>;
}

function Disclosure({ title, kind, count, children }: {
  title: string; kind: 'questions' | 'facts'; count?: number; children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return <div className={'meeting-fold meeting-fold--' + kind}>
    <button className="meeting-fold__button no-print" type="button" aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}>
      <span>{title}</span>
      {count !== undefined && count > 0 && <small>{count} {plural(count, 'сигнал', 'сигнала', 'сигналов')}</small>}
      <span aria-hidden="true" className="meeting-chevron" />
    </button>
    <div id={id} className="meeting-fold__body" data-open={open}>
      {kind === 'facts' && <h3 className="meeting-paper-only">Основания из анкет</h3>}
      {children}
    </div>
  </div>;
}
