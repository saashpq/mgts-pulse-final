import { useId, useMemo, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type ImportMapping, type ImportPreview, type ImportReport, type ImportTarget, type PreviewSheet } from '../api';
import { ROLE_TEXT } from '../format';
import { DATA_KEYS, invalidate } from '../queries';

type Role = 'intern' | 'mentor' | '';
export interface SheetChoice {
  role: Role;
  /** index колонки → target; '' — не выбрано */
  columns: Record<number, string>;
}

const ID_TARGETS = ['intern_id', 'mentor_id'];
const unused = (v: string) => v === '' || v === 'skip';

export interface SheetIssues {
  /** лист загружается: выбрана хотя бы одна колонка */
  active: boolean;
  errors: string[];
  /** колонки с ошибкой: дубликат или вопрос другой роли */
  bad: Set<number>;
}

/** Проверка сопоставления листа: роль, ID участника, месяц, без повторов и чужих вопросов. */
export function sheetIssues(choice: SheetChoice, targets: ImportTarget[]): SheetIssues {
  const byValue = new Map(targets.map((t) => [t.value, t]));
  const chosen = Object.entries(choice.columns).filter(([, v]) => !unused(v));
  const bad = new Set<number>();
  const errors: string[] = [];
  if (chosen.length === 0) return { active: false, errors, bad };
  if (!choice.role) errors.push('Укажите, чьи ответы на листе: стажёров или наставников.');
  const values = chosen.map(([, v]) => v);
  if (!values.some((v) => ID_TARGETS.includes(v))) errors.push('Не выбрана колонка с ID участника («ID стажёра» или «ID наставника»).');
  if (!values.includes('period')) errors.push('Не выбрана колонка «Месяц».');
  const seen = new Map<string, number[]>();
  for (const [i, v] of chosen) seen.set(v, [...(seen.get(v) ?? []), Number(i)]);
  for (const [v, idx] of seen) {
    if (idx.length < 2) continue;
    idx.forEach((i) => bad.add(i));
    errors.push(`«${byValue.get(v)?.label ?? v}» выбрано для нескольких колонок — оставьте одну.`);
  }
  for (const [i, v] of chosen) {
    const t = byValue.get(v);
    if (choice.role && t && t.role !== 'any' && t.role !== choice.role) {
      bad.add(Number(i));
      errors.push(`Вопрос «${t.label}» из анкеты ${t.role === 'intern' ? 'стажёра' : 'наставника'}, а лист — ${choice.role === 'intern' ? 'стажёров' : 'наставников'}.`);
    }
  }
  return { active: true, errors, bad };
}

/** Тело mapping для POST /imports. Лист без выбранных колонок передаётся пустым — он не загружается. */
export function buildMapping(sheets: PreviewSheet[], choices: SheetChoice[], fallback: Role): ImportMapping {
  const out: ImportMapping = {};
  sheets.forEach((sh, k) => {
    const c = choices[k];
    const columns: Record<string, string> = {};
    for (const [i, v] of Object.entries(c.columns)) if (v) columns[i] = v;
    out[sh.name] = { role: c.role || fallback || 'intern', columns };
  });
  return out;
}

const initialChoices = (p: ImportPreview, role: Role): SheetChoice[] =>
  p.sheets.map((sh) => ({
    role: sh.role || role,
    columns: Object.fromEntries(sh.columns.map((c) => [c.index, c.target ?? ''])),
  }));

/** Отчёт импорта: что принято, что не сопоставлено, ошибки по листам. */
export function ImportReportView({ result }: { result: ImportReport }) {
  return (
    <div className="import-report" role="status">
      <p>
        <b>Принято строк: {result.report.accepted}</b>, новых или изменённых ответов: {result.changed}, без изменений: {result.unchanged}, отклонено:{' '}
        {result.report.rejected}.{result.report.new_pairs?.length ? ` Новые пары: ${result.report.new_pairs.join(', ')}.` : ''}
        {result.remembered ? ` Запомнено названий колонок: ${result.remembered}.` : ''}
        {result.report.start_month && ` Первый месяц программы: ${result.report.start_month}.`}
      </p>
      {result.report.sheets.map((sh) => (
        <details key={sh.name} open={!!sh.errors?.length || !!sh.unmapped?.length}>
          <summary>
            Лист «{sh.name}» — {sh.role ? ROLE_TEXT[sh.role as 'intern'] : 'роль не определена'}, принято {sh.accepted}
          </summary>
          {sh.mapped && <p className="muted">Сопоставлено: {sh.mapped.join('; ')}</p>}
          {sh.unmapped && <p className="error">Не сопоставлено: {sh.unmapped.join('; ')} — сопоставьте колонки через «Проверить колонки»</p>}
          {sh.errors?.map((e) => (
            <p key={e} className="error">
              {e}
            </p>
          ))}
        </details>
      ))}
    </div>
  );
}

