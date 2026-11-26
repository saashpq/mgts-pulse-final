import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { MIN_DECISIONS, api, isStaticDemo, type SignalPrecision, type Thresholds as T } from '../api';
import { commaNum, ratioCI } from '../format';
import { THRESHOLD_KEYS, invalidate } from '../queries';
import { Failure, Loading } from './common';

export interface Field {
  key: string;
  label: string;
  step: number;
  signal: string;
  /** одна строка «что это значит» */
  hint?: string;
  min?: number;
  max?: number;
}

export const FIELDS: Field[] = [
  {
    key: 'cost_ratio',
    label: 'Сколько лишних разговоров HR готов провести, чтобы не пропустить одного стажёра',
    step: 1,
    signal: 'калибровка S1, S2',
    hint: 'Это ориентир для ручного выбора порогов. В имитации при цене 10 один пропуск весит столько же, сколько десять лишних разговоров. При такой цене наименьшую стоимость ошибок дал стартовый набор порогов.',
    min: 0.5,
    max: 100,
  },
  {
    key: 'z_drop',
    label: 'Резкий провал: во сколько раз падение больше обычного колебания пары',
    step: 0.1,
    signal: 'S1, S2',
    hint: 'При пороге 3 сигнал появится, если оценка упадёт на три обычных колебания пары или больше.',
    min: 1,
    max: 5,
  },
  {
    key: 'decline_slope',
    label: 'Устойчивое снижение: наклон за 4 месяца, балла в месяц',
    step: 0.05,
    signal: 'S1, S2',
    hint: 'При пороге −0,25 система ищет снижение со скоростью не менее четверти балла в месяц.',
  },
  {
    key: 'decline_from_base',
    label: 'Оценка ниже обычного уровня пары не меньше чем на (баллов)',
    step: 0.1,
    signal: 'S1, S2',
    hint: 'Для сигнала об устойчивом снижении должны выполняться оба условия — темп снижения и отклонение от обычной оценки.',
    min: 0.1,
    max: 2,
  },
  { key: 'low_score', label: 'Ниже этой оценки — критично', step: 0.1, signal: 'S1, S2' },
  { key: 'gap_min', label: 'Расхождение стажёра и наставника', step: 0.1, signal: 'S3' },
  { key: 'gap_high', label: 'Критичное расхождение', step: 0.1, signal: 'S3' },
  { key: 'persist', label: 'Сколько месяцев подряд, чтобы это был не шум', step: 1, signal: 'S3, S4, S8' },
  { key: 'blind_self', label: '«Слепое пятно»: одна сторона ставит не ниже', step: 0.5, signal: 'S4' },
  { key: 'blind_other', label: '«Слепое пятно»: другая сторона ставит не выше', step: 0.5, signal: 'S4' },
  { key: 'low_dimension', label: 'Поддержка или понятность задач не выше', step: 0.1, signal: 'S6' },
  { key: 'interest_drop', label: 'Падение интереса к задачам от первого месяца', step: 0.1, signal: 'S7' },
  { key: 'neg_sentiment', label: 'Негативная тональность комментария (≤)', step: 0.05, signal: 'S7' },
  { key: 'mentor_low', label: 'Наставник оценивает стажёра не выше', step: 0.1, signal: 'S8' },
  { key: 'short_ratio', label: 'Комментарий короче этой доли от обычного', step: 0.05, signal: 'S12' },
  { key: 'diverge', label: 'Разница наклонов «разных траекторий»', step: 0.05, signal: 'S5' },
];

const groups = [
  { title: 'Снижение оценок', keys: ['z_drop', 'decline_slope', 'decline_from_base', 'low_score'] },
  { title: 'Расхождения между стажёром и наставником', keys: ['gap_min', 'gap_high', 'blind_self', 'blind_other', 'diverge'] },
  { title: 'Поддержка, интерес и обратная связь', keys: ['low_dimension', 'interest_drop', 'neg_sentiment', 'mentor_low', 'short_ratio'] },
  { title: 'Длительность сигнала', keys: ['persist'] },
  { title: 'Цена пропуска сигнала', keys: ['cost_ratio'] },
];

