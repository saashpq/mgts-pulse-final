import { Fragment, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type FlatSignal, type GroupedSignal, type Severity, type SignalsView } from '../api';
import { SignalCard } from '../components/SignalCard';
import { SEVERITY_MARK, SEVERITY_RANK, SEVERITY_TEXT, STATUS_TEXT, plural } from '../format';
import { Failure, Loading } from './common';
import './signals-feed.css';

type State = 'active' | 'past' | '';

/** Активные выше снятых, затем по критичности, затем по сроку реакции. */
export const bySignalPriority = (a: FlatSignal, b: FlatSignal) =>
  Number(b.active) - Number(a.active) || SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity] || a.due_days - b.due_days || a.pair.localeCompare(b.pair);

/**
 * Лента сигналов. По умолчанию — по парам: главный сигнал пары и свёрнутые подтверждающие, чтобы HR видел
 * одну проблему пары, а не шесть её проявлений. «Все сигналы» — плоский список. Фильтры и счётчики — по выбранному виду.
 */
export function Signals() {
  const [expandedFilters, setExpandedFilters] = useState(false);
  const [view, setView] = useState<SignalsView>('grouped');
  const [state, setState] = useState<State>('active');
  const [sev, setSev] = useState<Severity | ''>('');
  const q = useQuery({ queryKey: ['signals', view], queryFn: () => api.signals(view) });
  const overview = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const all = q.data?.signals ?? [];
  const okState = (s: FlatSignal, v: State) => v === '' || (v === 'active') === s.active;
  const okSev = (s: FlatSignal, v: Severity | '') => v === '' || s.severity === v;
  const shown = all.filter((s) => okState(s, state) && okSev(s, sev)).sort(bySignalPriority);
  const grouped = view === 'grouped';
  const shownPairs = new Set(shown.map(s => s.pair));
  const pairCount = `${shownPairs.size} ${plural(shownPairs.size, 'пара', 'пары', 'пар')}`;
  const pairs = overview.data?.report.pairs.filter(p => shownPairs.has(p.pair));
  const statusSummary = pairs?.length === shownPairs.size
    ? (['attention', 'watch', 'ok'] as const).map(status => {
        const count = pairs.filter(p => p.status === status).length;
        const label = status === 'attention' ? 'Требуют вмешательства' : STATUS_TEXT[status];
        return count ? ` · ${label} — ${count}` : '';
      }).join('')
    : '';
  const clearFilters = () => {
    setState('');
    setSev('');
  };

  const filter = <T extends string>(cur: T, set: (v: T) => void, v: T, text: ReactNode, n: number) => (
    <button type="button" className={`filter${cur === v ? ' filter--active' : ''}`} aria-pressed={cur === v} onClick={() => set(v)}>
      {text} <span className="filter__n">{q.data ? n : '—'}</span>
    </button>
  );
  const sevText = (v: Severity) => (
    <>
      <span aria-hidden="true">{SEVERITY_MARK[v]}</span> {SEVERITY_TEXT[v]}
    </>
  );
  return (
    <>
      <div className="signals-toolbar" data-expanded={expandedFilters}>
        <div className="signals-toolbar__field">
          <span className="filters__label" id="f-state">
            {grouped ? 'Группы сигналов по состоянию' : 'Сигналы по состоянию'}
          </span>
          <div className="filters__group" role="group" aria-labelledby="f-state">
            {filter(state, setState, 'active', 'Активные', all.filter((s) => s.active && okSev(s, sev)).length)}
            {filter(state, setState, 'past', 'Снятые', all.filter((s) => !s.active && okSev(s, sev)).length)}
            {filter(state, setState, '', 'Все', all.filter((s) => okSev(s, sev)).length)}
          </div>
        </div>
        <button className="signal-filter-toggle" aria-expanded={expandedFilters} aria-controls="signal-view signal-severity" onClick={() => setExpandedFilters(!expandedFilters)}>
          {expandedFilters ? 'Свернуть фильтры' : 'Вид и критичность'}{!expandedFilters && sev && ' · ' + SEVERITY_TEXT[sev]}
        </button>
        <div className="signals-toolbar__field" id="signal-view">
          <span className="filters__label" id="f-view">
            Вид
          </span>
          <div className="filters__group" role="group" aria-labelledby="f-view">
            {(
              [
                ['grouped', 'По парам'],
                ['all', 'Все сигналы'],
              ] as const
            ).map(([v, t]) => (
              <button key={v} type="button" className={`filter${view === v ? ' filter--active' : ''}`} aria-pressed={view === v} onClick={() => setView(v)}>
                {t}
              </button>
            ))}
          </div>
        </div>
        <div className="signals-toolbar__field" id="signal-severity">
          <span className="filters__label" id="f-sev">
            {grouped ? 'Критичность главного сигнала' : 'Критичность сигнала'}
          </span>
          <div className="filters__group" role="group" aria-labelledby="f-sev">
            {filter(sev, setSev, '', 'Любая', all.filter((s) => okState(s, state)).length)}
            {(['high', 'medium', 'low'] as const).map((v) => (
              <Fragment key={v}>{filter(sev, setSev, v, sevText(v), all.filter((s) => s.severity === v && okState(s, state)).length)}</Fragment>
            ))}
          </div>
        </div>
      </div>
      {q.isPending ? (
        <Loading />
      ) : q.isError ? (
        <Failure error={q.error} onRetry={() => q.refetch()} />
      ) : shown.length === 0 ? (
        <section className="signals-empty">
          <h2>{all.length ? 'Сигналов с такими фильтрами нет' : 'Сигналов пока нет'}</h2>
          <p>{all.length ? 'Можно вернуться ко всем уровням критичности и состояниям.' : 'Здесь появятся сигналы, когда ответы участников укажут на трудности в паре.'}</p>
          {all.length > 0 && <button type="button" className="signals-reset" onClick={clearFilters}>Сбросить фильтры</button>}
        </section>
      ) : (
        <div className="sig-list">
          <h2 className="sr-only">Список сигналов</h2>
          <div className="signals-results">
            <span role="status">
              {grouped ? (
                <>
                  <strong>{pairCount} {state === 'active' ? 'с активными сигналами' : state === 'past' ? 'со снятыми сигналами' : 'с сигналами'}</strong>
                  {state === '' && ` · ${shown.length} ${plural(shown.length, 'группа', 'группы', 'групп')}, активные и снятые отдельно`}
                  {state === 'active' && statusSummary}
                </>
              ) : (
                <><strong>{shown.length} {plural(shown.length, 'сигнал', 'сигнала', 'сигналов')}</strong> · {pairCount}</>
              )}
            </span>
            {(sev || state === 'past') && <button type="button" className="signals-reset" onClick={clearFilters}>Сбросить фильтры</button>}
          </div>
          {shown.map((s) =>
            view === 'grouped' ? (
              <Group key={s.id} s={s} />
            ) : (
              <SignalCard key={s.id} s={s} active={s.active} showPair who={`${s.intern} · ${s.mentor}`} />
            ),
          )}
        </div>
      )}
    </>
  );
}

