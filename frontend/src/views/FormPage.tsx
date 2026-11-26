import { useEffect, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api, ApiError, isStaticDemo, type FormInfo } from '../api';

const audienceNotice = 'Ответы и комментарии доступны HR программы стажировок. HR видит вашу роль и пару и может определить, кто ответил.';

// Public examples of the standard questionnaire in data/survey.json.
const examples = {
  intern: {
    token: 'example',
    title: 'Анкета стажёра',
    scales: [
      'Как в целом проходит стажировка?',
      'Насколько вам интересны задачи?',
      'Насколько понятны поставленные задачи?',
      'Хватает ли вам поддержки наставника?',
    ],
    comment: 'Что получилось, что мешает, что хотели бы изменить?',
  },
  mentor: {
    token: 'example-mentor',
    title: 'Анкета наставника',
    scales: [
      'Как стажёр справляется с работой?',
      'Насколько оперативно стажёр выполняет задачи?',
      'Насколько стажёр самостоятелен?',
      'Насколько стажёр понимает поставленные задачи?',
    ],
    comment: 'Комментарий наставника о работе стажёра',
  },
};

export function FormExampleLinks() {
  return <nav className="quality-jumps" aria-label="Примеры анкет по ролям">
    {Object.values(examples).map(example => (
      <a key={example.token} href={`/form/${example.token}`}>{example.title}</a>
    ))}
  </nav>;
}

/** Публичная анкета по персональной ссылке: без меню и без доступа к аналитике. */
export function FormPage({ token }: { token: string }) {
  if (isStaticDemo()) {
    const example = Object.values(examples).find(item => item.token === token);
    return example ? <StaticFormExample example={example} /> : <StaticFormStub />;
  }
  return <LiveForm key={token} token={token} />;
}

function StaticFormExample({ example }: { example: typeof examples.intern }) {
  useEffect(() => {
    document.title = `${example.title} — пример анкеты МГТС`;
  }, [example.title]);
  const other = example === examples.intern ? examples.mentor : examples.intern;
  return (
    <main className="form-page">
      <article className="form-card">
        <p className="muted">Ежемесячный опрос МГТС · Пример для просмотра</p>
        <h1>{example.title}</h1>
        <p>{audienceNotice}</p>
        <p>В персональной анкете участник ставит четыре оценки за месяц и по желанию оставляет комментарий. Здесь можно только прочитать вопросы.</p>
        <h2>Обязательные оценки</h2>
        <p className="muted">Шкала от 1 до 5. Чем выше оценка, тем лучше участник оценивает ситуацию.</p>
        <ol style={{ display: 'grid', gap: '0.75rem', paddingInlineStart: '1.5rem' }}>
          {example.scales.map(text => <li key={text}>{text}</li>)}
        </ol>
        <h2>Комментарий по желанию</h2>
        <p>{example.comment}</p>
        <nav className="flow-links" aria-label="Другие страницы">
          <a href={`/form/${other.token}`}>{other.title}</a>
          <a href="/sources#sources-people">К списку анкет</a>
        </nav>
      </article>
    </main>
  );
}

/** В онлайн-демо нет сервера, который принял бы ответ: объясняем, как посмотреть анкету вживую. */
function StaticFormStub() {
  useEffect(() => {
    document.title = 'Анкета — онлайн-демо';
  }, []);
  return (
    <main className="form-page">
      <section className="form-card">
        <h1>Анкета в онлайн-демо не принимает ответы</h1>
        <p>
          Это статическая копия дашборда: данные сняты с сервера и только показываются. Персональная анкета работает в полной версии — ответ сразу попадает в
          дашборд, и экран HR обновляется сам.
        </p>
        <p className="muted">Если вы участник стажировки, запросите у куратора персональную ссылку. В демо можно посмотреть вопросы для обеих ролей.</p>
        <nav className="flow-links" aria-label="Примеры анкет"><a href="/form/example">Анкета стажёра</a><a href="/form/example-mentor">Анкета наставника</a></nav>
        <a className="btn btn--ghost" href="/">
          Открыть дашборд
        </a>
      </section>
    </main>
  );
}

