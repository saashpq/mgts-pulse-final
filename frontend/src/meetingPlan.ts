import type { Brief, BriefSignal, PairDetail, Signal } from './api';
import { plural, reactionDeadline } from './format';
import { pairMonthPath } from './router';

type MeetingSignal = BriefSignal | Signal;
export type MeetingRole = 'intern' | 'mentor';
type Role = MeetingRole;

function actionAuthor(s: MeetingSignal): Role | undefined {
  if ('evidence' in s) return s.evidence.find(e => e.quote)?.role ?? s.evidence[0]?.role;
  return s.side === 'both' ? undefined : s.side;
}

export function actionForAudience(s: MeetingSignal) {
  const author = s.type === 'S9' ? actionAuthor(s) : undefined;
  return author ? s.action.replace('у автора', author === 'intern' ? 'у стажёра' : 'у наставника') : s.action;
}

export function privateMeeting(s: MeetingSignal): { role: Role; question: string } | undefined {
  const author = actionAuthor(s);
  switch (s.action) {
    case 'Встреча HR 1:1 со стажёром: что изменилось и чем помочь':
      return { role: 'intern', question: 'Что изменилось за последние месяцы и чем мы можем помочь?' };
    case 'Предложите стажёру личный разговор, уточните его планы и спросите, какая поддержка ему нужна.':
      return { role: 'intern', question: 'Какие у вас планы по поводу стажировки? Какая поддержка вам сейчас нужна?' };
    case 'Разговор HR с наставником о целях и ожиданиях от стажёра':
    case 'Обсудить с наставником план развития и конкретные ожидания':
      return { role: 'mentor', question: 'Какие цели развития и конкретные результаты стоит согласовать со стажёром?' };
    case 'Уточнить у наставника, замечает ли он изменения у стажёра':
      return { role: 'mentor', question: 'Какие изменения стали заметны в работе стажёра?' };
    case 'Свяжитесь с автором комментария наедине, уточните обстоятельства и обсудите необходимую поддержку.':
    case 'Проведите личную встречу HR 1:1 с автором комментария, уточните обстоятельства и нужную поддержку.':
      return author && { role: author, question: 'Что произошло? Какая поддержка сейчас нужна?' };
    case 'Проведите личный чек-ин с участником и уточните, как идут дела и нужна ли поддержка.':
    case 'Короткий чек-ин: всё ли в порядке':
      return author && { role: author, question: 'Как сейчас идут дела? Нужна ли поддержка?' };
  }
}

export function scheduledSignals<T extends MeetingSignal>(signals: T[]) {
  return signals.map(signal => ({ signal, due: reactionDeadline(signal), meeting: privateMeeting(signal) }))
    .sort((a, b) => a.due.at - b.due.at || a.signal.due_days - b.signal.due_days);
}

export interface MeetingQuote {
  text: string;
  period?: number;
  href?: string;
}

interface ContactDeadline {
  days: number;
  at?: number;
}

function dueFor(signal: BriefSignal): ContactDeadline | undefined {
  if (!Number.isSafeInteger(signal.due_days) || signal.due_days < 0) return;
  const { at } = reactionDeadline(signal);
  return { days: signal.due_days, at: Number.isFinite(new Date(at).getTime()) ? at : undefined };
}

export function contactDate(due?: ContactDeadline, full = false) {
  if (!due) return 'По договорённости';
  if (due.at === undefined) return `${due.days} ${plural(due.days, 'день', 'дня', 'дней')} с появления сигнала`;
  const date = new Date(due.at);
  return 'до ' + date.toLocaleDateString('ru-RU', {
    day: 'numeric', month: 'long', ...((full || date.getFullYear() !== new Date().getFullYear()) && { year: 'numeric' }),
  });
}

function compareContact(a?: ContactDeadline, b?: ContactDeadline) {
  if (a?.at !== undefined && b?.at !== undefined) return a.at - b.at;
  if (a?.at !== undefined) return -1;
  if (b?.at !== undefined) return 1;
  if (a && b) return a.days - b.days;
  return Number(!a) - Number(!b);
}

/** В кратком брифе цитата хранится без месяца. Привязываем её только к её автору и точному исходному фрагменту. */
function quotesFor(signal: BriefSignal, source: Signal | undefined, role: Role, pair?: PairDetail): MeetingQuote[] {
  return [...new Set(signal.quotes ?? [])].filter(text => text.trim()).flatMap((text): MeetingQuote[] => {
    const matches = (source?.evidence ?? []).filter(e => e.quote === text);
    if (!matches.length) return signal.side === role ? [{ text }] : [];
    return matches.filter(e => e.role === role).map(e => {
      const period = Number.isSafeInteger(e.period) && e.period > 0 ? e.period : undefined;
      const comment = pair?.comments?.find(c =>
        c.response_id === e.response_id && c.role === role && c.period === period
        && (!e.question || c.question === e.question) && c.text.includes(text),
      );
      const href = pair && period && comment?.question
        ? `${pairMonthPath(pair.report.pair, period)}#comment-${role}-${encodeURIComponent(comment.question)}`
        : undefined;
      return { text, period, href };
    });
  });
}

