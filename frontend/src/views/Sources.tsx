import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isStaticDemo, type Overview } from '../api';
import { ROLE_TEXT, TONE_TEXT, toneOf } from '../format';
import { CopyButton, Failure, Loading } from './common';
import { ALL_KEYS, DATA_KEYS, invalidate } from '../queries';
import { ImportSection } from './ImportWizard';
import { FormExampleLinks } from './FormPage';
import { absoluteUrl, QrDialog } from '../components/QrDialog';
import './sources.css';

/** Источники данных: загрузка выгрузок, персональные ссылки на анкету, вебхук Яндекс Форм, журнал. */
export function Sources() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['sources'], queryFn: api.sources });
  const [qr, setQr] = useState<{ url: string; title: string } | null>(null);
  const [search, setSearch] = useState('');
  const reset = useMutation({ mutationFn: api.reset, onSuccess: () => invalidate(qc, ALL_KEYS) });
  if (q.isPending) return <Loading />;
  if (q.isError) return <Failure error={q.error} onRetry={() => q.refetch()} />;
  const s = q.data;
  const readOnly = isStaticDemo();
  const localForms = !readOnly && ['127.0.0.1','localhost'].includes(window.location.hostname)
    && ['8260','8270'].includes(window.location.port)
    ? `http://${window.location.hostname}:${Number(window.location.port) + 1}` : '';
  const links = s.links ?? [];
  const query = search.trim().toLocaleLowerCase('ru');
  const matches = links.filter((l) =>
    [l.pair, l.person, ROLE_TEXT[l.role as 'intern']].join(' ').toLocaleLowerCase('ru').includes(query)
  );
  return (
    <div className="sources-page">
      {localForms && <section className="sheet" aria-labelledby="survey-service-title">
        <h2 id="survey-service-title" className="sheet__h">Заполнить анкету во внешнем сервисе</h2>
        <p>Выберите роль. После отправки ответ автоматически появится в карточке стажёра.</p>
        <nav className="quality-jumps" aria-label="Анкеты SurveyJS">
          <a href={localForms + '/intern'} target="_blank" rel="noreferrer">Анкета стажёра ↗</a>
          <a href={localForms + '/mentor'} target="_blank" rel="noreferrer">Анкета наставника ↗</a>
        </nav>
      </section>}
      <nav className="quality-jumps" aria-label="Сбор ответов">
        <a href="#import-h">{readOnly ? 'Импорт файлов' : 'Загрузить файл'}</a>
        <a href="#sources-people">Найти анкету</a>
        <a href="#sources-log">Проверить поступления</a>
        <a href="#sources-connect">Интеграции</a>
      </nav>
      {readOnly ? (
        <section className="sheet sources-preview">
          <h2 id="import-h" className="sheet__h">Импорт Excel и CSV</h2>
          <p>В онлайн-демо показаны готовые данные. Чтобы загрузить свой файл, распакуйте конкурсный комплект Windows и запустите start.bat. В открывшемся кабинете выберите файл и нажмите «Проверить колонки». Если комплекта ещё нет, запросите его у организатора показа.</p>
        </section>
      ) : <ImportSection />}

      <section className="sheet">
        <h2 id="sources-people" className="sheet__h">{readOnly ? 'Анкеты участников' : 'Персональные ссылки на анкету'}</h2>
        {s.form_window && s.periods && !readOnly && (
          <FormWindowControl key={`${s.form_window.period}-${s.form_window.closed}`} current={s.form_window} periods={s.periods} />
        )}
        <p className="muted">{readOnly
          ? 'Посмотрите вопросы для каждой роли. В локальной версии у каждого участника есть персональная ссылка и QR-код для ответа.'
          : 'У каждого участника своя анкета. Отправьте ему ссылку или покажите QR-код. Если участник заполнит анкету ещё раз за тот же месяц, в обзоре останется последний ответ.'}</p>
        {readOnly && <FormExampleLinks />}
        {links.length > 0 && <>
        <div className="sources-search">
          <label>Поиск участника
            <input type="search" value={search} placeholder="ID, пара или роль" onChange={(e) => setSearch(e.target.value)} />
          </label>
          <span className="muted" role="status">{query ? `Найдено: ${matches.length} из ${links.length}` : `Всего анкет: ${links.length}`}</span>
        </div>
        {matches.length > 0 ? <div className="sources-directory" role="region" aria-label="Список участников" tabIndex={0}><table className="links" aria-label={readOnly ? 'Участники программы' : 'Персональные ссылки на анкеты'}>
          <thead><tr><th scope="col">Пара</th><th scope="col">Участник</th>{!readOnly && <th scope="col">Анкета</th>}</tr></thead>
          <tbody>
            {matches.map((l) => (
              <tr key={l.token}>
                <td>{l.pair}</td>
                <td>
                  <strong>{l.person}</strong><span className="sources-role">{ROLE_TEXT[l.role as 'intern']}</span>
                </td>
                {!readOnly && <td>
                  <div className="sources-link-actions">
                  <a href={`/form/${l.token}`} target="_blank" rel="noreferrer">
                    Открыть
                  </a>
                  <CopyButton text={absoluteUrl(l.url)} label="Скопировать ссылку" className="btn btn--ghost btn--small" />
                  <button
                    type="button"
                    className="btn btn--ghost btn--small"
                    aria-label={`QR-код анкеты ${l.person}`}
                    onClick={() => setQr({ url: l.url, title: `Анкета: ${ROLE_TEXT[l.role as 'intern'].toLowerCase()} ${l.person}, пара ${l.pair}` })}
                  >
                    QR-код
                  </button>
                  </div>
                </td>}
              </tr>
            ))}
          </tbody>
        </table></div> : <p className="muted">Участник не найден. Проверьте ID или название пары.</p>}
        </>}
        {!s.links?.length && <p className="muted">Ссылки появятся после добавления первой пары.</p>}
      </section>

      <section className="sheet">
        <h2 id="sources-log" className="sheet__h">Журнал поступлений</h2>
        <p className="muted">Ответов по источникам: {Object.entries(s.counts).map(([k, v]) => `${k === 'synthetic' ? 'демонстрационные данные' : k} — ${v}`).join(', ') || 'нет'}</p>
        <ul className="journal">
          {(s.events ?? []).map((e, i) => (
            <li key={i}>
              <span className="muted">{new Date(e.at).toLocaleString('ru-RU')}</span> · {e.text}
            </li>
          ))}
        </ul>
        {!s.events?.length && <p className="muted">Поступлений пока нет.</p>}
      </section>

      <div className="grid2">
        <section className="sheet">
          <h2 id="sources-connect" className="sheet__h">Яндекс Формы</h2>
          <p>Ответы из анкет в Яндекс 360 можно получать каждые пять минут или при каждой отправке формы.</p>
          <details className="sources-instructions"><summary>Инструкция по подключению</summary>
          <p>Для стажёра и наставника предусмотрены отдельные анкеты. В версии для Windows команда <code>python yandex_forms.py create</code> создаёт их в Яндекс 360 для бизнеса. Для получения ответов каждые пять минут запустите <code>python yandex_forms.py sync --interval 300</code>. Настройка подключения описана в <code>docs/YANDEX-FORMS.md</code>.</p>
          <p className="muted">
            Та же анкета в Яндекс Формах: в настройках формы — интеграция «HTTP-запрос» на адрес <code>{s.webhook.path}</code>, заголовок{' '}
            <code>X-Webhook-Secret</code>, тело:
          </p>
          <pre className="code">{JSON.stringify({ token: '<токен из ссылки>', period: s.form_window?.period ?? 1,
            answers: { i_overall: '{{общая оценка}}', i_clarity: '{{ясность задач}}', i_interest: '{{интерес}}', i_support: '{{поддержка}}', i_comment: '{{комментарий}}' },
          }, null, 2)}</pre>
          <p className="muted">Пример для типовой анкеты стажёра. Передайте все вопросы со шкалой вашей анкеты.</p>
          </details>
          {!readOnly && <p className={s.webhook.secret_set ? 'muted' : 'error'}>
            {s.webhook.secret_set ? 'Секрет задан, вебхук принимает ответы.' : 'WEBHOOK_SECRET не задан — вебхук выключен.'}
          </p>}
        </section>
        <section className="sheet">
          <h2 className="sheet__h">Анализ текста</h2>
          <p>
            <b>{s.analyzer.label}</b>
            {s.analyzer.model ? ` — ${s.analyzer.model}` : ''}
          </p>
          <p className="muted">
            Модель подключается через OpenAI-совместимый интерфейс (vLLM или Ollama в контуре компании). Её ответ принимается, только если темы из закрытого списка, а
            цитата дословно есть в комментарии; иначе — словарь.
          </p>
          {!readOnly && <LlmCheckBlock />}
          {!readOnly && s.demo && (
            <button type="button" className="btn btn--ghost btn--small" disabled={reset.isPending} onClick={() => {
                if (window.confirm('Сбросить демо? Загруженные ответы и решения HR заменятся синтетическими данными.')) reset.mutate();
              }}>
              Сбросить демо к синтетическим данным
            </button>
          )}
          {reset.isError && <p className="error" role="alert">{(reset.error as Error).message}</p>}
        </section>
      </div>

      <div className="flow-links"><a href="/">Перейти к обзору пар →</a></div>
      {qr && <QrDialog url={qr.url} title={qr.title} onClose={() => setQr(null)} />}

    </div>
  );
}