type Form = Record<string, string>;

const toForm = (t: T): Form => Object.fromEntries(Object.entries(t).map(([k, v]) => [k, String(v)]));
const sameForm = (left: Form, right: Form) =>
  Object.keys(left).length === Object.keys(right).length && Object.keys(left).every((key) => left[key] === right[key]);
/** Поле порога валидно, если это конечное число; пустое поле — ошибка, а не 0. */
export const validThreshold = (v: string | undefined) => v !== undefined && v.trim() !== '' && Number.isFinite(Number(v));
/** Ошибка поля: пусто/не число или вне допустимого диапазона (как проверяет сервер). */
export function thresholdError(f: Pick<Field, 'min' | 'max'>, v: string | undefined): string {
  if (!validThreshold(v)) return 'Введите число';
  const n = Number(v);
  if ((f.min !== undefined && n < f.min) || (f.max !== undefined && n > f.max)) return `Допустимо от ${num(f.min)} до ${num(f.max)}`;
  return '';
}
const toThresholds = (f: Form): T => Object.fromEntries(Object.entries(f).map(([k, v]) => [k, Number(v)]));

/** Пороги сигналов. На 5 парах × 6 месяцев их не обучить — это стартовые значения, HR правит их под программу. */
export function Thresholds() {
  const demo = isStaticDemo();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['thresholds'], queryFn: api.thresholds });
  // значения формы — строки: пустое поле остаётся пустым и не превращается в 0
  const [draft, setDraft] = useState<{ base: Form; values: Form } | null>(null);
  const [applyError, setApplyError] = useState('');
  const onSaved = async (res: { thresholds: T }) => {
    await qc.cancelQueries({ queryKey: ['thresholds'], exact: true });
    qc.setQueryData(['thresholds'], (old: typeof q.data) => (old ? { ...old, thresholds: res.thresholds } : old));
    setDraft(null);
    invalidate(qc, THRESHOLD_KEYS.filter((key) => key !== 'thresholds'));
  };
  const save = useMutation({ mutationFn: (t: T) => api.saveThresholds(t), onSuccess: onSaved });
  const apply = useMutation({ mutationFn: (t: T) => api.saveThresholds(t), onSuccess: onSaved });
  if (q.isError) return <Failure error={q.error} onRetry={() => q.refetch()} />;
  if (q.isPending) return <Loading />;
  const defaults = q.data.defaults;
  const saved = toForm(q.data.thresholds);
  const form = draft?.values ?? saved;
  const dirty = !sameForm(form, saved);
  const conflict = draft && dirty && !sameForm(draft.base, saved);
  if (draft && !dirty) setDraft(null);
  const setForm = (values: Form) => setDraft(sameForm(values, saved) ? null : { base: draft?.base ?? saved, values });
  const saving = save.isPending || apply.isPending;
  const fields = FIELDS.filter((f) => f.key in form);
  const invalid = fields.filter((f) => thresholdError(f, form[f.key]) !== '');

  // «Применить» из подсказки точности: текущая форма + одно поле; несохранённые правки — только с подтверждения
  const onApply = (s: NonNullable<SignalPrecision['suggest']>) => {
    setApplyError('');
    if (dirty) {
      if (invalid.length) {
        setApplyError('Сначала исправьте поля с ошибкой: подсказка сохраняется вместе с формой.');
        return;
      }
      if (!window.confirm('Есть несохранённые правки порогов. Сохранить их вместе с подсказкой?')) return;
    }
    const next = { ...form, [s.field]: String(s.to) };
    setForm(next);
    apply.mutate(toThresholds(next));
  };

  return (
    <>

    <section className="sheet" aria-labelledby="calibration-title">
      <div className="sheet__head">
        <h2 id="calibration-title">Как порог влияет на сигналы</h2>
        <span className="sheet__meta">Учебная имитация · S1 и S2</span>
      </div>
      <p>При пороге −0,50 балла в месяц система реже тревожит стабильные пары, но чаще пропускает снижение оценок.</p>
      <div className="chart-table__wrap">
        <table className="prec" aria-labelledby="calibration-title" aria-describedby="calibration-method">
          <thead><tr>
            <th scope="col">Результат к концу 6-го месяца</th>
            <th scope="col">−0,25 балла/мес. (стартовый)</th>
            <th scope="col">−0,50 балла/мес.</th>
          </tr></thead>
          <tbody>
            <tr><th scope="row">Ложная тревога при стабильных оценках</th><td>14 %</td><td>4 %</td></tr>
            <tr><th scope="row">Пропущено резкое падение</th><td>9 %</td><td>32 %</td></tr>
            <tr><th scope="row">Пропущено плавное снижение</th><td>2 %</td><td>21 %</td></tr>
          </tbody>
        </table>
      </div>
      <p id="calibration-method" className="muted">Каждая строка — отдельный сценарий из 3 000 синтетических рядов оценок одной роли. В обоих вариантах порог резкого провала равен 3, а минимальное снижение от обычной оценки — 0,5 балла.</p>
    </section>
    <section className="sheet" aria-labelledby="thr-title">
      <div className="sheet__head">
        <h2 id="thr-title">Пороги по типам сигналов</h2>
      </div>
      <p className="muted">
        {demo ? 'В демоверсии доступны для просмотра стартовые значения.' : 'После сохранения система пересчитает сигналы и тепловую карту.'}
      </p>
      <nav className="quality-jumps" aria-label="Группы порогов">
        {groups.filter(group => group.keys.some(key => key in form)).map(group =>
          <a key={group.title} href={'#rules-' + group.keys[0]}>{group.title}</a>)}
      </nav>
      <form
        className="thresholds"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          if (demo || saving || invalid.length) return;
          save.mutate(toThresholds(form));
        }}
      >
        {conflict && <div>
          <p role="alert">На сервере изменились пороги. Ваш черновик сохранён в форме. При сохранении он заменит текущие значения на сервере.</p>
          <button type="button" className="btn btn--ghost" disabled={saving} onClick={() => setDraft(null)}>Загрузить пороги с сервера</button>
        </div>}
        {groups.map(({ title, keys }) => {
          const members = fields.filter((f) => keys.includes(f.key));
          if (!members.length) return null;
          return (
            <section key={title}>
              <h3 id={'rules-' + keys[0]}>{title}</h3>
              {members.map((f) => {
                const err = thresholdError(f, form[f.key]);
                const bad = err !== '';
                const errId = `thr-err-${f.key}`;
                const hintId = `thr-hint-${f.key}`;
                return (
                  <div key={f.key} className={`thr${bad ? ' thr--bad' : ''}`}>
                    <label htmlFor={`thr-${f.key}`}>
                      {f.label} <span className="muted">· {f.signal}</span>
                    </label>
                    <input
                      id={`thr-${f.key}`}
                      type="number"
                      step={f.step}
                      readOnly={demo}
                      disabled={saving}
                      required={!demo}
                      inputMode="decimal"
                      value={form[f.key]}
                      min={f.min}
                      max={f.max}
                      aria-invalid={bad || undefined}
                      aria-describedby={bad ? errId : f.hint ? hintId : undefined}
                      onChange={(e) => setForm({ ...form, [f.key]: e.target.value })}
                    />
                    <span className="muted">по умолчанию {num(defaults[f.key])}</span>
                    {f.hint && (
                      <p id={hintId} className="thr__hint">
                        {f.hint}
                      </p>
                    )}
                    {bad && (
                      <p id={errId} className="error thr__error" role="alert">
                        {err}
                      </p>
                    )}
                  </div>
                );
              })}
            </section>
          );
        })}
        {!demo && <div className="thresholds__actions">
          <button className="btn btn--primary" disabled={saving || invalid.length > 0}>
            Сохранить и пересчитать
          </button>
          <button type="button" className="btn btn--ghost" disabled={saving} onClick={() => setForm(toForm(defaults))}>
            Вернуть стартовые
          </button>
          {invalid.length > 0 && <span className="error">Не сохранить: {invalid.length} {invalid.length === 1 ? 'поле' : 'поля'} с ошибкой</span>}
          {dirty && invalid.length === 0 && !save.isPending && <span className="muted">Есть несохранённые правки</span>}
          {save.isSuccess && !dirty && <span className="muted" role="status">Сохранено</span>}
          {save.isError && <span className="error" role="alert">{(save.error as Error).message}</span>}
        </div>}
      </form>
    </section>
    <Precision onApply={onApply} applying={saving} error={applyError || (apply.error as Error | null)?.message || ''} />
    <div className="flow-links"><a href="/signals">Проверить текущие сигналы →</a></div>
    </>
  );
}

