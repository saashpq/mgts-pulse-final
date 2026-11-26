// Снимок ответов API для статического онлайн-демо (VITE_STATIC_DEMO=true).
// Запуск: node scripts/snapshot.mjs  (API_URL — адрес бэкенда, по умолчанию http://127.0.0.1:8080; HR_TOKEN — если сервер его требует).
// Пишет frontend/public/demo/*.json. Перед снимком стоит сбросить демо: curl -X POST $API_URL/api/v1/demo/reset
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const API = (process.env.API_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const OUT = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'demo');
const headers = process.env.HR_TOKEN ? { Authorization: `Bearer ${process.env.HR_TOKEN}` } : {};

async function get(path) {
  const res = await fetch(API + path, { headers });
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.json();
}

async function save(file, data) {
  await writeFile(join(OUT, file), JSON.stringify(data, null, 1) + '\n');
  console.log('  ' + file);
}

await mkdir(OUT, { recursive: true });
console.log(`Снимок ${API} → ${OUT}`);
const overview = await get('/api/v1/overview');
await save('overview.json', overview);
await save('signals-grouped.json', await get('/api/v1/signals?view=grouped'));
await save('signals-all.json', await get('/api/v1/signals?view=all'));
await save('thresholds.json', await get('/api/v1/thresholds'));
const sources = await get('/api/v1/sources');
// Статические анкеты открывают демонстрационный экран. Для них достаточно локального номера.
sources.links = (sources.links ?? []).map(({ pair, person, role }, index) => {
  const token = `demo-${index + 1}`;
  return { pair, person, role, token, url: `/form/${token}` };
});
await save('sources.json', sources);
for (const p of overview.report.pairs) {
  const id = encodeURIComponent(p.pair);
  await save(`pair-${p.pair}.json`, await get(`/api/v1/pairs/${id}`));
  await save(`brief-${p.pair}.json`, await get(`/api/v1/pairs/${id}/brief`));
}
console.log('Готово');
