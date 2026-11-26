import { useEffect, type MouseEvent, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type ActionRow, type Effect, type Evaluation, type Overview as OverviewData, type PairReport, type PrecisionTotal, type Quality as Q } from '../api';
import { BlandAltmanChart, ShareBars, baPoints } from '../components/charts';
import { BlindReviewDownload } from '../components/BlindReviewDownload';
import { IconArrow, IconPlan } from '../components/icons';
import { VERDICT_TEXT, biasText, fmt, kappaText, pct, plural, ratioCI, signed } from '../format';
import { navigate, pairMonthPath, pairPath } from '../router';
import { deltaText } from './PairView';
import { Failure, Loading } from './common';
import { totalPrecision } from './Overview';
import { ValidationResults } from '../components/ValidationResults';
import './quality-report.css';

/** Подпись, если сервер её не прислал: N — число пар в отчёте. */
export const defaultCaveat = (n: number) =>
  `Метрики на ${n} ${plural(n, 'паре', 'парах', 'парах')}; интервалы широкие. Сигнал — повод для разговора, решение принимает HR.`;

const TRUTH_TEXT = {
  synthetic: {
    title: 'По учебным примерам.',
    text: 'В каждом примере заранее известно, была ли у пары проблема и когда она началась. Поэтому можно посчитать и найденные, и пропущенные проблемы.',
  },
  hr_verdicts: {
    title: 'По решениям HR.',
    text: 'HR оценивает показанные сигналы. Чтобы узнать, сколько проблем пропущено, нужна независимая проверка пар, включая пары без тревоги.',
  },
} as const;

/** Метрика: название, значение и одна строка «что это значит». */
function Metric({ label, value, means }: { label: string; value: ReactNode; means: ReactNode }) {
  return (
    <div className="qm__item">
      <dt className="qm__label">{label}</dt>
      <dd className="qm__value">{value}</dd>
      <dd className="qm__means">{means}</dd>
    </div>
  );
}

const NoData = ({ children }: { children: ReactNode }) => <p className="q-nodata">{children}</p>;

function followReportLink(event: MouseEvent<HTMLAnchorElement>) {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
  event.preventDefault();
  navigate(event.currentTarget.pathname + event.currentTarget.search + event.currentTarget.hash);
}

function revealQualityAnchor() {
  const id = window.location.hash.slice(1);
  if (!id.startsWith('q-')) return;
  const target = document.getElementById(id);
  if (!target?.closest('.quality-report')) return;
  const panel = target.closest('details');
  if (panel) panel.open = true;
  target.focus({ preventScroll: true });
  target.scrollIntoView?.({ block: 'start' });
}

function QualityJump({ to, children }: { to: string; children: ReactNode }) {
  return <a href={to} onClick={(event) => {
    if (to === window.location.hash && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) revealQualityAnchor();
  }}>{children}</a>;
}

function QualityPanel({ id, title, result, source, children }: {
  id: string; title: string; result: string; source?: ReactNode; children: ReactNode;
}) {
  return (
    <section className="quality-panel" aria-labelledby={id + '-title'}>
      <details>
        <summary id={id} aria-labelledby={id + '-title'} aria-describedby={id + '-result'}>
          <span className="quality-panel__title" id={id + '-title'}>{title}</span>
          <span className="quality-panel__result" id={id + '-result'}>{result}</span>
          <span className="quality-chevron" aria-hidden="true" />
        </summary>
        <div className="quality-panel__body">
          {source && <p className="quality-source"><b>Источник</b> {source}</p>}
          {children}
        </div>
      </details>
    </section>
  );
}