/** На старых данных дословный ответ встречается и внутри объяснения общего сигнала. */
function safeForConversation(role: Role, signals: BriefSignal[], pair?: PairDetail) {
  const fragments: string[] = [];
  const sources = pair?.report.signals ?? [];
  for (const signal of signals) {
    if (signal.side === role) continue;
    const evidence = signal.id ? sources.find(s => s.id === signal.id)?.evidence : undefined;
    for (const quote of signal.quotes ?? []) {
      if (!evidence?.some(e => e.role === role && e.quote === quote)) fragments.push(quote);
    }
  }
  for (const source of sources) {
    for (const e of source.evidence ?? []) if (e.role !== role && e.quote) fragments.push(e.quote);
  }
  for (const c of pair?.comments ?? []) {
    if (c.role === role || !c.text) continue;
    fragments.push(c.text, ...(c.text.match(/[^.!?…]+/gu) ?? []));
    if (c.analysis?.quote) fragments.push(c.analysis.quote);
  }
  const words = (text: string) => text.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, ' ').trim();
  const privatePhrases = [...new Set(fragments.map(words).filter(Boolean))].map(s => ' ' + s + ' ');
  return (text: string) => {
    const searchable = ' ' + words(text) + ' ';
    return !privatePhrases.some(phrase => searchable.includes(phrase));
  };
}

/** Исключаем серверные тезисы, которые дословно повторяют действие уже перечисленного сигнала. */
export function briefPoints(b: Pick<Brief, 'signals' | 'talking_points'>) {
  const norm = (x: string) => x.toLowerCase().replace(/\s+/g, ' ').trim();
  const actions = (b.signals ?? []).map(s => norm(s.action || '')).filter(Boolean);
  return (b.talking_points ?? []).filter(t => t.trim() && !actions.some(a => norm(t).includes(a)));
}

const REPEATED_DIFFICULTY = 'Ты несколько раз упоминал одну и ту же трудность. Что изменилось с тех пор, как она появилась?';

function agendaFor(b: Brief, role: Role, signals: BriefSignal[], privateQuestions: string[], exitRisk: boolean) {
  const questions = new Set<string>();
  if (role === 'intern' && exitRisk) {
    questions.add('Как ты сейчас смотришь на продолжение стажировки?');
    questions.add('Какая поддержка тебе сейчас нужна?');
  } else if (role === 'mentor' && signals.some(s => s.type === 'S10' && s.side === role)) {
    questions.add('Как сейчас складывается работа со стажёром? Какой случай ты хотел бы обсудить наедине?');
  } else if (role === 'intern' && signals.some(s => s.type === 'S6')) {
    questions.add('В какой задаче тебе сейчас нужна помощь и как удобнее её получить?');
  } else if (role === 'mentor' && signals.some(s => s.type === 'S3' || s.type === 'S4')) {
    questions.add('Какую задачу ты можешь предложить стажёру на этой неделе и чем сможешь помочь по ходу работы?');
  }
  for (const question of privateQuestions) questions.add(question);
  for (const q of b.questions ?? []) {
    if (q.with !== role || !q.text?.trim()) continue;
    // Старые брифы подбирали этот вопрос по наличию S9 у любой из сторон.
    if (q.text === REPEATED_DIFFICULTY && !signals.some(s => s.type === 'S9' && s.side === role)) continue;
    questions.add(q.text);
  }
  if (!questions.size) questions.add(role === 'intern'
    ? 'Что за последнее время получилось хорошо и чему хотелось бы научиться дальше?'
    : 'Что у стажёра получается хорошо и какой следующий шаг ты бы предложил?');
  return [...questions];
}

function agreementFor(role: Role, signals: BriefSignal[], exitRisk: boolean) {
  if (exitRisk && role === 'intern') return 'Выберите один шаг поддержки. Спросите, что можно передать наставнику, и назначьте день следующего личного разговора.';
  if (role === 'mentor' && signals.some(s => s.type === 'S10')) return 'Выберите конкретный рабочий эпизод и договоритесь, что наставник и куратор могут изменить. Назначьте день проверки.';
  if (role === 'intern' && signals.some(s => s.type === 'S6' || s.type === 'S14')) return 'Договоритесь, к кому стажёр обратится с вопросом по задаче, когда ждать ответ и в какой день проверить, помогло ли это.';
  if (role === 'mentor' && signals.some(s => s.type === 'S3' || s.type === 'S4' || s.type === 'S8')) return 'Выберите одну задачу, согласуйте, как выглядит хороший результат, кто даст обратную связь и когда.';
  return 'Выберите один следующий шаг. Договоритесь, кто его возьмёт и в какой день вы обсудите результат.';
}