function FormWindowControl({ current, periods }: { current: { period: number; closed: boolean }; periods: number }) {
  const qc = useQueryClient();
  const [month, setMonth] = useState(current.period);
  const update = useMutation({
    mutationFn: ({ period, closed }: { period: number; closed: boolean }) => api.setFormWindow(period, closed),
    onSuccess: () => invalidate(qc, DATA_KEYS),
  });
  return (
    <div>
      <p role="status"><b>Месяц {current.period}.</b> {current.closed ? 'Приём ответов закрыт.' : 'Приём ответов открыт.'}</p>
      <label>Месяц анкеты{' '}
        <select value={month} disabled={update.isPending} onChange={(e) => setMonth(Number(e.target.value))}>
          {Array.from({ length: periods }, (_, i) => <option key={i + 1} value={i + 1}>{i + 1}</option>)}
        </select>
      </label>{' '}
      <button type="button" className="btn btn--ghost btn--small"
        disabled={update.isPending || (month === current.period && !current.closed)}
        onClick={() => update.mutate({ period: month, closed: false })}>Открыть месяц {month}</button>{' '}
      <button type="button" className="btn btn--ghost btn--small" disabled={current.closed || update.isPending}
        onClick={() => update.mutate({ period: current.period, closed: true })}>Закрыть приём за месяц {current.period}</button>
      {update.isError && <p className="error" role="alert">{update.error.message}</p>}
    </div>
  );
}

