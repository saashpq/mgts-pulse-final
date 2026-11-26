import { useState } from 'react';
import type { ThemeTrend } from '../api';
import { navigate, pairPath } from '../router';

/**
 * Последовательные шкалы «светлее — меньше, темнее — больше», по одному тону на шкалу:
 * тревожная (красная) для негативных тем, спокойная зелёная для позитивных.
 * На двух тёмных ступенях число белое, на светлых — тёмное (контраст ≥ 4,5:1).
 */
const RAMP = {
  neg: ['#FEE4E2', '#FDA29B', '#D92D20', '#912018'],
  pos: ['#DCFAE6', '#75E0A7', '#067647', '#053321'],
};
const level = (n: number, max: number) => (n <= 0 ? -1 : Math.min(3, Math.ceil((n / max) * 4) - 1));

const word = (n: number) => {
  const a = n % 100;
  const b = n % 10;
  if (a > 10 && a < 20) return 'комментариев';
  return b === 1 ? 'комментарий' : b >= 2 && b <= 4 ? 'комментария' : 'комментариев';
};

/** Матрица «тема × месяц»: сколько комментариев с темой, по всем парам программы. */
export function ThemeMatrix({ themes, periods, current }: { themes: ThemeTrend[]; periods: number; current: number }) {
  const [table, setTable] = useState(false);
  const [open, setOpen] = useState('');
  const max = Math.max(1, ...themes.flatMap((t) => t.by_period));
  const groups = [
    { key: 'neg', title: 'Тревожные темы', items: themes.filter((t) => t.negative) },
    { key: 'pos', title: 'Позитивные темы', items: themes.filter((t) => !t.negative) },
  ] as const;
  const ms = Array.from({ length: periods }, (_, i) => i + 1);
  return (
    <figure className="chart themes">
      <div className="chart__bar">
        <figcaption className="themes__legend">
          {groups.map((g) => (
            <span key={g.key} className="themes__scale">
              {g.key === 'neg' ? 'тревожные' : 'позитивные'}: меньше
              {RAMP[g.key].map((c) => (
                <span key={c} className="themes__swatch" style={{ background: c }} aria-hidden="true" />
              ))}
              больше
            </span>
          ))}
        </figcaption>
        <button type="button" className="chart__toggle" aria-pressed={table} onClick={() => setTable((t) => !t)}>
          Таблица
        </button>
      </div>

      {table ? (
        <div className="chart-table__wrap">
          <table className="chart-table">
            <caption className="sr-only">Комментарии с темой по месяцам</caption>
            <thead>
              <tr>
                <th scope="col">Тема</th>
                {ms.map((m) => (
                  <th scope="col" key={m}>
                    M{m}
                  </th>
                ))}
                <th scope="col">Всего</th>
                <th scope="col">Пары</th>
              </tr>
            </thead>
            <tbody>
              {themes.map((t) => (
                <tr key={t.code}>
                  <th scope="row">
                    {t.label} <span className="muted">({t.negative ? 'тревожная' : 'позитивная'})</span>
                  </th>
                  {ms.map((m) => (
                    <td key={m}>{t.by_period[m - 1] ?? 0}</td>
                  ))}
                  <td>{t.total}</td>
                  <td>{t.pairs.join(', ')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="themes__wrap">
          <table className="themes__grid" aria-label="Темы комментариев по месяцам">
            <thead>
              <tr>
                <th scope="col" className="themes__corner">
                  Тема
                </th>
                {ms.map((m) => (
                  <th scope="col" key={m} className={m > current ? 'themes__future' : undefined}>
                    M{m}
                  </th>
                ))}
                <th scope="col" className="themes__total">
                  Всего
                </th>
                <th scope="col" className="themes__pairs-h">
                  Пары
                </th>
              </tr>
            </thead>
            {groups.map((g) =>
              g.items.length ? (
                <tbody key={g.key}>
                  <tr className="themes__group">
                    <th scope="rowgroup" colSpan={periods + 3}>
                      {g.title}
                    </th>
                  </tr>
                  {g.items.map((t) => (
                    <tr key={t.code} className={open === t.code ? 'themes__row--on' : undefined}>
                      <th scope="row" className="themes__label">
                        <button
                          type="button"
                          className="themes__theme"
                          aria-pressed={open === t.code}
                          title={`Пары: ${t.pairs.join(', ')}`}
                          onClick={() => setOpen(open === t.code ? '' : t.code)}
                        >
                          {t.label}
                        </button>
                      </th>
                      {ms.map((m) => {
                        const n = t.by_period[m - 1] ?? 0;
                        const l = level(n, max);
                        const text = `${t.label}, M${m}: ${n} ${word(n)}`;
                        return (
                          <td key={m} className="themes__td">
                            <span
                              role="img"
                              aria-label={text}
                              title={`${text}. Пары с темой: ${t.pairs.join(', ')}`}
                              className={`themes__cell${l >= 2 ? ' themes__cell--dark' : ''}${m > current ? ' themes__cell--future' : ''}`}
                              style={l >= 0 ? { background: RAMP[g.key][l] } : undefined}
                            >
                              {n > 0 ? n : ''}
                            </span>
                          </td>
                        );
                      })}
                      <td className="themes__total">{t.total}</td>
                      <td className="themes__pairs">
                        {t.pairs.map((p) => (
                          <button key={p} type="button" className="themes__pair" onClick={() => navigate(pairPath(p))}>
                            {p}
                          </button>
                        ))}
                      </td>
                    </tr>
                  ))}
                </tbody>
              ) : null,
            )}
          </table>
        </div>
      )}
    </figure>
  );
}
