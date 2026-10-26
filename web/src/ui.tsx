// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT
import { useEffect, useId, useRef, type ReactNode } from 'react';
import type { Level } from './api';
import { LEVELS } from './model';

export type IconName =
  | 'overview'
  | 'table'
  | 'trend'
  | 'settings'
  | 'sun'
  | 'moon'
  | 'monitor'
  | 'plus'
  | 'close'
  | 'refresh'
  | 'download'
  | 'upload'
  | 'print'
  | 'search'
  | 'chevron'
  | 'arrow'
  | 'check'
  | 'info'
  | 'alert'
  | 'star'
  | 'logout'
  | 'login'
  | 'calendar'
  | 'file'
  | 'heart'
  | 'external'
  | 'eye'
  | 'eyeOff'
  | 'trash';

const paths: Record<IconName, ReactNode> = {
  overview: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1.5" />
      <rect x="14" y="3" width="7" height="7" rx="1.5" />
      <rect x="3" y="14" width="7" height="7" rx="1.5" />
      <rect x="14" y="14" width="7" height="7" rx="1.5" />
    </>
  ),
  table: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M3 9h18M9 4v16M3 14h18" />
    </>
  ),
  trend: (
    <>
      <path d="M3 3v18h18M6 15l5-6 4 3 6-8" />
      <path d="M17 4h4v4" />
    </>
  ),
  settings: (
    <>
      <path d="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z" />
      <path d="m10 3-.6 2.1-2 .9-2-.7-2 3.4L5 10v2l-1.6 1.4 2 3.4 2-.6 2 1 .6 2h4l.6-2 2-1 2 .6 2-3.4L19 12v-2l1.6-1.3-2-3.4-2 .7-2-.9L14 3Z" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5" />
    </>
  ),
  moon: <path d="M20 14.5A8.5 8.5 0 0 1 9.5 4 8.5 8.5 0 1 0 20 14.5Z" />,
  monitor: (
    <>
      <rect x="3" y="3" width="18" height="13" rx="2" />
      <path d="M8 21h8m-4-5v5" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  close: <path d="m6 6 12 12M6 18 18 6" />,
  refresh: (
    <>
      <path d="M20 7v5h-5M4 17v-5h5" />
      <path d="M5.7 7a7 7 0 0 1 11.6-2L20 8M4 16l2.7 3A7 7 0 0 0 18.3 17" />
    </>
  ),
  download: (
    <>
      <path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5" />
    </>
  ),
  upload: (
    <>
      <path d="M12 16V4m-5 5 5-5 5 5M4 16v5h16v-5" />
    </>
  ),
  print: (
    <>
      <path d="M7 8V3h10v5M7 16H3v-8h18v8h-4" />
      <rect x="7" y="13" width="10" height="8" />
      <path d="M17 11h1" />
    </>
  ),
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m16 16 5 5" />
    </>
  ),
  chevron: <path d="m9 5 7 7-7 7" />,
  arrow: <path d="M4 12h16m-6-6 6 6-6 6" />,
  check: <path d="m5 12 4 4L19 6" />,
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v6m0-10v.1" />
    </>
  ),
  alert: (
    <>
      <path d="m12 3 10 18H2Z" />
      <path d="M12 9v5m0 3v.1" />
    </>
  ),
  star: <path d="m12 3 2.8 5.8 6.4.9-4.6 4.5 1.1 6.3L12 17.5l-5.7 3 1.1-6.3-4.6-4.5 6.4-.9Z" />,
  logout: (
    <>
      <path d="M9 4H4v16h5m6-13 5 5-5 5M9 12h11" />
    </>
  ),
  login: (
    <>
      <path d="M15 4h5v16h-5M9 7l5 5-5 5M3 12h11" />
    </>
  ),
  calendar: (
    <>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M7 3v4m10-4v4M3 11h18M7 15h3m4 0h3" />
    </>
  ),
  file: (
    <>
      <path d="M14 3H5v18h14V8Zm0 0v5h5M8 12h8m-8 4h6" />
    </>
  ),
  heart: (
    <>
      <path d="M20 5a5 5 0 0 0-8 1 5 5 0 0 0-8-1c-4 4 0 9 8 15 8-6 12-11 8-15Z" />
      <path d="M4 12h4l2-4 3 8 2-4h5" />
    </>
  ),
  external: (
    <>
      <path d="M14 3h7v7M10 14 21 3M10 3H3v18h18v-7" />
    </>
  ),
  eye: (
    <>
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  eyeOff: (
    <>
      <path d="m3 3 18 18M10 5c7-1 12 7 12 7s-1 2-3 4M6 6a21 21 0 0 0-4 6s4 7 10 7c2 0 4-.7 5-1.5" />
    </>
  ),
  trash: (
    <>
      <path d="M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7m4-7v7" />
    </>
  ),
};

export function Icon({
  name,
  size = 20,
  className = '',
}: {
  name: IconName;
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
    >
      {paths[name]}
    </svg>
  );
}

export function Copyright({ className = '' }: { className?: string }) {
  return (
    <small className={`copyright ${className}`}>
      © {new Date().getFullYear()} bestkakkoii llc. All rights reserved.
    </small>
  );
}

export function LevelBadge({ level, compact = false }: { level: Level | null; compact?: boolean }) {
  return (
    <span className={`level-badge level-${level ?? 0}`}>
      <span className="level-dot" />
      {level ? (
        <>
          <b>L{level}</b>
          <span>{compact ? LEVELS[level].short : LEVELS[level].label}</span>
        </>
      ) : (
        <span>未判定</span>
      )}
    </span>
  );
}

export function Legend() {
  return (
    <div className="level-legend" aria-label="顏色分級說明">
      {([1, 2, 3, 4, 5] as Level[]).map((level) => (
        <LevelBadge key={level} level={level} compact />
      ))}
      <LevelBadge level={null} />
    </div>
  );
}

export function Modal({
  title,
  subtitle,
  children,
  onClose,
  wide = false,
  busy = false,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  busy?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const descriptionId = useId();
  useEffect(() => {
    const dialog = ref.current;
    const previous = document.activeElement as HTMLElement | null;
    dialog?.showModal();
    return () => {
      dialog?.close();
      previous?.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={`modal ${wide ? 'modal-wide' : ''}`}
      aria-labelledby={titleId}
      aria-describedby={subtitle ? descriptionId : undefined}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <div className="modal-header">
        <div>
          <h2 id={titleId}>{title}</h2>
          {subtitle && <p id={descriptionId}>{subtitle}</p>}
        </div>
        <button
          type="button"
          className="icon-button"
          onClick={onClose}
          aria-label="關閉視窗"
          disabled={busy}
        >
          <Icon name="close" />
        </button>
      </div>
      <div className="modal-content">{children}</div>
      <div className="modal-footer">
        <button type="button" className="button secondary" onClick={onClose} disabled={busy}>
          關閉
        </button>
        <Copyright />
      </div>
    </dialog>
  );
}

export function Alert({
  children,
  tone = 'info',
}: {
  children: ReactNode;
  tone?: 'info' | 'error' | 'success' | 'warning';
}) {
  return (
    <div className={`notice notice-${tone}`} role={tone === 'error' ? 'alert' : undefined}>
      <Icon
        name={
          tone === 'error' || tone === 'warning' ? 'alert' : tone === 'success' ? 'check' : 'info'
        }
      />
      <div>{children}</div>
    </div>
  );
}

export function Loading({ label = '資料處理中…' }: { label?: string }) {
  return (
    <span className="loading" role="status">
      <span className="spinner" />
      {label}
    </span>
  );
}
