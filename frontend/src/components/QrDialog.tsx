import { useEffect, useMemo, useRef } from 'react';
import qrcode from 'qrcode-generator';

/** QR-код как SVG-путь: генерация на клиенте, без сетевых вызовов. */
export function QrCode({ text, size = 280 }: { text: string; size?: number }) {
  const { d, n } = useMemo(() => {
    const qr = qrcode(0, 'M');
    qr.addData(text);
    qr.make();
    const n = qr.getModuleCount();
    let d = '';
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) d += `M${c + 4} ${r + 4}h1v1h-1z`;
    return { d, n: n + 8 }; // поле в 4 модуля — по стандарту, иначе телефон хуже читает
  }, [text]);
  return (
    <svg className="qr" viewBox={`0 0 ${n} ${n}`} width={size} height={size} role="img" aria-label={`QR-код ссылки ${text}`} shapeRendering="crispEdges">
      <rect width={n} height={n} fill="#fff" />
      <path d={d} fill="#000" />
    </svg>
  );
}

/** Абсолютный адрес: в онлайн-демо ссылки относительные (/form/…). */
export const absoluteUrl = (url: string) => {
  try {
    return new URL(url, window.location.origin).href;
  } catch {
    return url;
  }
};

/** Телефон жюри не откроет адрес ноутбука вида localhost. */
export const isLoopback = (url: string) => /^https?:\/\/(localhost|127\.\d+\.\d+\.\d+|\[::1\])(?=[:/]|$)/i.test(url);

/** Модальное окно с крупным QR-кодом анкеты. Esc и клик по фону закрывают, фокус возвращается на кнопку. */
export function QrDialog({ url, title, onClose }: { url: string; title: string; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const href = absoluteUrl(url);
  useEffect(() => {
    const back = document.activeElement as HTMLElement | null;
    box.current?.querySelector<HTMLElement>('button')?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      if (e.key === 'Tab' && box.current) {
        // фокус не уходит из окна
        const f = box.current.querySelectorAll<HTMLElement>('a[href], button');
        const first = f[0];
        const last = f[f.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      back?.focus?.();
    };
  }, [onClose]);
  return (
    <div className="modal" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className="modal__box qr-box" role="dialog" aria-modal="true" aria-labelledby="qr-title">
        <div className="modal__head">
          <h2 id="qr-title" className="sheet__h">
            {title}
          </h2>
          <button type="button" className="btn btn--ghost btn--small" onClick={onClose}>
            Закрыть
          </button>
        </div>
        <QrCode text={href} />
        <p className="qr-box__lead">Для жюри: сканируйте телефоном</p>
        <a className="qr-box__url" href={href} target="_blank" rel="noreferrer">
          {href}
        </a>
        {isLoopback(href) && (
          <p className="qr-box__warn" role="note">
            Телефон не откроет localhost — задайте <code>{'PUBLIC_URL=http://<IP ноутбука>:8080'}</code> и перезапустите сервер. Телефон и ноутбук должны быть в одной сети.
          </p>
        )}
      </div>
    </div>
  );
}