/** Главный сигнал пары и подтверждающие: компактные строки, по нажатию — полная карточка. */
function Group({ s }: { s: GroupedSignal }) {
  const rel = [...(s.related_signals ?? [])].sort(bySignalPriority);
  return (
    <div className="sig-group">
      <SignalCard s={s} active={s.active} showPair who={`${s.intern} · ${s.mentor}`} />
      {rel.length > 0 && (
        <details className="sig-rel">
          <summary className="sig-rel__summary">
            Подтверждающие сигналы ({rel.length})
            <span className="sig-rel__peek" aria-hidden="true">
              {rel.map((r) => (
                <span key={r.id}>
                  <span className={`sev-text sev-text--${r.severity}`}>{SEVERITY_MARK[r.severity]}</span> {r.title}
                </span>
              ))}
            </span>
          </summary>
          <ul className="sig-rel__list">
            {rel.map((r) => (
              <li key={r.id}>
                <details className="sig-rel__item">
                  <summary className="sig-rel__row">
                    <span className={`sev-text sev-text--${r.severity}`}>
                      <span aria-hidden="true">{SEVERITY_MARK[r.severity]}</span> {SEVERITY_TEXT[r.severity]}
                    </span>
                    <span className="sig-rel__title">{r.title}</span>
                    <span className="muted">
                      {r.type} · {r.first_period === r.last_period ? `M${r.last_period}` : `M${r.first_period}–M${r.last_period}`}
                      {r.due_days ? ` · ${r.due_days} ${plural(r.due_days, 'день', 'дня', 'дней')}` : ''}
                    </span>
                  </summary>
                  <SignalCard s={r} active={r.active} />
                </details>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
