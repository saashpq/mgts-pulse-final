import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type ActionRow, type Agreement, type Comment, type PairReport, type QuestionSeries, type Severity, type Signal } from '../api';
import { DumbbellChart, LineChart } from '../components/charts';
import { SignalCard } from '../components/SignalCard';
import { ROLE_TEXT, VERDICT_TEXT, biasText, commaNum, fmt, SEVERITY_MARK, SEVERITY_RANK, SEVERITY_TEXT, STATUS_TEXT, TONE_TEXT, plural, signed, toneOf } from '../format';
import { navigate, pairMonthPath, pairPath } from '../router';
import { Failure, Loading, StaleData } from './common';
import { actionForAudience, scheduledSignals } from '../meetingPlan';
import { MonthEvidence } from './MonthEvidence';
import './pair-refresh.css';

const DIMENSION_TEXT: Record<string, string> = { satisfaction: 'как идёт стажировка', clarity: 'понятность задач' };
// вопросы внутри одной роли: свои цвета (не цвета ролей), проверены валидатором на различимость при дальтонизме
const PALETTE = ['#0A9A6E', '#B0479E'];

/** Карточка пары: что сделать, почему так считает система, исходные ответы и цитаты. */
export function PairView({ id, month }: { id: string; month?: number }) {
  const q = useQuery({ queryKey: ['pair', id], queryFn: () => api.pair(id) });
  const ov = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  if (q.isPending) return <Loading />;
  if (q.isLoadingError || (month !== undefined && q.isRefetchError)) return <Failure error={q.error} onRetry={() => q.refetch()} />;
  if (month !== undefined) return <MonthEvidence detail={q.data} month={month} />;
  const { report: p, questions, comments, themes, periods, current } = q.data;
  const lowScore = q.data.low_score ?? 3;
  const byCode = Object.fromEntries(questions.map((x) => [x.code, x])) as Record<string, QuestionSeries>;
  const mirrors = questions.filter((x) => x.role === 'intern' && x.mirror && byCode[x.mirror]);
  const others = questions.filter((x) => !x.mirror);
  const analyzers = Object.fromEntries(comments.map((c) => [`${c.response_id}|${c.question}`, c.analysis.analyzer]));
  const actions = (ov.data?.report.actions ?? []).filter((a) => a.pair === p.pair);
  return (
    <>
      {q.isRefetchError && <StaleData retry={() => void q.refetch()} />}
      <button type="button" className="back-link btn btn--link" onClick={() => navigate('/')}>
        ← Все пары
      </button>
      <section className={`sheet hero pair-profile hero--${p.status}`} aria-labelledby="pair-title">
        <div className="sheet__head">
          <h2 id="pair-title">
            Пара {p.pair}
          </h2>
          <span className="hero__actions">
            <span className={`status status--${p.status}`}>{STATUS_TEXT[p.status]}</span>
            {current > 0 && <button type="button" className="btn btn--ghost btn--small" onClick={() => navigate(pairMonthPath(id, current))}>Ответы за M{current}</button>}
            <button type="button" className="btn btn--primary btn--small" onClick={() => navigate(pairPath(p.pair, 'brief'))}>
              Бриф к встрече 1:1
            </button>
          </span>
        </div>
        <p className="pair-profile__people"><span>Стажёр <b>{p.intern}</b></span><span>Наставник <b>{p.mentor}</b></span>{p.track && <span>{p.track}</span>}</p>
        <p className="hero__summary">{p.headline}</p>
        {p.summary && <details className="pair-summary-details">
          <summary>Сводка по паре</summary>
          <p>{commaNum(p.summary).replace(/\.\. Цитата/g, '. Цитата').replace(/\. — в течение/g, ' — в течение')}</p>
        </details>}
        <DetectionScale det={p.detection} periods={periods} threshold={lowScore.toLocaleString('ru-RU')} />
      </section>

      <nav className="pair-jump" aria-label="Разделы карточки пары">
        {p.signals.some(s => s.verdict?.status !== 'false' && s.verdict?.status !== 'resolved') && <a href="#todo-title">План действий</a>}
        <a href="#sig-title">Основания сигналов</a>
        <a href="#comm-title">Комментарии</a>
        <a href="#pair-trend">Динамика</a>
      </nav>

      <ActionPlan signals={p.signals} pair={p.pair} />

      <SignalsBySeverity pair={p.pair} signals={p.signals} resolved={p.resolved} current={current} analyzers={analyzers} />

      <Comments comments={comments} themes={themes} periods={periods} />

      <div className="grid2">
        <section className="sheet">
          <h2 className="sheet__h" id="pair-trend">Общая динамика пары</h2>
          <LineChart
            label="Индекс стажёра и оценка наставника по месяцам"
            periods={periods}
            threshold={{ value: lowScore, text: `порог ${lowScore.toLocaleString('ru-RU')}` }}
            series={[
              { name: 'Стажёр о стажировке', values: p.intern_index, color: 'var(--c-intern)' },
              { name: 'Наставник о стажёре', values: p.mentor_index, color: 'var(--c-mentor)', dashed: true },
            ]}
          />
        </section>
        {mirrors.map((m) => (
          <section className="sheet" key={m.code}>
            <h2 className="sheet__h">Глазами двоих: {DIMENSION_TEXT[m.dimension] ?? m.text}</h2>
            <p className="muted mirror-q">
              Стажёр: «{m.text}» · Наставник: «{byCode[m.mirror!].text}»
            </p>
            <DumbbellChart
              label={`Стажёр: ${m.text} Наставник: ${byCode[m.mirror!].text} Оценки по месяцам и разница`}
              periods={periods}
              a={{ name: 'Стажёр', values: m.values }}
              b={{ name: 'Наставник', values: byCode[m.mirror!].values }}
            />
          </section>
        ))}
        {(['intern', 'mentor'] as const).map((role) => {
          const qs = others.filter((x) => x.role === role);
          if (!qs.length) return null;
          return (
            <section className="sheet" key={role}>
              <h2 className="sheet__h">{role === 'intern' ? 'Ответы стажёра по вопросам' : 'Ответы наставника по вопросам'}</h2>
              <LineChart
                label={`Вопросы: ${ROLE_TEXT[role]}`}
                periods={periods}
                series={qs.map((x, i) => ({ name: x.text, values: x.values, color: PALETTE[i % PALETTE.length], dashed: i % 2 === 1 }))}
              />
            </section>
          );
        })}
      </div>

      <AgreementBlock a={p.agreement_questions?.length ? p.agreement : null} names={p.agreement_questions ?? []} />

      <ActionHistory rows={actions} signals={[...p.signals, ...p.resolved]} />
    </>
  );
}

