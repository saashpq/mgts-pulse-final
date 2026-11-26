import { useEffect, useRef, useState, useSyncExternalStore, type RefObject } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, auth, eventsUrl, isStaticDemo } from './api';
import { STATUS_TEXT } from './format';
import { IconBook, IconFlag, IconGauge, IconInbox, IconPlan, IconSpark, IconTeam } from './components/icons';
import { invalidate, ALL_KEYS, DATA_KEYS } from './queries';
import { Login } from './views/Login';
import { match, navigate, pairPath, usePath } from './router';
import { BriefView } from './views/Brief';
import { FormPage } from './views/FormPage';
import { Overview } from './views/Overview';
import { PairView } from './views/PairView';
import { Signals } from './views/Signals';
import { Sources } from './views/Sources';
import { Thresholds } from './views/Thresholds';
import { Quality } from './views/Quality';
import './shell-refresh.css';

const NAV_GROUPS = [
  { title: 'Работа с парами', items: [
    { path: '/', view: 'overview', title: 'Обзор пар', icon: IconTeam },
    { path: '/signals', view: 'signals', title: 'Сигналы', icon: IconFlag },
  ] },
  { title: 'Отчёты', items: [
    { path: '/report', view: 'report', title: 'Результаты программы', icon: IconBook },
    { path: '/quality', view: 'quality', title: 'Качество системы', icon: IconGauge },
  ] },
  { title: 'Настройка программы', items: [
    { path: '/sources', view: 'sources', title: 'Источники данных', icon: IconInbox },
    { path: '/thresholds', view: 'thresholds', title: 'Пороги сигналов', icon: IconPlan },
  ] },
] as const;

const TITLES: Record<string, [string, string]> = {
  overview: ['Обзор пар', 'Выберите пару для первого разговора и проверьте ответы за месяц.'],
  report: ['Результаты программы', 'Сводка для руководителя, динамика оценок и темы обратной связи.'],
  month: ['Ответы за месяц', 'Сопоставьте оценки и комментарии стажёра и наставника.'],
  signals: ['Сигналы', 'Проверьте основание, выберите действие и отметьте решение.'],
  sources: ['Источники данных', 'Загрузите ответы из Excel или подключите анкеты стажёров и наставников.'],
  thresholds: ['Пороги сигналов', 'Согласуйте с куратором, какие изменения требуют внимания.'],
  quality: ['Качество системы', ''],
  brief: ['Бриф к встрече 1:1', ''],
  pair: ['Карточка пары', 'Динамика, расхождения, сигналы и комментарии с разбором'],
  'not-found': ['Страница не найдена', ''],
};

/** Задержки переподключения к потоку событий: 1 → 2 → 5 → 10 с (дальше — по 10 с). */
export const RECONNECT_DELAYS = [1000, 2000, 5000, 10000];

/**
 * Живое обновление: сервер шлёт событие при новом ответе, импорте или решении HR — экраны перечитывают данные.
 * Пачка событий схлопывается в одну инвалидацию (300 мс). Если поток закрылся (сервер перезапускается),
 * EventSource пересоздаётся с бэкоффом, после переподключения данные перечитываются. Возвращает false, пока связи нет.
 */