function QualityVerdict({ overview }: { overview: OverviewData }) {
  const { report, data_origin: origin } = overview;
  const quality = report.quality;
  const synthetic = quality?.ground_truth === 'synthetic' ? quality.evaluation : null;
  const submitted = origin === 'submitted' && report.effect && report.effect.active_signals > 0 ? report.effect : null;
  const knownEpisodes = synthetic && synthetic.episodes > 0 ? synthetic : null;
  const score = knownEpisodes ? [knownEpisodes.found, knownEpisodes.episodes] : submitted ? [submitted.decided_signals, submitted.active_signals] : null;
  const threshold = (report.low_score ?? 3).toLocaleString('ru-RU');
  const origins = {
    synthetic: 'Точность на рабочих парах пока неизвестна.',
    mixed: 'В отчёте объединены учебные и загруженные анкеты. Отдельной оценки загруженных пар нет.',
    submitted: 'Решения по показанным сигналам помогают оценить лишние тревоги. Для поиска пропусков нужна независимая разметка.',
    empty: 'Когда поступят первые ответы, сравните сигналы с анкетами участников.',
  };
  const noAnswers = origin === 'empty' || report.pairs.length === 0;
  return (
    <section className="quality-verdict" aria-labelledby="q-summary">
      <div className="quality-verdict__finding">
        <p className="quality-source-label"><IconPlan /> {knownEpisodes ? 'Учебная проверка сигналов' : submitted ? 'Загруженные анкеты' : 'Проверка сигналов'}</p>
        <h2 id="q-summary" tabIndex={-1} className="quality-verdict__headline">
          {score ? <>
            <span className="quality-verdict__score">
              {score[0]} <small>из</small> {score[1]}
            </span>
            <span>{knownEpisodes ? `${plural(knownEpisodes.episodes, 'учебного эпизода', 'учебных эпизодов', 'учебных эпизодов')} с сигналом` : 'активных сигналов с решением HR'}</span>
          </> : <span>Для оценки обнаружения пока недостаточно данных</span>}
        </h2>
        <p className="quality-verdict__comparison">
          {knownEpisodes?.naive
            ? `Правило «средняя оценка ниже ${threshold}» обнаружило ${knownEpisodes.naive.found} из ${knownEpisodes.episodes}.`
            : submitted ? 'Учитываются сохранённые решения куратора по активным сигналам.'
            : knownEpisodes ? 'Начало каждого учебного эпизода задано заранее.'
            : 'Нужна выборка пар с заранее размеченными проблемами.'}
        </p>
        {knownEpisodes && <p className="quality-verdict__scope">Засчитано совпадение пары и срока. Совпадение причины сигнала с темой эпизода отдельно не проверялось.</p>}
        <div className="quality-verdict__links">
          <QualityJump to="#q-early">Как считали</QualityJump>
          {!noAnswers && <QualityJump to="#q-load">Нагрузка на HR</QualityJump>}
        </div>
      </div>
      <div className="quality-verdict__next">
        <h3>{noAnswers ? 'С чего начать' : 'Что проверить HR'}</h3>
        <p>{origin ? origins[origin] : 'Независимая оценка на рабочих парах в отчёте не указана.'}
          {!noAnswers && ' Пусть два куратора независимо проверят пары с тревогами и без них.'}
        </p>
        <div className="quality-actions">
          {noAnswers ? <a className="quality-action" href="/sources" onClick={followReportLink}>Открыть источники данных <IconArrow /></a>
            : <BlindReviewDownload pairs={report.pairs.map(pair => pair.pair)} />}
        </div>
      </div>
    </section>
  );
}

export function Quality() {
  const q = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  if (q.isPending) return <Loading />;
  if (q.isError) return <Failure error={q.error} onRetry={() => q.refetch()} />;
  return <QualityReport overview={q.data} />;
}

