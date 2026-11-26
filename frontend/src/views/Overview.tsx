import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { api, type Overview as OverviewData, type PairReport } from '../api';
import { LineChart, Sparkline } from '../components/charts';
import { ThemeMatrix } from '../components/ThemeMatrix';
import { PriorityCase } from '../components/PriorityCase';
import { TextReview } from '../components/TextReview';
import { FollowUpQueue } from '../components/FollowUps';
import { SEVERITY_MARK, SEVERITY_TEXT, STATUS_TEXT, commaNum, fmt, monthScores, months, pct, pctCI, plural } from '../format';
import { MIN_DECISIONS, type Effect, type PrecisionTotal, type SignalPrecision } from '../api';
import { navigate, pairMonthPath, pairPath } from '../router';
import { CopyButton, Loading, Failure, StaleData } from './common';

/** Главный экран HR: кто требует вмешательства, как идёт программа, тепловая карта сигналов по месяцам. */
function submitted(pair: PairReport, role: 'intern' | 'mentor', month: number) {
  return pair.response_ids ? Boolean(pair.response_ids[role]?.[month - 1]) : pair[role === 'intern' ? 'intern_index' : 'mentor_index'][month - 1] != null;
}

export function Overview({ report = false }: { report?: boolean }) {
  const q = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const [pairStatus, setPairStatus] = useState('all');
  const [selectedMonth, setSelectedMonth] = useState<number>();
  if (q.isPending) return <Loading />;
  if (q.isLoadingError || (report && q.isRefetchError)) return <Failure error={q.error} onRetry={() => q.refetch()} />;
  const r = q.data.report;
  const month = Math.min(selectedMonth ?? r.current, r.current);
  if (r.current === 0)
    return (
      <>{q.isRefetchError && <StaleData retry={() => void q.refetch()} />}<section className="sheet">
        <h2>Данных пока нет</h2>
        <p className="muted">Загрузите выгрузку анкет в разделе «Источники данных» или дождитесь первых ответов по ссылкам.</p>
        <button type="button" className="btn btn--primary" onClick={() => navigate('/sources')}>Перейти к источникам данных</button>
      </section>{!report && <FollowUpQueue />}</>
    );
  if (report) return <ProgramReport data={q.data} />;
  return (
    <>
      {q.isRefetchError && <StaleData retry={() => void q.refetch()} />}
      <PriorityCase pairs={r.pairs} current={r.current} />
      <dl className="program-strip" aria-label={`Состояние программы за месяц ${r.current}`}>
        <div><dt>Пар в программе</dt><dd>{r.pairs.length}</dd></div>
        <div><dt>Требуют вмешательства</dt><dd>{r.attention}</dd></div>
        <div><dt>Анкет заполнено · M{r.current}</dt><dd>{pct(r.fill_rate)}</dd></div>
      </dl>
      <FollowUpQueue />
      <section className="sheet pairs-queue">
        <div className="sheet__head">
          <h2>Пары «стажёр — наставник»</h2>
          <span className="sheet__meta">Месяц {r.current} из {r.periods}</span>
        </div>
        <div className="overview-queue-tools">
          <label>Ответы за <select aria-label="Месяц участия" value={month} onChange={event => setSelectedMonth(Number(event.target.value))}>
            {Array.from({length:r.current}, (_, i) => <option key={i} value={i + 1}>Месяц {i + 1}</option>)}
          </select></label>
          <span aria-live="polite">Стажёры {r.pairs.filter(p => submitted(p, 'intern', month)).length}/{r.pairs.length} · Наставники {r.pairs.filter(p => submitted(p, 'mentor', month)).length}/{r.pairs.length}</span>
          <label>Статус сейчас <select aria-label="Статус пары" value={pairStatus} onChange={event => setPairStatus(event.target.value)}>
            <option value="all">Все пары ({r.pairs.length})</option>
            <option value="attention">Требуют вмешательства ({r.attention})</option>
            <option value="watch">Под наблюдением ({r.watch})</option>
            <option value="ok">Без срочных сигналов ({r.pairs.filter(pair => pair.status === 'ok').length})</option>
          </select></label>
          {!!q.data.text_review?.unclassified && <a href="#text-review">Комментарии без темы ({q.data.text_review.unclassified})</a>}
        </div>
        <div className="table-scroll">
        <table className="pairs">
          <thead>
            <tr>
              <th>Пара</th>
              <th>Статус</th>
              <th>Динамика</th>
              {months(r.periods).map((m) => (
                <th key={m} className="pairs__m">
                  {m}
                </th>
              ))}
              <th>Главное сейчас</th>
            </tr>
          </thead>
          <tbody>
            {r.pairs.filter(p => pairStatus === 'all' || pairStatus === p.status).map((p) => (
              <tr key={p.pair} className="pairs__row" onClick={() => navigate(pairPath(p.pair))}>
                <td>
                  <button type="button" className="pairs__name" onClick={event => { event.stopPropagation(); navigate(pairPath(p.pair)); }}>
                    {p.pair}
                  </button>
                  <div className="muted">
                    {p.intern} · {p.mentor}
                    {p.track ? ` · ${p.track}` : ''}
                  </div>
                  <button className="btn btn--link" type="button" onClick={event => {event.stopPropagation(); navigate(pairMonthPath(p.pair, month));}}>
                    М{month} · {submitted(p, 'intern', month) ? 'Стажёр ответил' : 'Ждём стажёра'} · {submitted(p, 'mentor', month) ? 'Наставник ответил' : 'Ждём наставника'}
                  </button>
                  {month !== r.current && p.month_reasons?.[month - 1] && <p className="muted">{p.month_reasons[month - 1]}</p>}
                </td>
                <td>
                  <span className={`status status--${p.status}`}>{STATUS_TEXT[p.status]}</span>
                </td>
                <td>
                  <Sparkline a={p.intern_index} b={p.mentor_index} />
                </td>
                {p.history.map((h, i) => {
                  const future = i + 1 > r.current;
                  const empty = !submitted(p, 'intern', i + 1) && !submitted(p, 'mentor', i + 1);
                  const text = future ? 'ещё не наступил' : empty ? 'нет анкет' : h ? SEVERITY_TEXT[h] : 'без сигналов';
                  return (
                    <td key={i} className="pairs__m" data-month={`M${i + 1}`}>
                      <button
                        type="button"
                        onClick={(event) => { event.stopPropagation(); navigate(pairMonthPath(p.pair, i + 1)); }}
                        className={`cell cell--open${h ? ` cell--${h}` : ''}${future ? ' cell--future' : ''}`}
                        title={`M${i + 1}: ${text}`}
                        aria-label={`${p.pair}, M${i + 1}: ${text}`}
                      >
                        {future ? '' : empty ? '—' : h ? SEVERITY_MARK[h] : ''}
                      </button>
                    </td>
                  );
                })}
                <td className="pairs__head">
                  {p.status === 'ok' && p.signals.length === 0 ? <span className="muted">{p.headline}</span> : p.headline}
                  {p.status === 'ok' && p.signals.some(s => s.severity === 'low' && !['false','resolved'].includes(s.verdict?.status ?? '')) &&
                    <p className="muted">Есть плановое действие. Срок указан в карточке пары.</p>}
                  {p.status === 'ok' && solved(p.history) && <p className="solved">Критический сигнал в {solved(p.history)}. В M{r.current} не повторился.</p>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
        {pairStatus !== 'all' && !r.pairs.some(p => p.status === pairStatus) && <p className="muted" role="status">Пар с таким статусом сейчас нет.</p>}
        <p className="legend">
          <span className="cell cell--high" aria-hidden="true">!</span> критично <span className="cell cell--medium" aria-hidden="true">•</span> внимание{' '}
          <span className="cell cell--low" aria-hidden="true">○</span> наблюдать <span className="cell" aria-hidden="true" /> без сигналов
          <span className="spark-key spark-key--intern" /> стажёр <span className="spark-key spark-key--mentor" /> наставник
        </p>
      </section>

      {q.data.text_review && <TextReview review={q.data.text_review} />}
      <div className="flow-links">
        <a href="/signals">Открыть ленту сигналов →</a>
        <a href="/report">Посмотреть результаты программы →</a>
      </div>
    </>
  );
}

function KpiPairLinks({ pairs }: { pairs: OverviewData['report']['pairs'] }) {
  if (!pairs.length) return null;
  return (
    <p className="kpi__label">
      Пары: {pairs.map(({ pair }, index) => (
        <span key={pair}>
          {index > 0 && ', '}
          <a href={pairPath(pair)} aria-label={`Карточка пары ${pair}`}
            style={{ color: 'inherit', outlineColor: 'currentColor', textDecoration: 'underline' }}>{pair}</a>
        </span>
      ))}
    </p>
  );
}

function ProgramReport({ data }: { data: OverviewData }) {
  const { report: r, themes_over_time: themes = [] } = data;
  const provenance = data.data_origin && { synthetic: 'Учебные данные', mixed: 'Учебные и загруженные ответы', submitted: 'Загруженные ответы', empty: 'Ответов пока нет' }[data.data_origin];
  const digest = [provenance, commaNum(r.digest)].filter(Boolean).join('. ');
  const lowScore = r.low_score ?? 3;
  const prec: PrecisionTotal = r.precision_total ?? totalPrecision(r.precision ?? []);
  const [ia, iaPrev] = monthScores(r.intern_avg, r.current);
  const [ma, maPrev] = monthScores(r.mentor_avg, r.current);
  const earlyPairs = r.pairs.filter(({ detection }) => detection.ours > 0 && (detection.naive === 0 || detection.ours < detection.naive));
  const attentionPairs = r.pairs.filter(pair => pair.status === 'attention');
  const delta = (a: number | null, b: number | null) => (a !== null && b !== null ? ` (${a - b >= 0 ? '+' : ''}${fmt(a - b)})` : '');
  return (
    <>
      <div className="kpis overview-metrics" role="group" aria-label={`Показатели программы, месяц ${r.current}`}>
        <div className="kpi kpi--main">
          <p><strong>Ранние сигналы · {r.current === 1 ? 'M1' : `M1–M${r.current}`}</strong></p>
          <p className="kpi__value">
            {r.earlier_or_only} <small>из {r.pairs.length} пар</small>
          </p>
          <p className="kpi__label">Сигнал появился раньше порога или без его пересечения</p>
          <KpiPairLinks pairs={earlyPairs} />
        </div>
        <div className={`kpi${r.attention > 0 ? ' kpi--alert' : ''}`}>
          <p><strong>Нужна помощь · M{r.current}</strong></p>
          <p className="kpi__value">
            {r.attention} <small>из {r.pairs.length} пар</small>
          </p>
          <KpiPairLinks pairs={attentionPairs} />
          {r.watch > 0 && <p className="kpi__label">Под наблюдением ещё {r.watch} {plural(r.watch, 'пара', 'пары', 'пар')}</p>}
        </div>
        <div className="kpi">
          <p className="kpi__value">{fmt(ia)}</p>
          <p className="kpi__label">индекс стажёров в M{r.current}{ia === null ? ' · нет данных за месяц' : delta(ia, iaPrev)}</p>
        </div>
        <div className="kpi">
          <p className="kpi__value">{fmt(ma)}</p>
          <p className="kpi__label">оценка стажёров наставниками в M{r.current}{ma === null ? ' · нет данных за месяц' : delta(ma, maPrev)}</p>
        </div>
        <div className="kpi">
          <p className="kpi__value">{pct(r.fill_rate)}</p>
          <p className="kpi__label">анкет заполнено за M{r.current}</p>
        </div>
      </div>

      <div className="grid2">
        <section className="sheet">
          <div className="sheet__head">
            <h2>Сводка для руководства</h2>
            <CopyButton text={digest} label="Скопировать" />
          </div>
          <p className="digest">{digest}</p>
          <dl className="kv">
            <div>
              <dt>Активных сигналов</dt>
              <dd>{r.signals_now}</dd>
            </div>
            <div>
              <dt>Подтверждено HR</dt>
              <dd>{r.confirmed}</dd>
            </div>
            <div>
              <dt>Отмечено ложными</dt>
              <dd>{r.false_positive}</dd>
            </div>
          </dl>
          <p className="prec-line">
            Точность сигналов по решениям HR: <PrecisionValue p={prec} />{' '}
            {!!r.precision?.length && <a href="/thresholds#prec-title">По типам сигналов</a>}
          </p>
        </section>
        <section className="sheet">
          <div className="sheet__head">
            <h2>Динамика программы</h2>
            <span className="sheet__meta">среднее по парам, шкала 1–5</span>
          </div>
          <LineChart
            label="Средний индекс стажёров и оценка наставников по месяцам"
            periods={r.periods}
            threshold={{ value: lowScore, text: `порог ${lowScore.toLocaleString('ru-RU')}` }}
            series={[
              { name: 'Стажёры о стажировке', values: r.intern_avg, color: 'var(--c-intern)' },
              { name: 'Наставники о стажёрах', values: r.mentor_avg, color: 'var(--c-mentor)', dashed: true },
            ]}
          />
          <p className="muted">
            Средние сглаживают проблемы: в M{r.current} индекс программы {fmt(ia)}, а {r.attention} из {r.pairs.length} пар требуют
            вмешательства. Поэтому главное — сигналы по каждой паре.
          </p>
        </section>
      </div>
      {r.effect && <EffectBlock e={r.effect} prec={prec} periods={r.periods} threshold={lowScore.toLocaleString('ru-RU')} />}

      {themes.length > 0 && (
        <section className="sheet" aria-labelledby="themes-title">
          <div className="sheet__head">
            <h2 id="themes-title">О чём пишут в комментариях</h2>
            <span className="sheet__meta">все пары · число комментариев с темой за месяц · темы из закрытого списка</span>
          </div>
          <ThemeMatrix themes={themes} periods={r.periods} current={r.current} />
        </section>
      )}
      <div className="flow-links"><a href="/quality">Проверить качество сигналов и время реакции →</a></div>
    </>
  );
}

/** Месяцы с критичными сигналами в истории пары: «M3–M4» или «M2, M5». */
function solved(history: string[]) {
  const ms = history.map((h, i) => (h === 'high' ? i + 1 : 0)).filter(Boolean);
  if (!ms.length) return '';
  const contiguous = ms.every((m, i) => i === 0 || m === ms[i - 1] + 1);
  return ms.length === 1 ? `M${ms[0]}` : contiguous ? `M${ms[0]}–M${ms[ms.length - 1]}` : ms.map((m) => `M${m}`).join(', ');
}

/** Общая точность (запасной расчёт, если бэкенд не прислал precision_total): (подтверждено + решено) / все решения HR. */
export function totalPrecision(list: SignalPrecision[]): PrecisionTotal {
  const good = list.reduce((a, x) => a + x.confirmed + x.resolved, 0);
  const n = good + list.reduce((a, x) => a + x.false, 0);
  return { decisions: n, precision: n ? good / n : null, enough: n >= MIN_DECISIONS };
}

/** Точность в процентах — только когда решений достаточно; иначе честно «мало решений». */
export function PrecisionValue({ p }: { p: PrecisionTotal }) {
  if (!p.enough || p.precision === null)
    return (
      <span className="muted">
        решений HR: {p.decisions}; для оценки нужно не меньше {MIN_DECISIONS}
      </span>
    );
  return (
    <>
      <b>{pctCI(p.precision, p.lo, p.hi)}</b> ({p.decisions} {plural(p.decisions, 'решение', 'решения', 'решений')})
    </>
  );
}

const lead = (ours: number | null, naive: number | null) => {
  if (ours === null) return { text: 'сигналов не было', tone: 'none' };
  if (naive === null) return { text: 'видит только система', tone: 'win' };
  const d = naive - ours;
  if (d > 0) return { text: `раньше на ${d} ${plural(d, 'месяц', 'месяца', 'месяцев')}`, tone: 'win' };
  if (d === 0) return { text: 'одновременно', tone: 'same' };
  return { text: `позже на ${-d} ${plural(-d, 'месяц', 'месяца', 'месяцев')}`, tone: 'lose' };
};

/** Сравнение с порогом оценки: когда проблему увидела система и когда — средняя оценка; как HR отрабатывает сигналы. */
function EffectBlock({ e, prec, periods, threshold }: { e: Effect; prec: PrecisionTotal; periods: number; threshold: string }) {
  return (
    <section className="sheet effect" aria-labelledby="effect-title">
      <div className="sheet__head">
        <h2 id="effect-title">Сравнение с порогом оценки</h2>
        <span className="sheet__meta">система против средней оценки · работа HR с сигналами</span>
      </div>
      <div className="effect__grid">
        <div className="effect__main">
          <p className="effect__lead">
            <b className="effect__num">{e.pairs_flagged_early}</b> из {e.detection_table.length}{' '}
            {plural(e.detection_table.length, 'пары', 'пар', 'пар')}: система подняла сигнал раньше, чем средняя оценка опустилась ниже {threshold}, или там, где она
            так и не опустилась
          </p>
          <div className="table-scroll">
            <table className="effect__table">
              <thead>
                <tr>
                  <th scope="col">Пара</th>
                  <th scope="col">Сигнал системы</th>
                  <th scope="col">Средняя &lt; {threshold}</th>
                  <th scope="col">Итог</th>
                </tr>
              </thead>
              <tbody>
                {e.detection_table.map((row) => {
                  const l = lead(row.ours, row.naive);
                  return (
                    <tr key={row.pair}>
                      <th scope="row">
                        <button type="button" className="pairs__name" onClick={() => navigate(pairPath(row.pair))}>
                          {row.pair}
                        </button>
                      </th>
                      <td>{row.ours === null ? <span className="muted">—</span> : `M${row.ours}`}</td>
                      <td>{row.naive === null ? <span className="muted">не упала за {periods} мес.</span> : `M${row.naive}`}</td>
                      <td>
                        <span className={`lead lead--${l.tone}`}>{l.text}</span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
        <dl className="effect__kv">
          <div>
            <dt>Покрытие решениями HR</dt>
            <dd>
              {e.verdict_coverage === null ? <span className="muted">нет активных сигналов</span> : <b>{pct(e.verdict_coverage)}</b>}
              <span className="muted">
                {' '}
                {e.decided_signals} из {e.active_signals} активных
              </span>
            </dd>
          </div>
          <div>
            <dt>Просрочено</dt>
            <dd>
              {e.overdue === null ? (
                <span className="muted">нет данных о датах</span>
              ) : (
                <b className={e.overdue > 0 ? 'prec__low' : undefined}>{e.overdue}</b>
              )}
              {e.overdue_unknown > 0 && <span className="muted"> · без даты появления: {e.overdue_unknown}</span>}
            </dd>
          </div>
          <div>
            <dt>Точность сигналов</dt>
            <dd>
              <PrecisionValue p={prec} />
            </dd>
          </div>
        </dl>
      </div>
    </section>
  );
}