function LiveForm({ token }: { token: string }) {
  const q = useQuery({ queryKey: ['form', token], queryFn: () => api.form(token), refetchOnWindowFocus: false });
  if (q.isPending) return <main className="form-page"><p>Загружаем анкету…</p></main>;
  if (q.isError) return <main className="form-page"><p className="error" role="alert">{(q.error as Error).message}</p></main>;
  if (q.data.closed)
    return <main className="form-page"><section className="form-card"><h1>Приём ответов закрыт</h1>
      <p>HR закрыл анкету за месяц {q.data.period}. Когда приём откроется, используйте эту же ссылку.</p>
    </section></main>;
  return <OpenForm key={q.data.period} token={token} f={q.data} reload={() => void q.refetch()} />;
}

function OpenForm({ token, f, reload }: { token: string; f: FormInfo; reload: () => void }) {
  const [answers, setAnswers] = useState<Record<string, string | number>>({});
  const send = useMutation({ mutationFn: () => api.submitForm(token, f.period, answers) });
  const docTitle = send.isSuccess ? 'Ответ отправлен' : f.title;
  useEffect(() => {
    document.title = `${docTitle} — анкета стажировки`;
  }, [docTitle]);
  if (send.isSuccess)
    return (
      <main className="form-page">
        <section className="form-card">
          <h1>Спасибо!</h1>
          <p>Ответ за месяц {send.data.period} сохранён. HR увидит его сразу.</p>
        </section>
      </main>
    );
  const scales = f.questions.filter((x) => x.kind === 'scale');
  const comments = f.questions.filter(x => x.kind !== 'scale');
  const completed = scales.filter((x) => answers[x.code] !== undefined).length;
  const ready = completed === scales.length;
  return (
    <main className="form-page">
      <form
        className="form-card"
        onSubmit={(e) => {
          e.preventDefault();
          if (ready && !send.isPending) send.mutate();
        }}
      >
        <p className="muted">{f.role === 'intern' ? 'Анкета стажёра' : 'Анкета наставника'} · месяц {f.period} из {f.periods}</p>
        <h1>{f.title}</h1>
        <p>{audienceNotice}</p>
        {scales.length > 0 && <div className="form-progress">
          <label htmlFor="scale-progress">Обязательные оценки · {completed} из {scales.length}</label>
          <progress id="scale-progress" value={completed} max={scales.length} />
        </div>}
        {scales.length > 0 && <h2>Оценки за месяц</h2>}
        {scales.map((x) => (
            <fieldset key={x.code} className="scale">
              <legend>{x.text}</legend>
              <div className="scale__opts">
                {Array.from({ length: x.max ?? 5 }, (_, i) => i + 1).map((v) => (
                  <label key={v} className={`scale__opt${answers[x.code] === v ? ' scale__opt--on' : ''}`}>
                    <input type="radio" name={x.code} value={v} checked={answers[x.code] === v} onChange={() => setAnswers({ ...answers, [x.code]: v })} />
                    {v}
                  </label>
                ))}
              </div>
              <p className="scale__hint muted">
                <span>1 — совсем нет</span>
                <span>{x.max ?? 5} — полностью</span>
              </p>
            </fieldset>
          ))}
        {comments.length > 0 && <h2>Комментарии</h2>}
        {comments.map((x) => (
            <label key={x.code} className="field">
              <span>{x.text} <small className="muted">(необязательно)</small></span>
              <textarea rows={4} maxLength={2000} value={(answers[x.code] as string) ?? ''} onChange={(e) => setAnswers({ ...answers, [x.code]: e.target.value })} />
            </label>
          ))}
        <button className="btn btn--primary" disabled={!ready || send.isPending}>
          {send.isPending ? 'Отправляем…' : 'Отправить'}
        </button>
        {!ready && <p className="muted">Ответьте на все вопросы с оценкой.</p>}
        {send.isError && <p className="error" role="alert">{(send.error as Error).message}</p>}
        {send.error instanceof ApiError && send.error.status === 409 && (
          <button type="button" className="btn btn--ghost" onClick={reload}>Проверить текущий месяц</button>
        )}
      </form>
    </main>
  );
}