export function useLiveUpdates(token: string) {
  const qc = useQueryClient();
  const [online, setOnline] = useState(true);
  useEffect(() => {
    if (typeof EventSource === 'undefined' || isStaticDemo()) return;
    let es: EventSource | null = null;
    let attempt = 0;
    let lost = false;
    let stopped = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let debounce: ReturnType<typeof setTimeout> | undefined;
    let refreshAll = false;
    const refresh = (includeThresholds: boolean) => {
      refreshAll ||= includeThresholds;
      clearTimeout(debounce);
      debounce = setTimeout(() => {
        invalidate(qc, refreshAll ? ALL_KEYS : DATA_KEYS);
        refreshAll = false;
      }, 300);
    };
    const connect = () => {
      if (stopped) return;
      const cur = new EventSource(eventsUrl());
      es = cur;
      cur.onopen = () => {
        attempt = 0;
        setOnline(true);
        if (lost) {
          lost = false;
          refresh(true);
        }
      };
      cur.onerror = () => {
        lost = true;
        setOnline(false);
        // CONNECTING — браузер переподключится сам; CLOSED — пересоздаём вручную
        if (cur.readyState === 2) {
          cur.close();
          const delay = RECONNECT_DELAYS[Math.min(attempt, RECONNECT_DELAYS.length - 1)];
          attempt++;
          retryTimer = setTimeout(connect, delay);
        }
      };
      cur.addEventListener('data.updated', (event: MessageEvent) => {
        try {
          refresh(JSON.parse(event.data).thresholds === true);
        } catch {
          refresh(true);
        }
      });
      cur.addEventListener('analysis.updated', () => refresh(false));
    };
    connect();
    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      clearTimeout(debounce);
      es?.close();
    };
  }, [qc, token]);
  return online;
}

/** Смена экрана: заголовок вкладки и фокус на h1, чтобы скринридер объявил новый раздел. */
function useScreenTitle(title: string, key: string, ref: RefObject<HTMLHeadingElement>) {
  // фокус — только при смене адреса, не при первой загрузке (и не при двойном запуске эффекта в StrictMode)
  const prev = useRef<string | null>(null);
  useEffect(() => {
    document.title = `${title} — Пульс стажировки`;
    if (prev.current !== null && prev.current !== key) {
      // A cached screen can focus its linked answer or decision before this parent effect.
      const destinationFocused = window.location.hash && document.activeElement?.id
        && ref.current?.closest('main')?.contains(document.activeElement);
      if (!destinationFocused) ref.current?.focus({ preventScroll: true });
    }
    prev.current = key;
  }, [title, key, ref]);
}

/** Токен HR и признак «сервер требует токен» (после 401). */
export function useAuth() {
  const required = useSyncExternalStore(auth.subscribe, auth.required);
  const token = useSyncExternalStore(auth.subscribe, auth.token);
  return { required, token };
}

export function App() {
  const route = match(usePath());
  const { required } = useAuth();
  if (route.view === 'form') return <FormPage token={route.token} />;
  if (required) return <Login />;
  return <Shell route={route} />;
}