/** Мини-шкала M1..Mn: когда сигнал поднял система и когда средняя оценка опустилась ниже 3. */
function DetectionScale({ det, periods, threshold }: { det: PairReport['detection']; periods: number; threshold: string }) {
  if (!det.ours) return <p className="muted">Значимых сигналов за программу не было.</p>;
  const lead = det.naive ? det.naive - det.ours : 0;
  const head =
    det.naive === 0
      ? `Система видит проблему с M${det.ours}, средняя оценка — нет`
      : lead > 0
        ? `Раньше на ${lead} ${plural(lead, 'месяц', 'месяца', 'месяцев')}`
        : lead === 0
          ? 'Одновременно со средней оценкой'
          : 'Средняя оценка заметила раньше';
  const text =
    `Система подняла сигнал в M${det.ours}. ` +
    (det.naive ? `Средняя оценка опустилась ниже ${threshold} в M${det.naive}.` : `Средняя оценка так и не опустилась ниже ${threshold}.`);
  return (
    <div className="detect">
      <p className="detect__head">{head}</p>
      <ol className="detect__scale" aria-label={text}>
        {Array.from({ length: periods }, (_, i) => {
          const m = i + 1;
          const ours = m === det.ours;
          const naive = m === det.naive;
          const between = det.naive > det.ours ? m >= det.ours && m < det.naive : det.naive === 0 && m >= det.ours;
          return (
            <li key={m} className={`detect__m${between ? ' detect__m--lead' : ''}`} aria-hidden="true">
              <span className="detect__tick">M{m}</span>
              <span className="detect__marks">
                {ours && <span className="detect__mark detect__mark--ours">сигнал системы</span>}
                {naive && <span className="detect__mark detect__mark--naive">средняя &lt; {threshold}</span>}
              </span>
            </li>
          );
        })}
      </ol>
      {!det.naive && <p className="detect__note">Средняя так и не упала ниже {threshold}: по ней проблему не видно.</p>}
    </div>
  );
}

