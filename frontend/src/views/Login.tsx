import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { auth } from '../api';

/** Вход по токену HR: сервер запущен с HR_TOKEN и ответил 401. Токен хранится только в этой вкладке (sessionStorage). */
export function Login() {
  const qc = useQueryClient();
  const [value, setValue] = useState('');
  const had = !!auth.token();
  useEffect(() => {
    document.title = 'Вход — Пульс стажировки';
  }, []);
  return (
    <main className="form-page">
      <form
        className="form-card login"
        onSubmit={(e) => {
          e.preventDefault();
          const t = value.trim();
          if (!t) return;
          auth.setToken(t);
          qc.removeQueries();
        }}
      >
        <h1>Пульс стажировки</h1>
        <p className="muted">Дашборд доступен только HR. Введите токен, который выдал администратор стенда.</p>
        {had && (
          <p className="error" role="alert">
            Токен не подошёл. Проверьте его и введите снова.
          </p>
        )}
        <label className="field">
          <span>Токен HR</span>
          <input type="password" autoComplete="current-password" required value={value} onChange={(e) => setValue(e.target.value)} />
        </label>
        <button className="btn btn--primary" disabled={!value.trim()}>
          Войти
        </button>
      </form>
    </main>
  );
}