/** Аудиторию задаёт бриф. На старых снимках пользуемся только известной стороной, без общей корзины личных цитат. */
export function briefParts(b: Brief, detail?: PairDetail) {
  const pair = detail?.report?.pair === b.pair ? detail : undefined;
  const all = b.signals ?? [];
  const parts = (['intern', 'mentor'] as const).map(role => {
    const audience = b.audiences?.[role];
    const canShow = safeForConversation(role, all, pair);
    const rows = all.flatMap(raw => {
      const source = raw.id ? pair?.report.signals?.find(s => s.id === raw.id) : undefined;
      if (source?.verdict?.status === 'resolved' || source?.verdict?.status === 'false') return [];
      if (!source && raw.id && pair?.report.resolved?.some(s => s.id === raw.id)) return [];
      // В старом ответе side может отсутствовать. Восстанавливаем сторону по первичным свидетельствам.
      const evidence = source?.evidence ?? [];
      const quoted = evidence.filter(e => e.quote && raw.quotes?.includes(e.quote));
      const authors = new Set((quoted.length ? quoted : evidence).map(e => e.role));
      const side = raw.side ?? (authors.size === 1 ? [...authors][0] : authors.size > 1 ? 'both' : undefined);
      if (side && side !== 'both' && side !== role) return [];
      if (audience ? !(audience.signals ?? []).includes(raw.id ?? raw.type) : !side) return [];
      const signal = {
        ...raw, side,
        title: raw.title && canShow(raw.title) ? raw.title : 'Сигнал по анкетам пары',
        explanation: raw.explanation && canShow(raw.explanation) ? raw.explanation : '',
        action: raw.action && canShow(raw.action) ? raw.action : '',
      };
      const quotes = quotesFor(raw, source, role, pair).filter(q =>
        source?.evidence?.some(e => e.role === role && e.quote === q.text) || canShow(q.text),
      );
      return [{ signal, due: dueFor(signal), quotes }];
    });
    const signals = rows.map(row => row.signal);
    const meetings = rows.flatMap(row => {
      const meeting = privateMeeting(row.signal);
      return meeting?.role === role ? [{ ...row, question: meeting.question }] : [];
    }).sort((a, b) => compareContact(a.due, b.due));
    const exitRisk = signals.some(s => s.side === 'intern' && s.type === 'S10' && s.title === 'Риск ухода со стажировки');
    const privateQuestions = meetings.filter(m => !(exitRisk && m.signal.type === 'S10')).map(m => m.question);
    const leadSource = [...meetings, ...rows].find(row => row.quotes.length);
    const leadQuote = leadSource?.quotes[0];
    const ownTrend = role === 'intern' ? 'Индекс стажёра в ' : 'Оценка наставника в ';
    const points = briefPoints({
      signals: all,
      talking_points: audience?.talking_points ?? (b.talking_points ?? []).filter(t => t.startsWith(ownTrend)),
    }).filter(t => !t.startsWith('Сигналов по этой стороне нет') && canShow(t));
    const scorePoint = points.find(text => text.startsWith(ownTrend));
    const scoreMatch = scorePoint?.slice(ownTrend.length).match(/^M(\d+)\s*[—–-]\s*(\d+(?:[.,]\d+)?)/);
    const scoreMonth = scoreMatch ? Number(scoreMatch[1]) : 0;
    const reportedScore = pair?.report[role === 'intern' ? 'intern_index' : 'mentor_index']?.[scoreMonth - 1];
    const scoreSource = pair && scoreMatch && scoreMonth > 0 && scoreMonth <= pair.current
      && reportedScore != null && Math.abs(reportedScore - Number(scoreMatch[2].replace(',', '.'))) < 0.0051
      ? { month: scoreMonth, href: pairMonthPath(b.pair, scoreMonth) + '#index-' + role } : undefined;
    const firstMeeting = meetings[0];
    const questions = agendaFor(b, role, signals, privateQuestions, exitRisk).filter(canShow);
    return {
      role, points, scorePoint, scoreSource, leadQuote, leadSignal: leadSource?.signal,
      rows: rows.map(row => ({
        ...row, sameMeeting: meetings.some(m => m.signal === row.signal && m !== firstMeeting),
      })),
      deadline: firstMeeting?.due,
      questions: questions.length ? questions : ['Что для тебя сейчас важно обсудить?'],
      agreement: agreementFor(role, signals, exitRisk),
      decision: firstMeeting?.signal ?? rows[0]?.signal,
    };
  });
  return parts.sort((a, b) => compareContact(a.deadline, b.deadline));
}

export type BriefPart = ReturnType<typeof briefParts>[number];