/** «Проверить модель»: тестовый комментарий через тот же путь, что и боевые (маскирование, проверка цитаты). */
function LlmCheckBlock() {
  const qc = useQueryClient();
  const check = useMutation({ mutationFn: api.llmCheck });
  const r = check.data;
  // подписи тем берём из уже загруженного обзора, если он есть
  const labels = new Map((qc.getQueryData<Overview>(['overview'])?.themes_over_time ?? []).map((t) => [t.code, t.label]));
  return (
    <div className="llm-check">
      <button type="button" className="btn btn--ghost btn--small" disabled={check.isPending} onClick={() => check.mutate()}>
        {check.isPending ? 'Проверяем модель…' : 'Проверить модель'}
      </button>
      {check.isError && (
        <p className="error" role="alert">
          {(check.error as Error).message}
        </p>
      )}
      {r && (
        <div className={`llm-check__res ${r.ok ? 'llm-check__res--ok' : 'llm-check__res--fail'}`} role="status">
          <p className="llm-check__verdict">
            <b>{r.ok ? 'Модель распознала темы контрольного примера' : r.mode === 'offline' ? 'Модель не подключена — работает словарь' : r.analysis?.analyzer.startsWith('llm:') ? 'Модель ответила с ошибкой в разборе примера' : 'Использован резервный словарь'}</b>
          </p>
          <dl className="llm-check__facts">
            <dt>Режим</dt>
            <dd>{r.mode === 'llm' ? 'модель' : 'словарь (offline)'}</dd>
            <dt>Модель</dt>
            <dd>{r.model || '—'}</dd>
            <dt>Задержка</dt>
            <dd>{r.latency_ms === null ? '—' : `${r.latency_ms.toLocaleString('ru-RU')} мс`}</dd>
          </dl>
          {r.sample && <p className="llm-check__sample">«{r.sample}»</p>}
          {r.analysis && (
            <dl className="llm-check__facts">
              <dt>Темы в итоговом разборе</dt>
              <dd>{r.analysis.themes.length ? r.analysis.themes.map((t) => labels.get(t) ?? t).join(', ') : 'нет'}</dd>
              <dt>Тональность</dt>
              <dd>{TONE_TEXT[toneOf(r.analysis.sentiment)]}</dd>
              <dt>Цитата</dt>
              <dd>{r.analysis.quote ? `«${r.analysis.quote}»` : '—'}</dd>
              <dt>Разбор</dt>
              <dd>{r.analysis.analyzer}</dd>
            </dl>
          )}
          {r.error && <p className="error">{r.error}</p>}
          {r.mode === 'offline' && (
            <p className="muted">
              Как подключить: задайте серверу переменные <code>LLM_BASE_URL</code>, <code>LLM_MODEL</code>, <code>LLM_API_KEY</code> (OpenAI-совместимый адрес vLLM или
              Ollama в контуре) и перезапустите его. Пошагово — в файле <code>docs/LLM.md</code> репозитория.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
