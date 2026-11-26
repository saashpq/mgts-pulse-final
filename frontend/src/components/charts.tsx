import { useId, useState, type ReactNode } from 'react';
import type { Num } from '../api';
import { fmt } from '../format';

export interface Series {
  name: string;
  values: Num[];
  color: string;
  dashed?: boolean;
}

/** Таблица значений вместо графика: для скринридера и для тех, кому так удобнее читать числа. */
function ValuesTable({ caption, periods, rows }: { caption: string; periods: number; rows: { name: string; values: (string | number | null)[] }[] }) {
  return (
    <div className="chart-table__wrap">
      <table className="chart-table">
        <caption className="sr-only">{caption}</caption>
        <thead>
          <tr>
            <th scope="col">Ряд</th>
            {Array.from({ length: periods }, (_, i) => (
              <th scope="col" key={i}>
                M{i + 1}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.name}>
              <th scope="row">{r.name}</th>
              {Array.from({ length: periods }, (_, i) => {
                const v = r.values[i];
                return <td key={i}>{v === null || v === undefined ? '—' : typeof v === 'number' ? fmt(v, 1) : v}</td>;
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Рамка графика: переключатель «График / Таблица», легенда. */
function ChartFrame({
  label,
  periods,
  rows,
  legend,
  children,
}: {
  label: string;
  periods: number;
  rows: { name: string; values: (string | number | null)[] }[];
  legend: ReactNode;
  children: ReactNode;
}) {
  const [table, setTable] = useState(false);
  const id = useId();
  return (
    <figure className="chart" aria-labelledby={id}>
      <div className="chart__bar">
        <figcaption id={id} className="chart__legend">
          <span className="sr-only">{label}. </span>
          {legend}
        </figcaption>
        <button type="button" className="chart__toggle" aria-pressed={table} onClick={() => setTable((t) => !t)}>
          Таблица
        </button>
      </div>
      {table ? <ValuesTable caption={label} periods={periods} rows={rows} /> : children}
    </figure>
  );
}

/** Ключ серии в легенде: линия (сплошная/пунктир) и маркер (заливка/кольцо) — цвет не единственный признак. */
export function SeriesKey({ color, dashed }: { color: string; dashed?: boolean }) {
  return (
    <svg className="series-key" width="26" height="12" viewBox="0 0 26 12" aria-hidden="true">
      <line x1="1" x2="25" y1="6" y2="6" stroke={color} strokeWidth="2" strokeDasharray={dashed ? '4 3' : undefined} />
      <circle cx="13" cy="6" r="4" fill={dashed ? 'var(--sheet)' : color} stroke={color} strokeWidth="2" />
    </svg>
  );
}

/** Линейный график по месяцам на SVG без библиотек. Пропуск месяца — разрыв линии, а не ноль. */
export function LineChart({
  series,
  periods,
  min = 1,
  max = 5,
  height = 200,
  threshold,
  label,
}: {
  series: Series[];
  periods: number;
  min?: number;
  max?: number;
  height?: number;
  threshold?: { value: number; text: string };
  label: string;
}) {
  const W = 560;
  const H = height;
  const pad = { l: 34, r: 14, t: 12, b: 26 };
  const x = (i: number) => pad.l + (periods === 1 ? 0 : (i * (W - pad.l - pad.r)) / (periods - 1));
  const y = (v: number) => pad.t + ((max - v) * (H - pad.t - pad.b)) / (max - min);
  const ticks = Array.from({ length: max - min + 1 }, (_, i) => min + i);
  return (
    <ChartFrame
      label={label}
      periods={periods}
      rows={series.map((s) => ({ name: s.name, values: s.values }))}
      legend={series.map((s) => (
        <span key={s.name}>
          <SeriesKey color={s.color} dashed={s.dashed} /> {s.name}
        </span>
      ))}
    >
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={pad.l} x2={W - pad.r} y1={y(t)} y2={y(t)} className="chart__grid" />
            <text x={pad.l - 8} y={y(t) + 4} className="chart__tick" textAnchor="end">
              {t}
            </text>
          </g>
        ))}
        {Array.from({ length: periods }, (_, i) => (
          <text key={i} x={x(i)} y={H - 6} className="chart__tick" textAnchor="middle">
            M{i + 1}
          </text>
        ))}
        {threshold && (
          <g>
            <line x1={pad.l} x2={W - pad.r} y1={y(threshold.value)} y2={y(threshold.value)} className="chart__threshold" />
            <text x={pad.l + 4} y={y(threshold.value) + 13} className="chart__tick" textAnchor="start">
              {threshold.text}
            </text>
          </g>
        )}
        {series.map((s) => {
          const segs: string[] = [];
          let cur = '';
          s.values.forEach((v, i) => {
            if (v === null) {
              if (cur) segs.push(cur);
              cur = '';
            } else cur += `${cur ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`;
          });
          if (cur) segs.push(cur);
          return (
            <g key={s.name}>
              {segs.map((d, k) => (
                <path key={k} d={d} fill="none" stroke={s.color} strokeWidth={2} strokeLinejoin="round" strokeDasharray={s.dashed ? '6 5' : undefined} />
              ))}
              {s.values.map((v, i) =>
                v === null ? null : (
                  <g key={i}>
                    <circle cx={x(i)} cy={y(v)} r={4} fill={s.dashed ? 'var(--sheet)' : s.color} stroke={s.color} strokeWidth={2} />
                    {/* зона наведения больше самой точки */}
                    <circle cx={x(i)} cy={y(v)} r={11} fill="transparent">
                      <title>{`${s.name}, M${i + 1}: ${fmt(v)}`}</title>
                    </circle>
                  </g>
                ),
              )}
            </g>
          );
        })}
      </svg>
    </ChartFrame>
  );
}

/**
 * «Гантель» по месяцам: точка стажёра (заливка) и наставника (кольцо) на одной шкале, между ними отрезок.
 * Отрезки с |Δ| ≥ gap выделены и подписаны — это и есть расхождение восприятия.
 */
export function DumbbellChart({
  a,
  b,
  periods,
  label,
  min = 1,
  max = 5,
  gap = 1,
  height = 210,
}: {
  a: { name: string; values: Num[] };
  b: { name: string; values: Num[] };
  periods: number;
  label: string;
  min?: number;
  max?: number;
  gap?: number;
  height?: number;
}) {
  const W = 560;
  const H = height;
  const pad = { l: 34, r: 14, t: 22, b: 26 };
  const step = (W - pad.l - pad.r) / periods;
  const x = (i: number) => pad.l + step * (i + 0.5);
  const y = (v: number) => pad.t + ((max - v) * (H - pad.t - pad.b)) / (max - min);
  const ticks = Array.from({ length: max - min + 1 }, (_, i) => min + i);
  const delta = (i: number) => {
    const va = a.values[i];
    const vb = b.values[i];
    return va === null || va === undefined || vb === null || vb === undefined ? null : vb - va;
  };
  const wide = Array.from({ length: periods }, (_, i) => delta(i)).filter((d) => d !== null && Math.abs(d) >= gap - 1e-9).length;
  return (
    <ChartFrame
      label={label}
      periods={periods}
      rows={[
        { name: a.name, values: a.values },
        { name: b.name, values: b.values },
        {
          name: 'Разница (наставник − стажёр)',
          values: Array.from({ length: periods }, (_, i) => {
            const d = delta(i);
            return d === null ? null : `${d > 0 ? '+' : d < 0 ? '−' : ''}${fmt(Math.abs(d), 1)}${Math.abs(d) >= gap - 1e-9 ? ' !' : ''}`;
          }),
        },
      ]}
      legend={
        <>
          <span>
            <SeriesKey color="var(--c-intern)" /> {a.name}
          </span>
          <span>
            <SeriesKey color="var(--c-mentor)" dashed /> {b.name}
          </span>
          <span>
            <i className="gap-key" aria-hidden="true" /> разрыв ≥ {fmt(gap, 0)} балла{wide ? `: ${wide} мес.` : ' — нет'}
          </span>
        </>
      }
    >
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={pad.l} x2={W - pad.r} y1={y(t)} y2={y(t)} className="chart__grid" />
            <text x={pad.l - 8} y={y(t) + 4} className="chart__tick" textAnchor="end">
              {t}
            </text>
          </g>
        ))}
        {Array.from({ length: periods }, (_, i) => {
          const va = a.values[i] ?? null;
          const vb = b.values[i] ?? null;
          const d = delta(i);
          const big = d !== null && Math.abs(d) >= gap - 1e-9;
          const top = Math.min(va === null ? Infinity : y(va), vb === null ? Infinity : y(vb));
          const tip = `M${i + 1}: ${a.name.toLowerCase()} ${fmt(va, 1)}, ${b.name.toLowerCase()} ${fmt(vb, 1)}${d !== null ? `, разница ${fmt(Math.abs(d), 1)}` : ''}`;
          return (
            <g key={i}>
              <text x={x(i)} y={H - 6} className="chart__tick" textAnchor="middle">
                M{i + 1}
              </text>
              {va !== null && vb !== null && (
                <line x1={x(i)} x2={x(i)} y1={y(va)} y2={y(vb)} className={big ? 'db__seg db__seg--gap' : 'db__seg'} />
              )}
              {vb !== null && <circle cx={x(i)} cy={y(vb)} r={5} fill="var(--sheet)" stroke="var(--c-mentor)" strokeWidth={2.5} />}
              {va !== null && <circle cx={x(i)} cy={y(va)} r={5.5} fill="var(--c-intern)" stroke="var(--sheet)" strokeWidth={1.5} />}
              {big && (
                <text x={x(i)} y={top - 11} className="db__label" textAnchor="middle">
                  Δ {fmt(Math.abs(d!), 1)}
                </text>
              )}
              <rect x={x(i) - step / 2} y={pad.t - 14} width={step} height={H - pad.t - pad.b + 14} fill="transparent">
                <title>{tip}</title>
              </rect>
            </g>
          );
        })}
      </svg>
    </ChartFrame>
  );
}

/** Мини-график пары для таблицы. */
export function Sparkline({ a, b, min = 1, max = 5 }: { a: Num[]; b: Num[]; min?: number; max?: number }) {
  const W = 96;
  const H = 28;
  const n = Math.max(a.length, 2);
  const path = (s: Num[]) =>
    s
      .map((v, i) => (v === null ? '' : `${i === 0 || s[i - 1] === null ? 'M' : 'L'}${((i * (W - 4)) / (n - 1) + 2).toFixed(1)},${(2 + ((max - v) * (H - 4)) / (max - min)).toFixed(1)}`))
      .join('');
  return (
    <svg className="spark" viewBox={`0 0 ${W} ${H}`} aria-hidden="true">
      <path d={path(b)} fill="none" stroke="var(--c-mentor)" strokeWidth={1.8} strokeDasharray="3 3" />
      <path d={path(a)} fill="none" stroke="var(--c-intern)" strokeWidth={2} />
    </svg>
  );
}

/** Точка Бланда–Альтмана: средняя оценка двух сторон и их разница (стажёр − наставник). */
export interface BAPoint {
  pair: string;
  period: number;
  mean: number;
  diff: number;
}

/** Точки Бланда–Альтмана из рядов пары: только месяцы, где ответили обе стороны. */
export function baPoints(pairs: { pair: string; intern_index: Num[]; mentor_index: Num[] }[]): BAPoint[] {
  const out: BAPoint[] = [];
  for (const p of pairs)
    p.intern_index.forEach((a, i) => {
      const b = p.mentor_index[i];
      if (a === null || b === null || b === undefined) return;
      out.push({ pair: p.pair, period: i + 1, mean: (a + b) / 2, diff: a - b });
    });
  return out;
}

/**
 * Мини-график Бланда–Альтмана: по X — средняя оценка двух сторон, по Y — разница «стажёр − наставник».
 * Сплошная линия — смещение, пунктир — границы, в которые попадают 95% разниц. Переключатель «Таблица» — для скринридера.
 */
export function BlandAltmanChart({ points, bias, lo, hi }: { points: BAPoint[]; bias: number; lo: number; hi: number }) {
  const [table, setTable] = useState(false);
  const id = useId();
  const W = 420;
  const H = 220;
  const P = { l: 40, r: 70, t: 12, b: 30 };
  const ext = Math.max(2, ...points.map((p) => Math.abs(p.diff)), Math.abs(lo), Math.abs(hi));
  const yMax = Math.ceil(ext * 2) / 2;
  const x = (v: number) => P.l + ((v - 1) * (W - P.l - P.r)) / 4;
  const y = (v: number) => P.t + ((yMax - v) * (H - P.t - P.b)) / (2 * yMax);
  const label = `Согласие сторон: ${points.length} месяцев с оценками обеих сторон. Смещение ${fmt(bias)}, границы ${fmt(lo)} и ${fmt(hi)}.`;
  const ticks = [-yMax, -yMax / 2, 0, yMax / 2, yMax];
  return (
    <figure className="chart" aria-labelledby={id}>
      <div className="chart__bar">
        <figcaption id={id} className="chart__legend">
          <span className="sr-only">{label} </span>
          <span className="legend-item">
            <svg className="series-key" width="26" height="12" viewBox="0 0 26 12" aria-hidden="true">
              <line x1="1" x2="25" y1="6" y2="6" stroke="var(--ink)" strokeWidth="2" />
            </svg>
            смещение
          </span>
          <span className="legend-item">
            <svg className="series-key" width="26" height="12" viewBox="0 0 26 12" aria-hidden="true">
              <line x1="1" x2="25" y1="6" y2="6" stroke="var(--sev-medium)" strokeWidth="2" strokeDasharray="4 3" />
            </svg>
            95%-е границы согласия
          </span>
        </figcaption>
        <button type="button" className="chart__toggle" aria-pressed={table} onClick={() => setTable((t) => !t)}>
          Таблица
        </button>
      </div>
      {table ? (
        <div className="chart-table__wrap">
          <table className="chart-table">
            <caption className="sr-only">{label}</caption>
            <thead>
              <tr>
                <th scope="col">Пара, месяц</th>
                <th scope="col">Средняя двух сторон</th>
                <th scope="col">Стажёр − наставник</th>
              </tr>
            </thead>
            <tbody>
              {points.map((p) => (
                <tr key={`${p.pair}-${p.period}`}>
                  <th scope="row">
                    {p.pair}, M{p.period}
                  </th>
                  <td>{fmt(p.mean)}</td>
                  <td>{fmt(p.diff)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <svg className="chart__svg ba" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label}>
          {ticks.map((t) => (
            <g key={t}>
              <line x1={P.l} x2={W - P.r} y1={y(t)} y2={y(t)} stroke="var(--rule-soft)" />
              <text x={P.l - 6} y={y(t) + 4} textAnchor="end" className="chart__tick">
                {fmt(t, 1)}
              </text>
            </g>
          ))}
          {[1, 2, 3, 4, 5].map((t) => (
            <text key={t} x={x(t)} y={H - 10} textAnchor="middle" className="chart__tick">
              {t}
            </text>
          ))}
          <line x1={P.l} x2={W - P.r} y1={y(bias)} y2={y(bias)} stroke="var(--ink)" strokeWidth="2" />
          <text x={W - P.r + 4} y={y(bias) + 4} className="chart__tick">
            {fmt(bias)}
          </text>
          {[lo, hi].map((v, i) => (
            <g key={i}>
              <line x1={P.l} x2={W - P.r} y1={y(v)} y2={y(v)} stroke="var(--sev-medium)" strokeWidth="1.5" strokeDasharray="5 4" />
              <text x={W - P.r + 4} y={y(v) + 4} className="chart__tick">
                {fmt(v)}
              </text>
            </g>
          ))}
          {points.map((p) => (
            <circle key={`${p.pair}-${p.period}`} cx={x(p.mean)} cy={y(p.diff)} r={4} fill="var(--c-intern)" fillOpacity={0.55} stroke="var(--c-intern)">
              <title>
                {p.pair}, M{p.period}: средняя {fmt(p.mean)}, разница {fmt(p.diff)}
              </title>
            </circle>
          ))}
        </svg>
      )}
    </figure>
  );
}

/** Горизонтальные полосы долей с вертикальной линией порога (например, потолок 15%). Подписи — текстом, цвет не единственный признак. */
export function ShareBars({ rows, threshold, label }: { rows: { name: string; share: number }[]; threshold: number; label: string }) {
  return (
    <ul className="bars" aria-label={label}>
      {rows.map((r) => {
        const over = r.share > threshold;
        return (
          <li key={r.name} className={`bars__row${over ? ' bars__row--over' : ''}`}>
            <span className="bars__name">{r.name}</span>
            <span className="bars__track" aria-hidden="true">
              <span className="bars__fill" style={{ width: `${Math.min(100, r.share * 100)}%` }} />
              <span className="bars__mark" style={{ left: `${threshold * 100}%` }} />
            </span>
            <span className="bars__val">
              {Math.round(r.share * 100)}%{over ? <span className="bars__flag"> выше порога</span> : null}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
