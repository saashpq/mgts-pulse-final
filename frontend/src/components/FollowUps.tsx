import { useId, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, isStaticDemo, type ContactOutcome, type Delivery, type FollowUp, type FollowUpCommand, type FollowUpQueueData, type Signal } from '../api';
import { navigate, pairPath } from '../router';
import './FollowUps.css';

const key = ['follow-ups'] as const;
const timing = { overdue: 'Срок прошёл', today: 'Сегодня', upcoming: 'Запланирован', closed: 'Контроль завершён' };
const deliveryLabels: Record<Delivery, string> = {
  done: 'Выполнено', partial: 'Выполнено частично', not_done: 'Не выполнено', unknown: 'Пока неизвестно',
};
const outcomeLabels: Record<ContactOutcome, string> = {
  done: 'Помогло', partial: 'Частично помогло', no_change: 'Без улучшения', unknown: 'Ответа пока нет',
};
const dateFormat = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', day: 'numeric', month: 'short', year: 'numeric' });
function formatDate(date: string) {
  return dateFormat.format(new Date(date.length === 10 ? date + 'T12:00:00Z' : date));
}
function useFollowUps(enabled = true) {
  return useQuery({ queryKey: key, queryFn: api.followUps, enabled, refetchInterval: isStaticDemo() ? false : 60_000 });
}
function QueueError({ retry, stale }: { retry: () => void; stale: boolean }) {
  return <div className="fu-error" role="alert">{stale ? 'Не удалось обновить договорённости. Показаны последние загруженные данные.' : 'Не удалось загрузить договорённости.'} <button className="btn btn--link" type="button" onClick={retry}>Повторить загрузку</button></div>;
}

export function FollowUpQueue() {
  const q = useFollowUps();
  const title = useId();
  const [notice, setNotice] = useState('');
  const open = q.data?.items.filter(item => item.status === 'open') ?? [];
  const closed = q.data?.items.filter(item => item.status === 'closed') ?? [];
  function renderItem(item: FollowUp) {
    return <li className="fu-row" key={item.signal_id + '/' + item.episode}>
      <div className="fu-row__title">
        <button type="button" className="btn btn--link" onClick={() => navigate(pairPath(item.pair))}>Пара {item.pair}</button>
        <strong>{item.title}</strong><span className="muted">Эпизод с М{item.episode}</span>
      </div>
      <RecordView item={item} signal={item.signal_id} episode={item.episode} today={q.data!.today} notify={setNotice} />
    </li>;
  }
  return <section className="sheet fu-queue" aria-labelledby={title}>
    <div className="sheet__head"><h2 id={title}>Повторные контакты</h2>
      {q.data && <span className="sheet__meta">{isStaticDemo() ? 'Учебный пример' : 'На контроле'} · {isStaticDemo() ? formatDate(q.data.today) : open.length}</span>}
    </div>
    {isStaticDemo() && <p className="muted">Демонстрационный сценарий доступен для просмотра. Запись договорённостей работает в полной версии.</p>}
    {q.isPending && <p className="muted">Загружаем договорённости…</p>}
    {q.isError && <QueueError stale={Boolean(q.data)} retry={() => void q.refetch()} />}
    {q.data && <>
      {open.length ? <ul className="fu-list">{open.map(renderItem)}</ul> :
        <p className="muted">Повторных контактов в плане нет. После встречи откройте сигнал и укажите, когда проверить договорённость.</p>}
      {closed.length > 0 && <details className="fu-history"><summary>Завершённые контакты ({closed.length})</summary><ul className="fu-list">{closed.map(renderItem)}</ul></details>}
    </>}
    <div className="fu-notice" aria-live="polite">{notice}</div>
  </section>;
}