function QualityReport({ overview }: { overview: OverviewData }) {
  const r = overview.report;
  const ql: Q | null = r.quality ?? null;
  const caveat = ql?.caveat?.trim() || defaultCaveat(r.pairs.length);
  const prec = r.precision_total ?? totalPrecision(r.precision ?? []);
  const truth = ql?.ground_truth ? TRUTH_TEXT[ql.ground_truth] : null;
  useEffect(() => {
    revealQualityAnchor();
    window.addEventListener('hashchange', revealQualityAnchor);
    window.addEventListener('popstate', revealQualityAnchor);
    return () => {
      window.removeEventListener('hashchange', revealQualityAnchor);
      window.removeEventListener('popstate', revealQualityAnchor);
    };
  }, []);
  return (
    <div className="quality-report">
      <div>
        <QualityVerdict overview={overview} />
        <p className="quality-verdict-note" role="note" aria-label="Оговорка">{caveat}</p>
      </div>
      <TextAi overview={overview} />
      <section className="quality-group" aria-labelledby="q-operation">
        <h2 id="q-operation" tabIndex={-1}>Сигналы и решения HR</h2>
        <div className="quality-panels">
          <EarlyWarning ev={ql?.evaluation ?? null} truth={ql?.ground_truth} prec={prec} threshold={(r.low_score ?? 3).toLocaleString('ru-RU')}
            source={truth ? <><b>{truth.title}</b> {truth.text}</> : 'В отчёте это не указано.'} />
          <AlertLoad load={ql?.alert_load ?? null} effect={r.effect} />
          {r.reaction && <QualityPanel id="q-reaction" title="Время до первого решения HR"
            result={r.reaction.median_hours == null ? 'Замеров пока нет' : `Медиана · ${r.reaction.median_hours.toLocaleString('ru-RU')} ч`}
            source="Даты поступления анкет и первого сохранённого решения HR.">
            <p>Из {r.reaction.total} эпизодов измерено {r.reaction.measured}, без решения {r.reaction.pending}, с неполными датами {r.reaction.unknown}.</p>
            <p>Отсчёт начинается с последнего ответа пары в месяце начала эпизода. Для каждого сигнала учитывается последний эпизод. Дату встречи куратор фиксирует отдельно.</p>
          </QualityPanel>}
          <ActionEffect ae={ql?.action_effect ?? null} rows={r.actions ?? []} />
        </div>
      </section>
      <section className="quality-group" aria-labelledby="q-surveys">
        <h2 id="q-surveys" tabIndex={-1}>Оценки в анкетах</h2>
        <div className="quality-panels">
          <Scales sc={ql?.scales ?? null} />
          <AgreementSection ag={ql?.agreement ?? null} icc={ql?.icc_note} pairs={r.pairs} />
        </div>
      </section>
      {!ql && <NoData>В этом отчёте доступны только отдельные показатели. Для полного отчёта нужна версия системы с расчётом качества.</NoData>}
      <div className="quality-footer"><a href="/thresholds" onClick={followReportLink}>Посмотреть пороги сигналов <IconArrow /></a></div>
    </div>
  );
}