/** Загрузка выгрузки: быстрый путь и мастер с ручным сопоставлением колонок. */
export function ImportSection() {
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [role, setRole] = useState<Role>('');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [choices, setChoices] = useState<SheetChoice[]>([]);
  const [remember, setRemember] = useState(true);
  const [result, setResult] = useState<ImportReport | null>(null);
  const [exportError, setExportError] = useState('');

  const done = (r: ImportReport) => {
    setResult(r);
    setPreview(null);
    invalidate(qc, DATA_KEYS);
  };
  const check = useMutation({
    mutationFn: (f: File) => api.previewImport(f, role),
    onSuccess: (p) => {
      setResult(null);
      setPreview(p);
      setChoices(initialChoices(p, role));
    },
  });
  const quick = useMutation({ mutationFn: (f: File) => api.importFile(f, role), onSuccess: done });
  const upload = useMutation({
    mutationFn: ({ f, mapping }: { f: File; mapping: ImportMapping }) => api.importFile(f, role, mapping, remember),
    onSuccess: done,
  });

  const issues = useMemo(() => (preview ? choices.map((c) => sheetIssues(c, preview.targets)) : []), [preview, choices]);
  const anyActive = issues.some((i) => i.active);
  const blocked = !anyActive || issues.some((i) => i.errors.length > 0);
  const busy = check.isPending || quick.isPending || upload.isPending;
  const error = (check.error ?? quick.error ?? upload.error) as Error | null;

  const clearSelectionResults = () => {
    setResult(null);
    setPreview(null);
    check.reset();
    upload.reset();
    quick.reset();
  };

  const pick = (k: number, patch: Partial<SheetChoice> | ((c: SheetChoice) => SheetChoice)) =>
    setChoices((cs) => cs.map((c, j) => (j === k ? (typeof patch === 'function' ? patch(c) : { ...c, ...patch }) : c)));

  const download = async () => {
    setExportError('');
    try {
      const blob = await api.surveyExport();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'survey.json';
      document.body.append(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      setExportError((e as Error).message);
    }
  };

  return (
    <section className="sheet" aria-labelledby="import-h">
      <h2 id="import-h" className="sheet__h">
        Загрузить выгрузку анкет
      </h2>
      <p className="muted">
        Excel (.xlsx) или CSV. Если названия колонок отличаются от анкеты, нажмите «Проверить колонки» и укажите, что в какой колонке. Повторная загрузка того же
        файла ничего не дублирует.
      </p>
      <p className="muted">Для новой пары нужны ID стажёра и наставника. Если в одной из выгрузок есть оба ID, загрузите её первой. Если файлы содержат только собственные ID участников, укажите связь в списке пар программы.</p>
      <form
        className="upload"
        onSubmit={(e) => {
          e.preventDefault();
          if (file && !busy) check.mutate(file);
        }}
      >
        <input
          ref={input}
          type="file"
          disabled={busy}
          accept=".xlsx,.csv,text/csv"
          aria-label="Файл выгрузки"
          onChange={(e) => {
            setFile(e.target.files?.[0] ?? null);
            clearSelectionResults();
          }}
        />
        <select value={role} disabled={busy} onChange={(e) => {
          setRole(e.target.value as Role);
          clearSelectionResults();
        }} aria-label="Чьи ответы">
          <option value="">Роль — определить по колонкам</option>
          <option value="intern">Ответы стажёров</option>
          <option value="mentor">Ответы наставников</option>
        </select>
        <button className="btn btn--primary" disabled={busy || !file}>
          {check.isPending ? 'Читаем файл…' : 'Проверить колонки'}
        </button>
        <button type="button" className="btn btn--ghost" disabled={busy || !file} onClick={() => file && quick.mutate(file)}>
          {quick.isPending ? 'Загружаем…' : 'Загрузить без проверки'}
        </button>
      </form>
      {error && (
        <p className="error" role="alert">
          {error.message}
        </p>
      )}

      {preview && file && (
        <div className="mapper" aria-label="Сопоставление колонок" role="group">
          {preview.sheets.length === 0 && <p className="error">В файле нет листов с данными.</p>}
          {preview.sheets.map((sh, k) => (
            <SheetMapper
              key={sh.name}
              sheet={sh}
              targets={preview.targets}
              choice={choices[k]}
              issues={issues[k]}
              onRole={(r) => pick(k, { role: r })}
              onColumn={(i, v) => pick(k, (c) => ({ ...c, columns: { ...c.columns, [i]: v } }))}
            />
          ))}
          <div className="mapper__foot">
            <label className="check">
              <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
              Запомнить названия колонок для следующих загрузок
            </label>
            <div className="mapper__actions">
              {!anyActive && <span className="muted">Выберите хотя бы одну колонку для загрузки.</span>}
              <button type="button" className="btn btn--ghost" onClick={() => setPreview(null)}>
                Отмена
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={blocked || busy}
                onClick={() => upload.mutate({ f: file, mapping: buildMapping(preview.sheets, choices, role) })}
              >
                {upload.isPending ? 'Загружаем…' : 'Загрузить'}
              </button>
            </div>
          </div>
        </div>
      )}

      {result && <ImportReportView result={result} />}

      <p className="upload__survey muted">
        Выученные названия колонок хранятся в анкете сервера. Чтобы перенести их в репозиторий, положите файл в <code>data/survey.json</code>:{' '}
        <button type="button" className="btn btn--link" onClick={download}>
          Скачать survey.json
        </button>
      </p>
      {exportError && (
        <p className="error" role="alert">
          {exportError}
        </p>
      )}
    </section>
  );
}

function SheetMapper({
  sheet,
  targets,
  choice,
  issues,
  onRole,
  onColumn,
}: {
  sheet: PreviewSheet;
  targets: ImportTarget[];
  choice: SheetChoice;
  issues: SheetIssues;
  onRole: (r: Role) => void;
  onColumn: (index: number, value: string) => void;
}) {
  const id = useId();
  const service = targets.filter((t) => t.role === 'any');
  const questions = (r: 'intern' | 'mentor') => targets.filter((t) => t.role === r);
  const unknown = sheet.columns.filter((c) => !choice.columns[c.index]).length;
  return (
    <fieldset className="map-sheet" aria-describedby={issues.errors.length ? `${id}-err` : undefined}>
      <legend className="map-sheet__h">
        Лист «{sheet.name}» <span className="muted">— строк: {sheet.rows}</span>
      </legend>
      <div className="map-sheet__bar">
        <label>
          Чьи ответы{' '}
          <select value={choice.role} onChange={(e) => onRole(e.target.value as Role)} aria-invalid={issues.active && !choice.role}>
            <option value="">— выберите —</option>
            <option value="intern">стажёров</option>
            <option value="mentor">наставников</option>
          </select>
        </label>
        {unknown > 0 ? (
          <span className="map-flag">Не распознано колонок: {unknown}</span>
        ) : (
          <span className="muted">Все колонки сопоставлены</span>
        )}
      </div>
      <div className="map-scroll">
        <table className="map-table">
          <thead>
            <tr>
              <th scope="col">Колонка в файле</th>
              <th scope="col">Примеры значений</th>
              <th scope="col">Куда загрузить</th>
            </tr>
          </thead>
          <tbody>
            {sheet.columns.map((c) => {
              const v = choice.columns[c.index] ?? '';
              const bad = issues.bad.has(c.index);
              const current = targets.find((t) => t.value === v);
              const foreign = current && current.role !== 'any' && choice.role && current.role !== choice.role;
              const group = (r: 'intern' | 'mentor') => (
                <optgroup label={r === 'intern' ? 'Вопросы анкеты стажёра' : 'Вопросы анкеты наставника'}>
                  {questions(r).map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </optgroup>
              );
              return (
                <tr key={c.index} className={!v ? 'map-row--unknown' : bad ? 'map-row--bad' : undefined}>
                  <th scope="row">
                    {c.title || <span className="muted">без названия, № {c.index + 1}</span>}
                    {!v && <span className="map-row__tag">не распознано</span>}
                  </th>
                  <td className="map-samples">
                    {c.samples.length ? c.samples.slice(0, 3).map((s, i) => <span key={i}>{s}</span>) : <span className="muted">пусто</span>}
                  </td>
                  <td>
                    <select
                      value={v}
                      aria-label={`Куда загрузить колонку «${c.title || c.index + 1}»`}
                      aria-invalid={bad || undefined}
                      onChange={(e) => onColumn(c.index, e.target.value)}
                    >
                      <option value="">— не выбрано —</option>
                      <optgroup label="Служебные">
                        {service.map((t) => (
                          <option key={t.value} value={t.value}>
                            {t.label}
                          </option>
                        ))}
                      </optgroup>
                      {(!choice.role || choice.role === 'intern' || (foreign && current?.role === 'intern')) && group('intern')}
                      {(!choice.role || choice.role === 'mentor' || (foreign && current?.role === 'mentor')) && group('mentor')}
                    </select>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {issues.errors.length > 0 && (
        <ul id={`${id}-err`} className="map-issues" aria-live="polite">
          {issues.errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      )}
      {!issues.active && <p className="muted">Лист не загружается: не выбрано ни одной колонки.</p>}
    </fieldset>
  );
}