/** «Что сделать сейчас»: один разговор на участника в ближайший из сроков его сигналов. */
function ActionPlan({ signals, pair }: { signals: Signal[]; pair: string }) {
  const open = signals.filter((s) => s.verdict?.status !== 'false' && s.verdict?.status !== 'resolved');
  if (!open.length) return null;
  type PlanItem = { action: string; questions: Set<string>; signals: Signal[] };
  const groups = new Map<string, PlanItem[]>();
  const items = new Map<string, PlanItem>();
  for (const { signal: s, due, meeting } of scheduledSignals(open)) {
    const specificAction = actionForAudience(s);
    const key = meeting ? meeting.role : JSON.stringify([due.label, specificAction]);
    let item = items.get(key);
    if (!item) {
      const action = meeting ? `Личная встреча HR ${meeting.role === 'intern' ? 'со стажёром' : 'с наставником'}` : specificAction;
      item = { action, questions: new Set(), signals: [] };
      items.set(key, item);
      const group = groups.get(due.label) ?? [];
      group.push(item);
      groups.set(due.label, group);
    }
    item.signals.push(s);
    if (meeting) item.questions.add(meeting.question);
  }
  return (
    <section className="sheet todo" aria-labelledby="todo-title">
      <div className="sheet__head">
        <h2 id="todo-title">Что сделать сейчас</h2>
        <span className="sheet__meta">из активных сигналов · решение принимает HR</span>
      </div>
      <ol className="todo__groups">
        {[...groups].map(([label, actions]) => (
          <li key={label} className="todo__group">
            <p className="todo__due">{label}</p>
            <ul className="todo__list">
              {actions
                .sort((a, b) => Math.min(...a.signals.map((s) => SEVERITY_RANK[s.severity])) - Math.min(...b.signals.map((s) => SEVERITY_RANK[s.severity])))
                .map(({action, questions, signals: sigs}) => (
                  <li key={sigs[0].id} className="todo__item">
                    <span className="todo__action">{action}</span>
                    {questions.size > 0 && <ul aria-label="Вопросы для разговора">{[...questions].map(q => <li key={q}>{q}</li>)}</ul>}
                    <span className="todo__why">
                      {sigs.map(s => {
                        const e = s.evidence.find(item => item.quote && item.period > 0) ?? s.evidence.find(item => item.period > 0);
                        return <span key={s.id} className={`sev-text sev-text--${s.severity}`}>
                          <span aria-hidden="true">{SEVERITY_MARK[s.severity]}</span> {s.title}
                          {e && <button type="button" className="pair-source" aria-label={`Открыть основание «${s.title}» за M${e.period}`}
                            onClick={() => {
                              const question = e.question && (e.quote || e.value != null);
                              const anchor = question ? `${e.quote ? 'comment' : 'score'}-${e.role}-${encodeURIComponent(e.question!)}` : `month-${e.role}`;
                              navigate(pairMonthPath(pair, e.period) + '#' + anchor);
                            }}>Анкета M{e.period} <span aria-hidden="true">→</span></button>}
                        </span>;
                      })}
                    </span>
                  </li>
                ))}
            </ul>
          </li>
        ))}
      </ol>
    </section>
  );
}

/** «После действия индекс +0,8 за 1 мес» или честное «данных после решения ещё нет». */
export function deltaText(r: Pick<ActionRow, 'delta' | 'months' | 'period' | 'role'>) {
  const who = r.role === 'mentor' ? 'оценка наставника' : 'индекс стажёра';
  if (r.delta === null || r.delta === undefined)
    return `эффект пока не измерить: после M${r.period} анкет ещё нет`;
  return `после действия ${who} ${signed(r.delta)} за ${r.months} мес.`;
}