const months = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${fmt(v, v % 1 ? 1 : 0)} мес.`);

type EpisodeRow = { pair: string; role: string; class: string; t0: number; ours: number | null; naive: number | null };
type DetailedEvaluation = Evaluation & { window?: number; rows?: EpisodeRow[]; neither?: number };
const episodeNames: Record<string, string> = {
  motivation: 'Мотивация', support: 'Поддержка наставника', performance: 'Выполнение задач',
  divergence: 'Расхождение оценок', dip: 'Снижение оценок',
};

function EpisodeEvidence({ rows }: { rows: EpisodeRow[] }) {
  return <details className="quality-archive">
    <summary>Исходные анкеты по учебным эпизодам ({rows.length})</summary>
    <ul className="quality-episodes">
      {rows.map(row => {
        const side = row.role === 'intern' || row.role === 'mentor' ? row.role : null;
        const label = side === 'intern' ? 'стажёр' : side === 'mentor' ? 'наставник' : 'обе стороны';
        const href = pairMonthPath(row.pair, row.t0) + (side ? '#month-' + side : '');
        return <li key={[row.pair, row.role, row.class, row.t0].join('/')}>
          <div>
            <a href={href} onClick={followReportLink}>{row.pair} · M{row.t0} · {label}</a>
            <span>{episodeNames[row.class] ?? row.class}</span>
          </div>
          <p>Сигнал {row.ours != null ? 'в M' + row.ours : 'не появился в окне проверки'}. Правило средней оценки {row.naive != null ? 'сработало в M' + row.naive : 'не сработало в окне проверки'}.</p>
        </li>;
      })}
    </ul>
  </details>;
}

function EarlyWarning({ ev, truth, prec, threshold, source }: {
  ev: DetailedEvaluation | null; truth?: Q['ground_truth']; prec: PrecisionTotal; threshold: string; source: ReactNode;
}) {
  const good = prec.precision === null ? 0 : Math.round(prec.precision * prec.decisions);
  const falsePairs = Array.isArray(ev?.false_pairs) ? ev.false_pairs : null;
  const windowMonths = ev?.window ?? 2;
  return (
    <QualityPanel id="q-early" title="Раннее предупреждение" source={source}
      result={truth === 'synthetic' && ev ? `${ev.found} из ${ev.episodes} учебных эпизодов` : 'Полнота обнаружения пока неизвестна'}>
      {truth === 'hr_verdicts' && ev ? (
        <p className="q-nodata">По решениям HR известно эпизодов: {ev.episodes}. Для оценки полноты и опережения попросите двух HR независимо отметить начало проблем по исходным анкетам до просмотра сигналов.</p>
      ) : truth === 'synthetic' && ev ? (
        <>
          <dl className="qm">
            <Metric
              label="Полнота: найдено проблем"
              value={ratioCI(ev.found, ev.episodes, ev.recall.value, ev.recall.lo, ev.recall.hi)}
              means={`Какую долю известных проблем система заметила по сигналу у той же пары в месяц начала или не позднее чем через ${windowMonths} ${plural(windowMonths, 'месяц', 'месяца', 'месяцев')}. Тема сигнала может отличаться от темы эпизода.`}
            />
            <Metric
              label="Тревоги, совпавшие с учебной разметкой по паре"
              value={ratioCI(ev.true_alerts, ev.alerts, ev.precision.value, ev.precision.lo, ev.precision.hi)}
              means="Тревога засчитывается, если у пары есть учебный эпизод и сигнал появился не раньше чем за месяц до первого эпизода. Причина каждой тревоги с разметкой не сверяется."
            />
            <Metric
              label="Задержка (медиана)"
              value={months(ev.median_delay)}
              means="Сколько месяцев прошло от начала найденного эпизода до первого сигнала. Ноль означает тот же месяц."
            />
            <Metric
              label="Тревоги на стабильных парах"
              value={falsePairs ? (falsePairs.length ? falsePairs.join(', ') : 'нет') : typeof ev.false_pairs === 'number' ? ev.false_pairs : '—'}
              means="Учебные пары без размеченных проблем, у которых появилась тревога."
            />
          </dl>
          <h3 className="q-h3">Сравнение с правилом «средняя оценка ниже {threshold}»</h3>
          <div className="table-scroll">
            <table className="q-table">
              <thead>
                <tr>
                  <th scope="col">Показатель</th>
                  <th scope="col" className="num">Наша система</th>
                  <th scope="col" className="num">Правило «средняя &lt; {threshold}»</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <th scope="row">Найдено проблем</th>
                  <td className="num">
                    {ev.found} из {ev.episodes}
                  </td>
                  <td className="num">{ev.naive ? `${ev.naive.found} из ${ev.episodes}` : '—'}</td>
                </tr>
                <tr>
                  <th scope="row">Задержка (медиана)</th>
                  <td className="num">{months(ev.median_delay)}</td>
                  <td className="num">{ev.naive ? months(ev.naive.median_delay) : '—'}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <h3 className="q-h3" id="q-timing">Когда два способа обнаружили учебные эпизоды</h3>
          <dl className="quality-timing" aria-labelledby="q-timing">
            {Object.entries({
              'Раньше правила': ev.earlier, 'Одновременно': ev.same, 'Позже правила': ev.later,
              'Только наша система': ev.only_ours, 'Только правило': ev.only_naive,
              ...(ev.neither ? { 'Оба пропустили': ev.neither } : {}),
            }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value ?? '—'}</dd></div>)}
          </dl>
          <p className="qm__means">Сигналы о пропущенной или формально заполненной анкете в расчёт раннего обнаружения не входят.</p>
          {!!ev.rows?.length && <EpisodeEvidence rows={ev.rows} />}
        </>
      ) : (
        <NoData>
          {truth === 'hr_verdicts'
            ? 'Не с чем сравнить: HR ещё не подтвердил ни одного сигнала. Отмечайте в карточках «Подтвердить» или «Решено» — подтверждённые сигналы станут эпизодами для оценки.'
            : truth === 'synthetic'
              ? 'В отчёте нет результатов проверки на учебных примерах. Для такой проверки сравнивают дату первого сигнала с заданным началом проблемы.'
              : 'В отчёте нет данных о том, сколько проблем система обнаружила и как быстро это сделала.'}
        </NoData>
      )}
      <dl className="qm qm--one">
        <Metric
          label="Подтверждено по решениям HR"
          value={
            prec.decisions === 0 || prec.precision === null ? (
              <span className="muted">решений пока нет</span>
            ) : !prec.enough ? (
              <span className="muted">мало решений ({prec.decisions} из 5)</span>
            ) : (
              ratioCI(good, prec.decisions, prec.precision, prec.lo, prec.hi)
            )
          }
          means="Доля подтверждённых или решённых среди сигналов с решением HR. Интервал отражает неопределённость из-за размера этой выборки."
        />
      </dl>
    </QualityPanel>
  );
}

function AlertLoad({ load, effect }: { load: Q['alert_load'] | null; effect?: Effect }) {
  return (
    <QualityPanel id="q-load" title="Нагрузка на HR"
      result={load ? `${load.active_cards} ${plural(load.active_cards, 'открытая карточка', 'открытые карточки', 'открытых карточек')}` : 'Нет подсчёта карточек'}
      source="Открытые сигналы и сохранённые решения по парам текущего месяца.">
      {load ? (
        <dl className="qm">
          <Metric label="Активных карточек" value={load.active_cards} means="Сколько пар сейчас ждут реакции HR (одна карточка на пару)." />
          <Metric
            label="На 10 пар в месяц"
            value={fmt(load.per_10_pairs, 1)}
            means="Число открытых карточек в текущем месяце в пересчёте на 10 пар."
          />
          <Metric label="Вмешательство или наблюдение" value={pct(load.attention_share)} means="Доля пар с этими двумя статусами. У остальных пар могут быть плановые действия по сигналам низкой критичности." />
          <Metric
            label="Повторные сигналы"
            value={load.flapping?.length ? load.flapping.join(', ') : 'нет'}
            means="Сигнал исчезал и снова появлялся. Такие случаи помогают куратору проверить, подходит ли порог."
          />
        </dl>
      ) : (
        <NoData>В отчёте нет показателей нагрузки на HR.</NoData>
      )}
      {effect && (
        <dl className="qm">
          <Metric
            label="Покрытие решениями HR"
            value={effect.verdict_coverage === null ? <span className="muted">нет активных сигналов</span> : pct(effect.verdict_coverage)}
            means={`По ${effect.decided_signals} из ${effect.active_signals} активных сигналов HR принял решение.`}
          />
          <Metric
            label="Просрочено"
            value={effect.overdue === null ? <span className="muted">нет данных о датах</span> : effect.overdue}
            means="Срок реакции прошёл, решения HR нет."
          />
        </dl>
      )}
      <a className="quality-source-link" href="/signals" onClick={followReportLink}>Открыть сигналы <IconArrow /></a>
    </QualityPanel>
  );
}

function Scales({ sc }: { sc: Q['scales'] | null }) {
  const th = sc?.ceiling_threshold ?? 0.15;
  return (
    <QualityPanel id="q-scales" title="Данные и шкалы"
      result={sc?.flat_share != null ? `${pct(sc.flat_share)} анкет с одинаковыми оценками` : 'Как участники используют шкалу от 1 до 5'}
      source="Числовые ответы стажёров и наставников по месяцам.">
      {sc ? (
        <>
          <dl className="qm">
            <Metric
              label="Обычное колебание оценки"
              value={sc.noise_sigma === null ? '—' : `${fmt(sc.noise_sigma)} балла`}
              means="Размер обычного изменения индекса между соседними месяцами. Система использует его при поиске резкого снижения."
            />
            <Metric
              label="Анкеты с одинаковыми оценками"
              value={sc.flat_share === null || sc.flat_share === undefined ? '—' : pct(sc.flat_share)}
              means="Все оценки в анкете одинаковые — возможно, анкету заполнили формально."
            />
          </dl>
          <h3 className="q-h3">Потолок шкалы: доля максимальных оценок</h3>
          <p className="qm__means">Если больше {pct(th)} ответов получили максимум, различать участников с высокими оценками труднее.</p>
          {sc.ceiling?.length ? (
            <ShareBars rows={sc.ceiling.map((c) => ({ name: c.text ? `${c.role === 'mentor' ? 'Наставник' : 'Стажёр'}: ${c.text}` : c.question, share: c.share }))} threshold={th} label={`Доля максимальных оценок по вопросам, порог ${pct(th)}`} />
          ) : (
            <NoData>Нет данных по вопросам.</NoData>
          )}
        </>
      ) : (
        <NoData>В отчёте нет данных о том, как участники использовали шкалу оценок.</NoData>
      )}
    </QualityPanel>
  );
}

function AgreementSection({ ag, icc, pairs }: { ag: Q['agreement'] | null; icc?: string; pairs: PairReport[] }) {
  const pts = baPoints(pairs);
  const perPair = pairs.filter((p) => p.agreement);
  return (
    <QualityPanel id="q-agree" title="Согласие сторон"
      result={ag ? `${ag.n} ${plural(ag.n, 'парное наблюдение', 'парных наблюдения', 'парных наблюдений')}` : 'Недостаточно совместных наблюдений'}
      source="Месяцы, в которых ответили оба участника пары. Из индекса стажёра вычитается индекс наставника.">
      {ag ? (
        <>
          <dl className="qm">
            <Metric label="Смещение" value={signed(ag.bias, 2)} means={`${biasText(ag.bias)[0].toUpperCase()}${biasText(ag.bias).slice(1)}.`} />
            <Metric
              label="Границы согласия (95%)"
              value={`${signed(ag.loa_lo, 2)} … ${signed(ag.loa_hi, 2)}`}
              means="Расчётный диапазон разницы индексов по методу Бланда–Альтмана. Широкие границы указывают на большой разброс."
            />
            <Metric label="Наблюдений" value={ag.n} means={`Месяцев, где ответили обе стороны${ag.method ? ` · метод: ${ag.method}` : ''}.`} />
          </dl>
          {pts.length > 0 && <BlandAltmanChart points={pts} bias={ag.bias} lo={ag.loa_lo} hi={ag.loa_hi} />}
          {perPair.length > 0 && (
            <>
              <h3 className="q-h3">Смещение по парам</h3>
              <ul className="q-pairs">
                {perPair.map((p) => (
                  <li key={p.pair}>
                    <button type="button" className="pairs__name" onClick={() => navigate(pairPath(p.pair))}>
                      {p.pair}
                    </button>{' '}
                    <b>{signed(p.agreement!.bias, 2)}</b> <span className="muted">— {biasText(p.agreement!.bias)}</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      ) : (
        <NoData>В отчёте нет сравнения оценок стажёра и наставника. Для сравнения берут месяцы, когда ответили оба.</NoData>
      )}
      {icc && <p className="qm__means">{icc}</p>}
    </QualityPanel>
  );
}

const GOLD_LABELS: Record<string, [string, (v: unknown) => string]> = {
  date: ['Дата прогона', (v) => String(v)],
  model: ['Модель', (v) => String(v)],
  prompt_hash: ['Версия промпта', (v) => String(v)],
  macro_f1: ['Macro-F1 по темам', (v) => fmt(v as number)],
};

function TextAi({ overview }: { overview: OverviewData }) {
  const t = overview.report.quality?.text_ai;
  const review = overview.text_review;
  const firstUnclassified = review?.rows?.[0];
  const analyzer = overview.analyzer;
  const analyzerName = analyzer.label || (analyzer.mode === 'llm' ? analyzer.model || 'Языковая модель' : 'Словарь тем');
  const summary = review && review.total > 0 ? `У ${review.unclassified} из ${review.total} комментариев тема не определена.`
    : t?.total ? `В отчёте ${t.total} ${plural(t.total, 'разобранный комментарий', 'разобранных комментария', 'разобранных комментариев')}.`
    : 'Подробности разбора и независимой разметки.';
  return (
    <section className="quality-group" aria-label="ИИ-разбор комментариев">
      <h2 id="q-ai" tabIndex={-1}>Разбор комментариев</h2>
      <div className="quality-methods">
        <details className="quality-method">
          <summary id="q-analyzer" aria-labelledby="q-analyzer-label q-analyzer-name" aria-describedby="q-analyzer-result">
            <span className="quality-method__label" id="q-analyzer-label">В этой программе</span>
            <span className="quality-method__name" id="q-analyzer-name">{analyzerName}</span>
            <span className="quality-method__result" id="q-analyzer-result">{summary}</span>
            <span className="quality-chevron" aria-hidden="true" />
          </summary>
          <div className="quality-panel__body">
            <p className="quality-source"><b>Источник</b> Обработанные ответы участников в отчёте программы.</p>
            {firstUnclassified && <a className="quality-source-link"
              href={pairMonthPath(firstUnclassified.pair, firstUnclassified.period) + '#month-' + firstUnclassified.role} onClick={followReportLink}>
              Открыть анкету с комментарием без темы · {firstUnclassified.pair}, M{firstUnclassified.period} <IconArrow />
            </a>}
            <TextAnalysisMetrics t={t} goldset={overview.report.quality?.goldset} />
          </div>
        </details>
        <ValidationResults />
      </div>
    </section>
  );
}

function TextAnalysisMetrics({ t, goldset }: { t: Q['text_ai']; goldset: Q['goldset'] }) {
  const reasons = Object.entries(t?.reasons ?? {}).sort((a, b) => b[1] - a[1]);
  const reportUsedModel = Boolean(t?.analyzer?.startsWith('llm') || t?.model);
  return (
    <>
      {t ? (
        <>
          <dl className="qm">
            <Metric
              label="Режим анализа в отчёте"
              value={reportUsedModel ? `Модель ${t.model || t.analyzer?.replace(/^llm:/, '') || ''}`.trim() : 'Словарь тем'}
              means={reportUsedModel ? t.prompt_hash ? `Версия инструкции модели ${t.prompt_hash}.` : 'Версия инструкции модели в отчёте не указана.' : 'Словарь присваивает темы по выражениям, найденным в комментарии.'}
            />
            {reportUsedModel || t.rejected > 0 ? <Metric
              label="Отклонено проверкой"
              value={t.total ? `${t.rejected} из ${t.total} (${pct(t.rejected / t.total)})` : '0'}
              means="Если ответ модели нарушает формат, тема вне списка или цитаты нет в тексте, программа применяет словарь."
            /> : <Metric label="Комментариев в отчёте" value={t.total} means="Каждый ответ можно сверить с исходным текстом анкеты." />}
          </dl>
          {reasons.length > 0 && (
            <div className="table-scroll">
              <table className="q-table">
                <caption className="q-cap">Причины отклонения</caption>
                <thead>
                  <tr>
                    <th scope="col">Причина</th>
                    <th scope="col" className="num">Ответов</th>
                  </tr>
                </thead>
                <tbody>
                  {reasons.map(([k, n]) => (
                    <tr key={k}>
                      <th scope="row">{k}</th>
                      <td className="num">{n}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {(reportUsedModel || !!t.agreement?.length) && <h3 className="q-h3">Согласие модели и словаря по темам (κ)</h3>}
          {t.agreement?.length ? (
            <>
              <p className="qm__means">Каппа показывает, насколько часто модель и словарь совпадают сверх случайного уровня. Ноль означает случайное совпадение, единица — полное.</p>
              <div className="table-scroll">
                <table className="q-table">
                  <thead>
                    <tr>
                      <th scope="col">Тема</th>
                      <th scope="col" className="num">κ</th>
                      <th scope="col">Согласие</th>
                      <th scope="col" className="num">Оба</th>
                      <th scope="col" className="num">Только словарь</th>
                      <th scope="col" className="num">Только модель</th>
                    </tr>
                  </thead>
                  <tbody>
                    {t.agreement.map((a) => (
                      <tr key={a.theme}>
                        <th scope="row">{a.theme}</th>
                        <td className="num">{fmt(a.kappa)}</td>
                        <td className={a.kappa !== null && a.kappa < 0.4 ? 'prec__low' : undefined}>{kappaText(a.kappa)}</td>
                        <td className="num">{a.both}</td>
                        <td className="num">{a.only_offline}</td>
                        <td className="num">{a.only_llm}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : reportUsedModel ? <NoData>В отчёте пока недостаточно сопоставлений модели и словаря.</NoData> : null}
        </>
      ) : (
        <NoData>В отчёте нет показателей качества разбора комментариев.</NoData>
      )}
      <h3 className="q-h3">Сравнение с независимой разметкой</h3>
      {goldset ? (
        <dl className="qm">
          {Object.entries(goldset)
            .filter(([, v]) => v !== null && v !== undefined && typeof v !== 'object')
            .map(([k, v]) => {
              const [label, show] = GOLD_LABELS[k] ?? [k, (x: unknown) => (typeof x === 'number' ? fmt(x) : String(x))];
              return (
                <Metric
                  key={k}
                  label={label}
                  value={show(v)}
                  means={k === 'macro_f1' ? 'Среднее значение F1 по темам. Оно учитывает пропуски и лишние метки.' : ''}
                />
              );
            })}
        </dl>
      ) : (
        <NoData>
          Не проводилась для текущей программы по данным этого отчёта. Два куратора могут независимо разметить одни и те же комментарии и сравнить результат с выводами анализатора.
        </NoData>
      )}
    </>
  );
}

function ActionEffect({ ae, rows }: { ae: Q['action_effect'] | null; rows: ActionRow[] }) {
  const note = ae?.note || ae?.caveat || 'Без контрольной группы это наблюдение, а не доказательство.';
  return (
    <QualityPanel id="q-act" title="Эффект действий HR"
      result={ae?.measured ? `${ae.measured} ${plural(ae.measured, 'действие с замером', 'действия с замером', 'действий с замером')}` : 'Нет замеров после действий'}
      source="Журнал действий HR и анкета участника в следующем доступном месяце.">
      {ae ? (
        <dl className="qm">
          <Metric label="Решений с действием" value={ae.with_action} means="HR отметил, что сделано по сигналу (встреча, смена задач…)." />
          <Metric label="Есть замер после" value={ae.measured} means="Для скольких действий уже пришли анкеты следующего месяца." />
          <Metric
            label="Изменение индекса (медиана)"
            value={ae.median_delta === null ? '—' : `${signed(ae.median_delta)} балла`}
            means="Насколько индекс роли сигнала изменился после действия."
          />
        </dl>
      ) : (
        <NoData>Нет данных: при решении по сигналу отмечайте «Что сделано» — через месяц здесь появится изменение индекса.</NoData>
      )}
      <p className="q-note" role="note">
        {note}
      </p>
      {rows.length > 0 && (
        <div className="table-scroll">
          <table className="q-table">
            <caption className="q-cap">Журнал действий</caption>
            <thead>
              <tr>
                <th scope="col">Пара</th>
                <th scope="col">Сигнал</th>
                <th scope="col">Что сделано</th>
                <th scope="col">Результат</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.signal + r.period}>
                  <th scope="row">
                    <button type="button" className="pairs__name" onClick={() => navigate(pairPath(r.pair))}>
                      {r.pair}
                    </button>
                  </th>
                  <td>
                    {r.title} <span className="muted">· {VERDICT_TEXT[r.status] ?? r.status}</span>
                  </td>
                  <td>
                    M{r.period} · {r.action || <span className="muted">не указано</span>}
                  </td>
                  <td className={r.delta === null ? 'muted' : undefined}>{deltaText(r)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </QualityPanel>
  );
}