function Shell({ route }: { route: ReturnType<typeof match> }) {
  const qc = useQueryClient();
  const { token } = useAuth();
  const online = useLiveUpdates(token);
  const demo = isStaticDemo();
  const ov = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const section = route.view === 'pair' && route.month ? 'month' : route.view;
  const [title, subtitle] = TITLES[section] ?? TITLES['not-found'];
  const pairId = route.view === 'pair' || route.view === 'brief' ? route.id : '';
  const h1 = useRef<HTMLHeadingElement>(null);
  useScreenTitle(pairId ? `${title} ${pairId}` : title, window.location.pathname, h1);
  const attention = ov.data?.report.attention ?? 0;
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  const origin = ov.data?.data_origin;
  const sourceLabel = origin && { synthetic: 'Учебные данные', mixed: 'Учебные и загруженные ответы', submitted: 'Загруженные ответы', empty: 'Ответов пока нет' }[origin];
  return (
    <div className="app pulse-shell">
      <a className="skip-to-workspace no-print" href="#workspace-main">Перейти к содержанию</a>
      <div className="shell">
        <aside className={`sidebar${menuOpen ? ' sidebar--expanded' : ''}`} onKeyDown={(event) => {
          if (event.key === 'Escape' && menuOpen) {
            setMenuOpen(false);
            menuButton.current?.focus();
          }
        }}>
          <div className="brand">
            <span className="brand__mark" aria-hidden="true">МГТС</span>
            <span className="brand__text">
              <span className="brand__name">Пульс стажировки</span>
              <span className="brand__tagline">Кабинет HR · МГТС</span>
            </span>
          </div>
          <button ref={menuButton} type="button" className="mobile-sections" aria-expanded={menuOpen}
            aria-controls="main-sections" onClick={() => setMenuOpen(!menuOpen)}>
            Разделы <span aria-hidden="true">{menuOpen ? '▴' : '▾'}</span>
          </button>
          <nav id="main-sections" className="nav" aria-label="Разделы">
            {NAV_GROUPS.map(group => <div className="shell-nav-group" key={group.title}>
            <p className="shell-nav-label">{group.title}</p>
            <ul className="nav__list" aria-label={group.title}>
              {group.items.map((n) => (
                <li key={n.path}>
                  <button
                    type="button"
                    className={`nav__item${section === n.view ? ' nav__item--active' : ''}`}
                    aria-current={section === n.view ? 'page' : undefined}
                    onClick={() => { setMenuOpen(false); navigate(n.path); }}
                  >
                    <n.icon className="nav__icon" />
                    <span>{n.title}</span>
                    {n.view === 'overview' && attention > 0 && (
                      <span className="nav__badge" aria-label={`требуют вмешательства: ${attention}`}>
                        {attention}
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ul></div>)}
          </nav>
          {!!ov.data?.report.pairs.length && (
            <div className="shell-pair-group">
            <p className="shell-nav-label">Пары программы</p>
            <ul className="nav__list side-pairs" aria-label="Пары">
              {ov.data.report.pairs.map((p) => (
                <li key={p.pair}>
                  <button
                    type="button"
                    className={`nav__item${(route.view === 'pair' || route.view === 'brief') && route.id === p.pair ? ' nav__item--active' : ''}`}
                    aria-current={pairId === p.pair ? 'page' : undefined}
                    onClick={() => { setMenuOpen(false); navigate(pairPath(p.pair)); }}
                  >
                    <span className={`dot dot--${p.status}`} aria-hidden="true" />
                    <span>
                      {p.pair} · {p.intern}
                      <br /><small>{STATUS_TEXT[p.status]}</small>
                    </span>
                  </button>
                </li>
              ))}
            </ul></div>
          )}
          {token && (
            <button
              type="button"
              className="btn btn--ghost btn--small side-logout"
              onClick={() => {
                auth.setToken('');
                qc.removeQueries();
                invalidate(qc, ALL_KEYS);
              }}
            >
              Выйти
            </button>
          )}
          <div className="side-note">
            <IconSpark className="nav__icon" />
            <p>
              {ov.isPending ? 'Загрузка режима анализа…' : ov.isError ? 'Режим анализа недоступен' : ov.data?.analyzer.mode === 'llm' ? `Текст разбирает модель ${ov.data.analyzer.model}` : 'Текст разбирает словарь тем (без модели)'}. Решение принимает HR.
            </p>
          </div>
        </aside>
        <main className="workspace" id="workspace-main" tabIndex={-1}>
          <header className="workspace__head">
            <div>
              <h1 className="workspace__title" ref={h1} tabIndex={-1}>
                {title}
              </h1>
              {subtitle && <p className="workspace__subtitle">{subtitle}</p>}
            </div>
            <span className="workspace__flags">
            {!online && (
              <span className="offline" role="status">
                Связь с сервером потеряна, переподключаемся…
              </span>
            )}
            {demo ? (
              <span className="synthetic synthetic--demo" role="note">
                Онлайн-демо: только просмотр, данные синтетические
              </span>
            ) : sourceLabel && <span className="synthetic">{sourceLabel}</span>}
            </span>
          </header>
          <div className="workspace__body">
            {(section === 'overview' || section === 'report') && <Overview report={section === 'report'} />}
            {route.view === 'signals' && <Signals />}
            {route.view === 'sources' && <Sources />}
            {route.view === 'thresholds' && <Thresholds />}
            {route.view === 'quality' && <Quality />}
            {route.view === 'pair' && <PairView id={route.id} month={route.month} />}
            {route.view === 'brief' && <BriefView id={route.id} />}
            {route.view === 'not-found' && (
              <section className="sheet missing-page">
                <h2>Куда перейти?</h2>
                <p>Проверьте адрес ссылки или выберите рабочий раздел.</p>
                <div className="flow-links"><a href="/">Обзор пар</a><a href="/signals">Активные сигналы</a></div>
              </section>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}