export function SignalFollowUp({ signal, active }: { signal: Signal; active: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [notice, setNotice] = useState('');
  const panel = useId();
  const q = useFollowUps(expanded);
  const episode = signal.episode ?? signal.first_seen;
  const item = q.data?.items.find(i => i.signal_id === signal.id && i.episode === episode);
  return <div className="fu-signal">
    <button type="button" className="btn btn--ghost" aria-expanded={expanded} aria-controls={panel} onClick={() => setExpanded(!expanded)}>
      Договорённость и повторный контакт
    </button>
    <div id={panel} hidden={!expanded}>{expanded && <>
      {q.isPending && <p>Загружаем договорённость…</p>}
      {q.isError && <QueueError stale={Boolean(q.data)} retry={() => void q.refetch()} />}
      {q.data && <>
        {!item && <p className="muted">{active ? 'Для этого эпизода пока нет договорённости.' : 'Для этого прошедшего эпизода договорённость не записана.'}</p>}
        {isStaticDemo() && <p className="muted">Демонстрационный сценарий · только просмотр.</p>}
        <RecordView item={item} signal={signal.id} episode={episode} today={q.data.today} notify={setNotice} />
      </>}
    </>}</div>
    <div className="fu-notice" aria-live="polite">{notice}</div>
  </div>;
}

type RecordProps = { item?: FollowUp; signal: string; episode: number; today: string; notify: (message: string) => void };
type EditorMode = 'plan' | 'check';
function RecordView({ item, signal, episode, today, notify }: RecordProps) {
  const [editing, setEditing] = useState<EditorMode | null>(null);
  const history = item?.checks ?? [];
  const moves = item?.reschedules ?? [];
  const recent = history[history.length - 1];
  function open(mode: EditorMode) {
    notify('');
    setEditing(mode);
  }
  function saved(updated: FollowUp) {
    setEditing(null);
    const message = editing === 'plan' ? 'Договорённость сохранена.' : 'Результат записан.';
    notify(message + (updated.status === 'closed' ? ' Контроль завершён.' : ' Следующий контакт ' + formatDate(updated.due_date) + '.'));
  }
  return <div className="fu-record">
    {item && <>
      <p className="fu-action">{item.agreed_action}</p>
      <div className="fu-meta"><span className={'fu-badge fu-badge--' + item.timing}>{timing[item.timing]}</span>
        <span>Срок <time dateTime={item.due_date}>{formatDate(item.due_date)}</time></span><span>Ответственный · {item.owner}</span>
      </div>
      {recent && <div className="fu-outcome">Последний контакт · {formatDate(recent.at)}. Действие — {deliveryLabels[recent.delivery].toLowerCase()}. Ответ участника — {outcomeLabels[recent.outcome].toLowerCase()}.</div>}
      {moves.length > 0 && <details className="fu-history"><summary>Переносы срока ({moves.length})</summary>
        <ol>{[...moves].reverse().map((move, i) => <li key={moves.length - i}>
          {formatDate(move.at)} перенесли с {formatDate(move.due_date)} на {formatDate(move.next_due_date)}.
        </li>)}</ol>
      </details>}
      {history.length > 0 && <details className="fu-history"><summary>История контактов ({history.length})</summary>
        <ol>{[...history].reverse().map((check, i) => <li key={history.length - i}><strong>{formatDate(check.at)}</strong>
          <div>{check.agreed_action} · {check.owner} · срок {formatDate(check.due_date)}</div>
          <div>Действие — {deliveryLabels[check.delivery].toLowerCase()}. Ответ участника — {outcomeLabels[check.outcome].toLowerCase()}.</div>
          {check.note && <p>{check.note}</p>}
        </li>)}</ol>
      </details>}
    </>}
    {editing && !isStaticDemo() ? <FollowUpEditor mode={editing} item={item} signal={signal} episode={episode} today={today}
      onCancel={() => setEditing(null)} onSaved={saved} /> : !isStaticDemo() && item?.status !== 'closed' &&
      <div className="fu-controls">
        {item && <button type="button" className="btn btn--primary" onClick={() => open('check')}>Записать результат</button>}
        <button type="button" className={'btn ' + (item ? 'btn--ghost' : 'btn--primary')} onClick={() => open('plan')}>
          {item ? 'Изменить договорённость' : 'Назначить повторный контакт'}
        </button>
      </div>}
  </div>;
}

type EditorProps = Omit<RecordProps, 'notify'> & { mode: EditorMode; onCancel: () => void; onSaved: (updated: FollowUp) => void };
function FollowUpEditor({ mode, item, signal, episode, today, onCancel, onSaved }: EditorProps) {
  const client = useQueryClient();
  // The draft retains its original revision when another HR colleague updates the case.
  const [revision] = useState(item?.revision ?? 0);
  const [action, setAction] = useState(item?.agreed_action ?? '');
  const [owner, setOwner] = useState(item?.owner ?? 'HR');
  const [date, setDate] = useState(item?.due_date ?? '');
  const [delivery, setDelivery] = useState<Delivery | ''>('');
  const [outcome, setOutcome] = useState<ContactOutcome | ''>('');
  const [note, setNote] = useState('');
  const [nextStep, setNextStep] = useState<'' | 'continue' | 'close'>('');
  const stepName = useId();
  const [nextDate, setNextDate] = useState('');
  const sending = useRef(false);
  const m = useMutation({
    mutationFn: (command: FollowUpCommand) => api.updateFollowUp(signal, episode, command),
    onSuccess: (updated) => {
      client.setQueryData<FollowUpQueueData>(key, current => current && {
        ...current, items: [...current.items.filter(i => i.signal_id !== signal || i.episode !== episode), updated]
          .sort((a, b) => a.due_date.localeCompare(b.due_date) || a.signal_id.localeCompare(b.signal_id) || a.episode - b.episode),
      });
      onSaved(updated);
      void client.invalidateQueries({ queryKey: key });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) void client.invalidateQueries({ queryKey: key });
    },
    onSettled: () => { sending.current = false; },
  });
  const conflict = (item?.revision ?? 0) !== revision || (m.error instanceof ApiError && m.error.status === 409);
  function submit(event: FormEvent) {
    event.preventDefault();
    if (sending.current || conflict) return;
    let command: FollowUpCommand;
    if (mode === 'plan') {
      if (!action.trim() || !date) return;
      command = { operation: 'plan', revision, agreed_action: action.trim(), owner: owner.trim() || 'HR', due_date: date };
    } else {
      if (!delivery || !outcome || (outcome !== 'done' && !nextStep) || (nextStep === 'continue' && !nextDate)) return;
      command = { operation: 'check', revision, delivery, outcome, note: note.trim(), ...(nextStep === 'continue' ? { next_due_date: nextDate } : {}) };
    }
    sending.current = true;
    m.mutate(command);
  }
  return <form className="fu-form" onSubmit={submit} aria-label={mode === 'plan' ? 'План повторного контакта' : 'Итог повторного контакта'}>
    <fieldset disabled={m.isPending || conflict}><legend>{mode === 'plan' ? 'План повторного контакта' : 'Итог повторного контакта'}</legend>
      {mode === 'plan' ? <>
        <label>О чём договорились<textarea autoFocus required maxLength={500} value={action} onChange={e => setAction(e.target.value)} placeholder="Например, наставник покажет на примере, как выглядит готовая задача" /></label>
        <div className="fu-fields"><label>Ответственный<input maxLength={100} value={owner} onChange={e => setOwner(e.target.value)} /></label>
          <label>Дата повторного контакта<input required type="date" value={date} onChange={e => setDate(e.target.value)} /></label></div>
      </> : <>
        <div className="fu-fields">
          <label>Выполнение договорённости<select autoFocus required value={delivery} onChange={e => setDelivery(e.target.value as Delivery)}>
            <option value="">Выберите</option>{Object.entries(deliveryLabels).map(([v, label]) => <option key={v} value={v}>{label}</option>)}
          </select></label>
          <label>Что сказал участник<select required value={outcome} onChange={e => { setOutcome(e.target.value as ContactOutcome); setNextStep(''); }}>
            <option value="">Выберите</option>{Object.entries(outcomeLabels).map(([v, label]) => <option key={v} value={v}>{label}</option>)}
          </select></label>
        </div>
        {outcome === 'partial' && <p className="muted">Спросите участника: «Что стало проще и на какой задаче трудность осталась?»</p>}
        {outcome === 'no_change' && <p className="muted">Спросите участника: «На каком шаге вы в последний раз застряли?»</p>}
        <label>Заметка HR (необязательно)<textarea maxLength={500} value={note} onChange={e => setNote(e.target.value)} /></label>
        {outcome && outcome !== 'done' && <fieldset className="fu-next-step"><legend>Что делать дальше?</legend>
          <label className="fu-toggle"><input type="radio" name={stepName} value="continue" required checked={nextStep === 'continue'} onChange={() => setNextStep('continue')} />Назначить ещё один контакт</label>
          <label className="fu-toggle"><input type="radio" name={stepName} value="close" required checked={nextStep === 'close'} onChange={() => setNextStep('close')} />Завершить без следующего контакта</label>
        </fieldset>}
        {nextStep === 'continue' && <label>Дата следующего контакта<input type="date" min={today} required value={nextDate} onChange={e => setNextDate(e.target.value)} /></label>}
        {(outcome === 'done' || nextStep === 'close') && <p className="muted">Сохранение завершит контроль по этой договорённости.</p>}
        {nextStep === 'continue' && <p className="muted">Договорённость останется в очереди.</p>}
      </>}
    </fieldset>
    {conflict ? <p className="fu-error" role="alert">Договорённость уже изменилась. Закройте эту форму и проверьте актуальную запись перед новой правкой.</p> :
      m.isError && <p className="fu-error" role="alert">Не удалось сохранить. {m.error.message}</p>}
    <div className="fu-controls">
      {!conflict && <button type="submit" className="btn btn--primary" disabled={m.isPending || (mode === 'plan' ? !action.trim() : !delivery || !outcome || (outcome !== 'done' && !nextStep))}>{m.isPending ? 'Сохранение…' : mode === 'plan' ? 'Сохранить договорённость' : 'Сохранить результат'}</button>}
      <button type="button" disabled={m.isPending} className="btn btn--ghost" onClick={onCancel}>{conflict ? 'Закрыть форму' : 'Отмена'}</button>
    </div>
  </form>;
}
