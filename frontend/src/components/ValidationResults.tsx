import results from './validation-results.json';
import examples from './validation-examples.json';

const models: Record<string, string> = {
  'run-offline-final': 'Словарь без модели',
  'run-3b': 'Qwen 3B, исходная версия',
  'run-final': 'Qwen 7B со словарём',
};
const themes = { attrition: 'Риск ухода', conflict: 'Конфликт' } as const;

function ResultTable({ group, title }: { group: keyof typeof results; title: string }) {
  const runs: Record<string, { production: typeof results.unseen32['run-final']['production'] }> = results[group];
  return (
    <div className="table-scroll quality-comparison-scroll" role="region" aria-label={title} tabIndex={0}>
      <table className="q-table quality-comparison-table">
        <caption className="q-cap">{title}</caption>
        <thead><tr><th scope="col">Анализатор</th><th scope="col">Тема</th><th scope="col">Найдено</th><th scope="col">Лишние тревоги</th></tr></thead>
        <tbody>{Object.entries(models).flatMap(([model, label]) => {
          const run = runs[model];
          if (!run) return [];
          return (Object.keys(themes) as Array<keyof typeof themes>).map(theme => {
            const { tp, fp, fn, tn } = run.production[theme];
            return <tr key={model + theme}>
              <th scope="row">{label}</th><td>{themes[theme]}</td><td>{tp} из {tp + fn}</td><td>{fp} из {fp + tn}</td>
            </tr>;
          });
        })}</tbody>
      </table>
    </div>
  );
}

export function ValidationResults() {
  const tested = results.unseen32['run-final'];
  const { tp, fn, fp, tn } = tested.production.attrition;
  const dictionary = results.unseen32['run-offline-final'].production.attrition;
  return (
    <details className="quality-method quality-method--experiment">
      <summary id="q-experiment" aria-labelledby="q-experiment-label q-experiment-name" aria-describedby="q-experiment-result">
        <span className="quality-method__label" id="q-experiment-label">Отдельный эксперимент</span>
        <span className="quality-method__name" id="q-experiment-name">Qwen 7B со словарём</span>
        <span className="quality-method__result" id="q-experiment-result">На {tested.n} искусственных комментариях найдено {tp} из {tp + fn} случаев риска ухода. Лишние тревоги — в {fp} из {fp + tn} примеров без риска. Один словарь нашёл {dictionary.tp} из {dictionary.tp + dictionary.fn} случаев.</span>
        <span className="quality-chevron" aria-hidden="true" />
      </summary>
      <div className="quality-panel__body">
        <p className="quality-source"><b>Источник</b> Контрольный прогон команды от 22 ноября 2026 года. Тексты и ключ с известными темами зафиксировали до прогона.</p>
        <ResultTable group="unseen32" title="Одна выборка для обоих анализаторов · 32 новых искусственных комментария" />
        <p className="qm__means">«Найдено» показывает, сколько известных случаев отмечено. «Лишние тревоги» показывает, сколько раз метка появилась в комментариях без этой темы.</p>
        <h3 className="q-h3">Что анализатор пропускает на контрольных примерах</h3>
        <dl className="quality-examples">{examples.map(example => <div key={example.id}>
          <dt>{example.title}</dt>
          <dd>«{example.text}»</dd>
        </div>)}</dl>
        <details className="quality-archive">
          <summary>Ранние серии и исходная модель Qwen 3B</summary>
          <ResultTable group="challenge48" title="48 сложных комментариев" />
          <ResultTable group="confirm16" title="16 дополнительных комментариев" />
        </details>
      </div>
    </details>
  );
}