const fieldLabel = (key: string) => FIELDS.find((f) => f.key === key)?.label ?? key;
function num(v: number | undefined) {
  return v === undefined ? '—' : String(v).replace('.', ',');
}

/** Точность по решениям HR: какая доля сигналов каждого типа оказалась по делу. */
function Precision({ onApply, applying, error }: { onApply: (s: NonNullable<SignalPrecision['suggest']>) => void; applying: boolean; error: string }) {
  const ov = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const list = ov.data?.report.precision ?? [];
  useEffect(() => {
    if (list.length && location.hash === '#prec-title') document.getElementById('prec-title')?.focus();
  }, [list.length]);
  if (!list.length) return null;
  const decided = list.filter((x) => x.precision !== null);
  return (
    <section className="sheet" aria-labelledby="prec-title">
      <div className="sheet__head">
        <h2 id="prec-title" tabIndex={-1}>Точность по решениям HR</h2>
        <span className="sheet__meta">точность = (подтверждено + решено) / все решения</span>
      </div>
      <p className="muted">
        {isStaticDemo() ? 'Таблица показывает решения HR, включённые в демонстрационный набор.' : decided.length
          ? 'Когда HR отмечает сигнал ложным, это учитывается здесь. Если ложных много, система предложит загрубить порог — применяется одной кнопкой.'
          : 'Решений HR по сигналам пока нет. Отмечайте сигналы «Подтвердить», «Решён» или «Ложный» — здесь появится точность по каждому типу.'}
      </p>
      {decided.length > 0 && (
        <p className="muted">
          «95% ДИ» — диапазон, в котором с высокой уверенностью лежит настоящая точность. На 5 решениях он широкий: 4/5 может означать и 28%, и 99%.
        </p>
      )}
      <div className="chart-table__wrap">
        <table className="prec">
          <thead>
            <tr>
              <th scope="col">Сигнал</th>
              <th scope="col" className="num">Подтверждено</th>
              <th scope="col" className="num">Решено</th>
              <th scope="col" className="num">Ложно</th>
              <th scope="col" className="num">Точность</th>
              <th scope="col">Что сделать</th>
            </tr>
          </thead>
          <tbody>
            {list.map((x) => (
              <tr key={x.type} className={x.suggest || x.hint ? 'prec__row--warn' : undefined}>
                <th scope="row">
                  <span className="prec__type">{x.type}</span> {x.title}
                </th>
                <td className="num">{x.confirmed || '—'}</td>
                <td className="num">{x.resolved || '—'}</td>
                <td className="num">{x.false || '—'}</td>
                <td className="num">
                  {x.precision === null ? (
                    <span className="muted">нет решений</span>
                  ) : x.enough === false ? (
                    <span className="muted" title={`Точность показывается от ${MIN_DECISIONS} решений`}>
                      мало решений
                    </span>
                  ) : (
                    <b className={x.precision < 0.5 ? 'prec__low' : undefined}>
                      {ratioCI(x.confirmed + x.resolved, x.decisions ?? x.confirmed + x.resolved + x.false, x.precision, x.lo, x.hi)}
                    </b>
                  )}
                </td>
                <td className="prec__hint">
                  {x.hint && <p>{commaNum(x.hint)}</p>}
                  {x.suggest && !isStaticDemo() && (
                    <button
                      type="button"
                      className="btn btn--ghost btn--small"
                      disabled={applying}
                      title={`${fieldLabel(x.suggest.field)}: ${num(x.suggest.from)} → ${num(x.suggest.to)}`}
                      onClick={() => onApply(x.suggest!)}
                    >
                      Применить: {num(x.suggest.to)}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