/** История действий HR по паре и изменение индекса после них. Без контрольной группы это наблюдение, а не доказательство. */
function ActionHistory({ rows, signals }: { rows: ActionRow[]; signals: Signal[] }) {
  const known = new Set(rows.map((r) => r.signal));
  // решения с «что сделано», которых ещё нет в журнале сервера (старый снимок или только что сохранено)
  const extra = signals.filter((s) => s.verdict?.action && !known.has(s.id));
  if (!rows.length && !extra.length) return null;
  return (
    <section className="sheet" aria-labelledby="acts-title">
      <div className="sheet__head">
        <h2 id="acts-title">Что уже сделано и что изменилось</h2>
        <span className="sheet__meta">наблюдение, а не доказательство: контрольной группы нет</span>
      </div>
      <ul className="acts">
        {rows.map((r) => (
          <li key={r.signal + r.period} className="acts__item">
            <span className="acts__what">
              M{r.period} · {r.action || <span className="muted">действие не указано</span>}
            </span>
            <span className="muted">
              {r.title} · {VERDICT_TEXT[r.status] ?? r.status}
            </span>
            <span className={`acts__delta${r.delta === null ? ' muted' : r.delta > 0 ? ' acts__delta--up' : r.delta < 0 ? ' acts__delta--down' : ''}`}>
              {deltaText(r)}
              {r.before !== null && r.after !== null && (
                <span className="muted">
                  {' '}
                  ({fmt(r.before, 2)} → {fmt(r.after, 2)})
                </span>
              )}
            </span>
          </li>
        ))}
        {extra.map((s) => (
          <li key={s.id} className="acts__item">
            <span className="acts__what">
              {s.verdict?.period ? `M${s.verdict.period} · ` : ''}
              {s.verdict!.action}
            </span>
            <span className="muted">
              {s.title} · {VERDICT_TEXT[s.verdict!.status]}
            </span>
            <span className="acts__delta muted">эффект появится после следующего месяца анкет</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Согласие сторон: на сколько в среднем стажёр оценивает выше/ниже наставника и разброс разницы. */
function AgreementBlock({ a, names }: { a?: Agreement | null; names: string[] }) {
  return (
    <section className="sheet" aria-labelledby="agree-title">
      <div className="sheet__head">
        <h2 id="agree-title">Согласие сторон</h2>
        <span className="sheet__meta">разница ответов на сопоставимые вопросы</span>
      </div>
      {a ? (
        <>
          <p className="agree__main">
            <b className={Math.abs(a.bias) >= 1 ? 'prec__low' : undefined}>Смещение {signed(a.bias, 2)}</b>: {biasText(a.bias)}.
          </p>
          <p className="muted">
            Стажёр — «{names[0]}». Наставник — «{names[1]}». Расчётные границы разницы {signed(a.loa_lo, 2)}…{signed(a.loa_hi, 2)} балла · {a.n}{' '}
            {plural(a.n, 'месяц', 'месяца', 'месяцев')} с оценками обеих сторон.
            {Math.abs(a.bias) >= 1 ? ' Разрыв больше балла — стороны видят стажировку по-разному, стоит обсудить ожидания.' : ''}
          </p>
        </>
      ) : (
        <p className="muted">
          Для расчёта нужны ответы на сопоставимые вопросы обеих сторон хотя бы за два месяца.
        </p>
      )}
    </section>
  );
}

/** Сигналы по критичности: критичные раскрыты, остальные свёрнуты. */
function SignalsBySeverity({
  pair,
  signals,
  resolved,
  current,
  analyzers,
}: {
  pair: string;
  signals: Signal[];
  resolved: Signal[];
  current: number;
  analyzers: Record<string, string>;
}) {
  const by = (sev: Severity) => signals.filter((s) => s.severity === sev);
  useEffect(() => {
    let linkedDecision: HTMLElement | null = null;
    function focusDecision() {
      linkedDecision?.removeAttribute('data-linked-decision');
      linkedDecision = null;
      const hash = window.location.hash.slice(1);
      if (hash !== 'sig-title' && !hash.startsWith('decision-')) return;
      let target: HTMLElement | null = null;
      try {
        target = document.getElementById(decodeURIComponent(hash));
      } catch {
        // An old, malformed fragment still leads to the pair's signals.
      }
      target ??= document.getElementById('sig-title');
      const disclosure = target?.closest('details');
      if (disclosure) disclosure.open = true;
      if (target?.classList.contains('sig__verdict')) {
        linkedDecision = target;
        linkedDecision.setAttribute('data-linked-decision', '');
      }
      target?.focus();
    }
    focusDecision();
    window.addEventListener('hashchange', focusDecision);
    window.addEventListener('popstate', focusDecision);
    return () => {
      linkedDecision?.removeAttribute('data-linked-decision');
      window.removeEventListener('hashchange', focusDecision);
      window.removeEventListener('popstate', focusDecision);
    };
  }, [pair]);
  return (
    <section className="sheet" aria-labelledby="sig-title">
      <div className="sheet__head">
        <h2 id="sig-title" tabIndex={-1}>Сигналы и почему они сработали</h2>
        <span className="sheet__meta">месяц {current}</span>
      </div>
      {signals.length === 0 && <p className="muted">Активных сигналов нет.</p>}
      {by('high').length > 0 && (
        <div className="sig-list">
          {by('high').map((s) => (
            <SignalCard key={s.id} s={s} analyzers={analyzers} />
          ))}
        </div>
      )}
      {(['medium', 'low'] as const).map((sev) =>
        by(sev).length ? (
          <details className="block sev-group" key={sev}>
            <summary className="block__title">
              <span className={`sev-text sev-text--${sev}`}>
                <span aria-hidden="true">{SEVERITY_MARK[sev]}</span> {SEVERITY_TEXT[sev]}
              </span>{' '}
              · {by(sev).length} {plural(by(sev).length, 'сигнал', 'сигнала', 'сигналов')}
            </summary>
            <div className="sig-list">
              {by(sev).map((s) => (
                <SignalCard key={s.id} s={s} analyzers={analyzers} />
              ))}
            </div>
          </details>
        ) : null,
      )}
      {resolved.length > 0 && (
        <details className="block sev-group">
          <summary className="block__title">Были раньше и сейчас не срабатывают ({resolved.length})</summary>
          <div className="sig-list">
            {resolved.map((s) => (
              <SignalCard key={s.id} s={s} active={false} analyzers={analyzers} />
            ))}
          </div>
        </details>
      )}
    </section>
  );
}

type Themes = Record<string, { label: string; negative: boolean }>;

/** Комментарии: фильтры, матрица «тема × месяц», список с подсвеченной цитатой. */
function Comments({ comments, themes, periods }: { comments: Comment[]; themes: Themes; periods: number }) {
  const [role, setRole] = useState<'' | 'intern' | 'mentor'>('');
  const [neg, setNeg] = useState(false);
  const [theme, setTheme] = useState('');
  const used = [...new Set(comments.flatMap((c) => c.analysis.themes))].sort(
    (a, b) => Number(!!themes[b]?.negative) - Number(!!themes[a]?.negative) || (themes[a]?.label ?? a).localeCompare(themes[b]?.label ?? b),
  );
  const shown = comments.filter(
    (c) => (!role || c.role === role) && (!neg || toneOf(c.analysis.sentiment) === 'neg') && (!theme || c.analysis.themes.includes(theme)),
  );
  const base = comments.filter((c) => (!role || c.role === role) && (!neg || toneOf(c.analysis.sentiment) === 'neg'));
  const count = (t: string, m: number) => base.filter((c) => c.period === m && c.analysis.themes.includes(t)).length;
  const label = (t: string) => themes[t]?.label ?? t;
  return (
    <section className="sheet" aria-labelledby="comm-title">
      <div className="sheet__head">
        <h2 id="comm-title">Комментарии и разбор ИИ</h2>
        <span className="sheet__meta">темы — из закрытого списка, цитата — дословно из ответа</span>
      </div>
      <div className="filters" role="group" aria-label="Фильтры комментариев">
        {(
          [
            ['', 'Все'],
            ['intern', 'Стажёр'],
            ['mentor', 'Наставник'],
          ] as const
        ).map(([v, t]) => (
          <button key={v} type="button" className={`filter${role === v ? ' filter--active' : ''}`} aria-pressed={role === v} onClick={() => setRole(v)}>
            {t}
          </button>
        ))}
        <span className="filters__gap" />
        <button type="button" className={`filter${neg ? ' filter--active' : ''}`} aria-pressed={neg} onClick={() => setNeg((v) => !v)}>
          Только негатив
        </button>
        <label className="filter-select">
          Тема
          <select value={theme} onChange={(e) => setTheme(e.target.value)}>
            <option value="">все темы</option>
            {used.map((t) => (
              <option key={t} value={t}>
                {label(t)}
              </option>
            ))}
          </select>
        </label>
      </div>

      {used.length > 0 && (
        <div className="tm__wrap">
          <table className="tm">
            <caption className="tm__cap">Темы по месяцам: сколько комментариев с темой. Нажмите на тему, чтобы отфильтровать список.</caption>
            <thead>
              <tr>
                <th scope="col">Тема</th>
                {Array.from({ length: periods }, (_, i) => (
                  <th scope="col" key={i}>
                    M{i + 1}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {used.map((t) => (
                <tr key={t} className={theme === t ? 'tm__row--on' : undefined}>
                  <th scope="row">
                    <button type="button" className="tm__theme" aria-pressed={theme === t} onClick={() => setTheme(theme === t ? '' : t)}>
                      {label(t)}
                    </button>
                  </th>
                  {Array.from({ length: periods }, (_, i) => {
                    const n = count(t, i + 1);
                    return (
                      <td key={i} aria-label={`${label(t)}, M${i + 1}: ${n}`}>
                        {n > 0 && (
                          <span className={`tm__dot${themes[t]?.negative ? ' tm__dot--neg' : ''}`} aria-hidden="true">
                            {n > 1 ? n : ''}
                          </span>
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="muted comments__count" aria-live="polite">
        Показано {shown.length} из {comments.length}
      </p>
      {shown.length === 0 ? (
        <p className="muted">Нет комментариев с такими фильтрами.</p>
      ) : (
        <ol className="comments">
          {shown.map((c) => {
            const tone = toneOf(c.analysis.sentiment);
            return (
              <li key={c.response_id + c.question} className={`comment comment--${c.role}`}>
                <p className="comment__meta">
                  <b>
                    {ROLE_TEXT[c.role]}, M{c.period}
                  </b>
                  <span className={`tone tone--${tone}`}>{TONE_TEXT[tone]}</span>
                  {c.analysis.analyzer.startsWith('llm:') ? (
                    <span className="engine engine--llm" title={`Разбор моделью ${c.analysis.analyzer.slice(4)}`}>
                      Модель
                    </span>
                  ) : (
                    <span className="engine" title="Разбор по словарю тем">
                      Словарь
                    </span>
                  )}
                  {c.analysis.masked && c.analysis.masked.length > 0 && (
                    <span className="masked" title="Персональные данные заменены метками до отправки комментария в модель">
                      Перед моделью скрыто: {c.analysis.masked.join(', ')}
                    </span>
                  )}
                </p>
                <p className="comment__text">{highlight(c.text, c.analysis.quote)}</p>
                {c.analysis.themes.length > 0 && (
                  <p className="chips">
                    {c.analysis.themes.map((t) => (
                      <span key={t} className={`chip ${themes[t]?.negative ? 'chip--neg' : 'chip--pos'}`}>
                        {label(t)}
                      </span>
                    ))}
                  </p>
                )}
                {c.analysis.note && <p className="comment__note">{noteText(c.analysis.note)}</p>}
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}

/** Заметка разбора: при отклонённом ответе модели — причина и чем заменили. */
export function noteText(note: string) {
  if (!note.includes('отклонён')) return note;
  return note.includes('словарь') ? note : `${note}, использован словарь`;
}

/** Подсветка цитаты-основания внутри комментария. */
function highlight(text: string, quote: string) {
  const i = quote ? text.indexOf(quote) : -1;
  if (i < 0) return text;
  return (
    <>
      {text.slice(0, i)}
      <mark>{quote}</mark>
      {text.slice(i + quote.length)}
    </>
  );
}
