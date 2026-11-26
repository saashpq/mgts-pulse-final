import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api, type PairDetail } from '../api';

function csvCell(value: string) {
  const literal = /^\s*[=+\-@]/.test(value) ? "'" + value : value;
  return '"' + literal.replace(/"/g, '""') + '"';
}

function answers(pair: PairDetail, role: 'intern' | 'mentor', month: number) {
  const rows: string[] = [];
  const questions = pair.questions ?? [];
  for (const q of questions) {
    const value = q.values[month - 1];
    if (q.role === role && value != null) rows.push(q.text + ' — ' + value.toLocaleString('ru-RU'));
  }
  for (const c of pair.comments ?? []) {
    if (c.role !== role || c.period !== month || !c.text.trim()) continue;
    const label = questions.find(q => q.role === role && q.code === c.question)?.text ?? 'Комментарий';
    rows.push(label + ' — ' + c.text);
  }
  return rows.join('\n') || 'Нет ответов за месяц';
}

/** One blank verdict for every observed pair-month, in neutral order and without model output. */
export function blindWorksheet(pairs: PairDetail[]) {
  const rows = [['Пара', 'Месяц', 'Ответы стажёра', 'Ответы наставника', 'Нужен разговор (да / нет / недостаточно данных)', 'Основание решения']];
  for (const p of [...pairs].sort((a,b) => a.report.pair.localeCompare(b.report.pair, 'ru', { numeric: true }))) {
    for (let month = 1; month <= Math.min(p.current, p.periods); month++) {
      rows.push([p.report.pair, String(month), answers(p,'intern',month), answers(p,'mentor',month), '', '']);
    }
  }
  return '\uFEFF' + rows.map(row => row.map(csvCell).join(';')).join('\r\n') + '\r\n';
}

export function BlindReviewDownload({ pairs }: {pairs: string[]}) {
  const client = useQueryClient();
  const [state, setState] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  async function download() {
    setState('loading');
    try {
      const reports: PairDetail[] = [];
      for (let i=0; i<pairs.length; i+=5) {
        const batch = pairs.slice(i,i+5);
        const result = await Promise.all(batch.map(id => client.fetchQuery({
          queryKey:['pair',id], queryFn:() => api.pair(id),
        })));
        reports.push(...result);
      }
      const file = new Blob([blindWorksheet(reports)], {type:'text/csv;charset=utf-8'});
      const url = URL.createObjectURL(file);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'МГТС — анкеты для независимой сверки.csv';
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1500);
      setState('ready');
    } catch {
      setState('error');
    }
  }
  return <div className="quality-download">
    <button type="button" className="quality-action" disabled={state === 'loading'} onClick={download}>
      {state === 'loading' ? 'Готовим бланк…' : state === 'error' ? 'Скачать бланк ещё раз' : 'Скачать бланк для сверки'}
      <svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M8 2v8m-3-3 3 3 3-3M2.5 11v2.5h11V11" /></svg>
    </button>
    <span role="status" className="quality-download__status">
      {state === 'error' ? 'Не удалось собрать все анкеты. Попробуйте ещё раз.' : state === 'ready'
        ? 'Бланк CSV готов. Передайте каждому куратору свою копию.'
        : 'CSV для Excel · исходные ответы всех пар, без сигналов системы'}
    </span>
  </div>;
}
