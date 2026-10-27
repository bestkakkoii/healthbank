// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  ApiError,
  api,
  defaultSettings,
  readableError,
  setCsrfToken,
  type CatalogResponse,
  type ItemOverride,
  type LabRecord,
  type ReportResponse,
  type Session,
  type Settings,
  type Theme,
} from './api';
import {
  BaselineDialog,
  clearRememberedLogin,
  DetailDialog,
  ImportDialog,
  LoginDialog,
} from './Dialogs';
import { ComparisonTable } from './ComparisonTable';
import { TrendChart } from './TrendChart';
import {
  categoryLabel,
  dateLabel,
  downloadText,
  formatTime,
  groupRecords,
  referenceLabel,
  referenceTemplateLabel,
  rulePreview,
  toCsv,
  yearLabel,
  type MeasurementGroup,
} from './model';
import { Alert, Copyright, Icon, Legend, LevelBadge, Loading, Modal, type IconName } from './ui';

type View = 'overview' | 'comparison' | 'trends' | 'settings';
const NAV: { id: View; label: string; short: string; icon: IconName; eyebrow: string }[] = [
  {
    id: 'overview',
    label: '檢驗總覽',
    short: '總覽',
    icon: 'overview',
    eyebrow: 'YOUR HEALTH, IN PERSPECTIVE',
  },
  {
    id: 'comparison',
    label: '年度比較',
    short: '比較',
    icon: 'table',
    eyebrow: 'EVERY RESULT, SIDE BY SIDE',
  },
  {
    id: 'trends',
    label: '數值趨勢',
    short: '趨勢',
    icon: 'trend',
    eyebrow: 'A CLEARER VIEW OVER TIME',
  },
  {
    id: 'settings',
    label: '個人設定',
    short: '設定',
    icon: 'settings',
    eyebrow: 'YOUR PERSONAL REFERENCE',
  },
];
const EMPTY_REPORT: ReportResponse = { records: [], fetched_at: '', complete: true, warnings: [] };
const EMPTY_CATALOG: CatalogResponse = { items: [], sources: [] };
const APPEARANCE_KEY = 'bestkakkoi.healthbank.appearance';
const today = () => new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Taipei' });

function initialSettings(): Settings {
  const settings = defaultSettings();
  try {
    const raw = localStorage.getItem(APPEARANCE_KEY);
    if (raw) {
      const value = JSON.parse(raw);
      if (['light', 'dark', 'system'].includes(value.theme)) settings.theme = value.theme;
      if ([1, 1.1, 1.2, 1.5, 2].includes(value.font_scale)) settings.font_scale = value.font_scale;
    }
  } catch {
    /* Storage is optional. */
  }
  return settings;
}

function Panel({
  title,
  subtitle,
  action,
  children,
  className = '',
}: {
  title: string;
  subtitle?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel ${className}`}>
      <div className="panel-heading">
        <div>
          <h2>{title}</h2>
          {subtitle && <p>{subtitle}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

export default function App() {
  const [session, setSession] = useState<Session>(() => ({
    authenticated: false,
    mode: 'anonymous',
    settings: initialSettings(),
    version: '',
  }));
  const [report, setReport] = useState<ReportResponse>(EMPTY_REPORT);
  const [catalog, setCatalog] = useState<CatalogResponse>(EMPTY_CATALOG);
  const [view, setView] = useState<View>('overview');
  const [booting, setBooting] = useState(true);
  const [busy, setBusy] = useState('');
  const [settingsBusy, setSettingsBusy] = useState(false);
  const settingsLock = useRef(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [assessmentStale, setAssessmentStale] = useState(false);
  const [loginOpen, setLoginOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [detail, setDetail] = useState<LabRecord | null>(null);
  const [baseline, setBaseline] = useState<MeasurementGroup | null>(null);
  const [restoreBaselinesOpen, setRestoreBaselinesOpen] = useState(false);
  const [restoreBaselinesError, setRestoreBaselinesError] = useState('');
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('all');
  const [year, setYear] = useState('all');
  const [onlyFavorites, setOnlyFavorites] = useState(false);
  const [onlyAbnormal, setOnlyAbnormal] = useState(false);
  const [granularity, setGranularity] = useState<'date' | 'year'>('year');
  const [selectedGroupId, setSelectedGroupId] = useState('');
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');
  const [rangeOpen, setRangeOpen] = useState(false);
  const mainHeading = useRef<HTMLHeadingElement>(null);
  const settings = session.settings;
  const persisted = session.authenticated && session.mode === 'official';
  const isDemo = session.mode === 'demo';

  useEffect(() => {
    const controller = new AbortController();
    async function initialize() {
      try {
        // Establish the session cookie before issuing other session-bearing requests.
        const fetched = await api.session(controller.signal);
        if (controller.signal.aborted) return;
        setCsrfToken(fetched.csrf_token);
        setSession(
          fetched.mode !== 'official'
            ? {
                ...fetched,
                settings: {
                  ...fetched.settings,
                  theme: initialSettings().theme,
                  font_scale: initialSettings().font_scale,
                },
              }
            : fetched,
        );
        const results = await Promise.allSettled([
          api.catalog(controller.signal),
          fetched.mode !== 'anonymous'
            ? api.currentReports(controller.signal)
            : Promise.resolve(EMPTY_REPORT),
        ]);
        if (controller.signal.aborted) return;
        if (results[0].status === 'fulfilled') setCatalog(results[0].value);
        else setError(`參考項目目錄暫時無法載入。${readableError(results[0].reason)}`);
        if (results[1].status === 'fulfilled') {
          const current = results[1].value;
          setReport(current);
          if (current.official_query) {
            setDateFrom(current.official_query.start_date);
            setDateTo(current.official_query.end_date);
          }
        } else setError(readableError(results[1].reason));
      } catch (error) {
        if (!controller.signal.aborted) setError(readableError(error));
      } finally {
        if (!controller.signal.aborted) setBooting(false);
      }
    }
    void initialize();
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const applyTheme = () => {
      const activeTheme =
        settings.theme === 'system' ? (media.matches ? 'dark' : 'light') : settings.theme;
      document.documentElement.dataset.theme = activeTheme;
      document.documentElement.style.colorScheme = activeTheme;
      document.documentElement.style.setProperty('--font-scale', String(settings.font_scale || 1));
      document
        .querySelector('meta[name="theme-color"]')
        ?.setAttribute('content', activeTheme === 'dark' ? '#111d24' : '#f5f7f7');
    };
    applyTheme();
    media.addEventListener('change', applyTheme);
    try {
      localStorage.setItem(
        APPEARANCE_KEY,
        JSON.stringify({ theme: settings.theme, font_scale: settings.font_scale }),
      );
    } catch {
      /* The application works without local storage. */
    }
    return () => media.removeEventListener('change', applyTheme);
  }, [settings.theme, settings.font_scale]);

  useEffect(() => {
    if (!message) return;
    const timeout = window.setTimeout(() => setMessage(''), 6500);
    return () => window.clearTimeout(timeout);
  }, [message]);

  const groups = useMemo(() => groupRecords(report.records), [report.records]);
  const years = useMemo(
    () =>
      [...new Set(report.records.map((record) => record.exam_date.slice(0, 4)).filter(Boolean))]
        .sort()
        .reverse(),
    [report.records],
  );
  const filteredGroups = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return groups
      .map((group) => ({
        ...group,
        records: group.records.filter(
          (record) => year === 'all' || record.exam_date.startsWith(`${year}-`),
        ),
      }))
      .filter(
        (group) =>
          group.records.length &&
          (category === 'all' || group.category === category) &&
          (!onlyFavorites || settings.favorites.includes(group.configurationKey)) &&
          (!onlyAbnormal ||
            group.records.some(
              (record) =>
                (record.level !== null && record.level >= 3) ||
                record.reference_status === 'high' ||
                record.reference_status === 'low' ||
                record.official_flag === '1',
            )) &&
          (!normalized ||
            `${group.name} ${group.abbreviation} ${group.unit} ${group.records.map((record) => record.order_code).join(' ')}`
              .toLocaleLowerCase()
              .includes(normalized)),
      );
  }, [groups, category, year, onlyFavorites, onlyAbnormal, settings.favorites, query]);
  const selectedGroup = groups.find((group) => group.id === selectedGroupId) ?? groups[0];
  const trendGroup = selectedGroup
    ? {
        ...selectedGroup,
        records: selectedGroup.records.filter(
          (record) => year === 'all' || record.exam_date.startsWith(`${year}-`),
        ),
      }
    : undefined;
  const uniqueDates = [...new Set(report.records.map((record) => record.exam_date))]
    .sort()
    .reverse();
  const latestRecords = groups.flatMap((group) =>
    group.records.filter((record) => record.exam_date === group.records[0].exam_date),
  );
  const attentionRecords = latestRecords
    .filter(
      (record) =>
        (record.level && record.level >= 3) ||
        record.reference_status === 'high' ||
        record.reference_status === 'low' ||
        record.official_flag === '1',
    )
    .sort((a, b) => (b.level ?? 0) - (a.level ?? 0) || b.exam_date.localeCompare(a.exam_date));
  const priorityCount = latestRecords.filter((record) => record.level && record.level >= 4).length;
  const catalogByKey = useMemo(
    () => new Map(catalog.items.map((item) => [item.configuration_key, item])),
    [catalog.items],
  );
  const configuredGroups = useMemo(() => {
    // The catalog owns defaults. Observations add real history, never placeholder results.
    const configured = new Map<string, MeasurementGroup>();
    for (const item of catalog.items) {
      if (!item.configuration_key || item.category === 'metadata') continue;
      configured.set(item.configuration_key, {
        id: `${item.id}\u0001${item.unit}`,
        configurationKey: item.configuration_key,
        name: item.display_name,
        abbreviation: item.aliases.find((alias) => alias !== item.display_name) || item.display_name,
        unit: item.unit || '無單位',
        category: item.category || 'other',
        records: [],
      });
    }
    for (const item of Object.values(settings.known_items ?? {})) {
      if (!item.configuration_key || item.category === 'metadata' || configured.has(item.configuration_key))
        continue;
      configured.set(item.configuration_key, {
        id: `${item.id}\u0001${item.unit}`,
        configurationKey: item.configuration_key,
        name: item.name,
        abbreviation: item.name,
        unit: item.unit || '無單位',
        category: item.category || 'other',
        records: [],
      });
    }
    for (const group of groups) {
      if (!group.configurationKey || group.category === 'metadata') continue;
      const current = configured.get(group.configurationKey);
      if (current) current.records.push(...group.records);
      else if (group.records.some((record) => !['missing', 'metadata'].includes(record.value_kind)))
        configured.set(group.configurationKey, { ...group, records: [...group.records] });
    }
    for (const group of configured.values())
      group.records.sort((a, b) => b.exam_date.localeCompare(a.exam_date) || a.id.localeCompare(b.id));
    return [...configured.values()].sort(
      (a, b) =>
        categoryLabel(a.category).localeCompare(categoryLabel(b.category), 'zh-Hant') ||
        a.name.localeCompare(b.name, 'zh-Hant'),
    );
  }, [catalog.items, groups, settings.known_items]);
  const categories = useMemo(
    () =>
      [...new Set((view === 'settings' ? configuredGroups : groups).map((group) => group.category))]
        .sort((a, b) => categoryLabel(a).localeCompare(categoryLabel(b), 'zh-Hant')),
    [view, configuredGroups, groups],
  );
  const configurableKeys = configuredGroups.map((group) => group.configurationKey);
  const customBaselineCount = Object.keys(settings.overrides).length;
  const builtinCount = configuredGroups.filter((group) => catalogByKey.get(group.configurationKey)?.builtin).length;
  const visibleConfigurationGroups = configuredGroups.filter(
    (group) =>
      (category === 'all' || group.category === category) &&
      `${group.name} ${group.abbreviation} ${group.unit} ${catalogByKey.get(group.configurationKey)?.aliases.join(' ') ?? ''} ${catalogByKey.get(group.configurationKey)?.order_codes.join(' ') ?? ''} ${group.records.map((record) => record.order_code).join(' ')}`
        .toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
  );
  const nav = NAV.find((item) => item.id === view)!;

  function navigate(next: View) {
    setView(next);
    setRangeOpen(false);
    window.scrollTo({ top: 0, behavior: 'auto' });
    window.requestAnimationFrame(() => mainHeading.current?.focus({ preventScroll: true }));
  }
  function resetFilters() {
    setQuery('');
    setCategory('all');
    setYear('all');
    setOnlyFavorites(false);
    setOnlyAbnormal(false);
  }
  function openTrend(id: string) {
    setSelectedGroupId(id);
    setYear('all');
    setDetail(null);
    navigate('trends');
  }

  async function refreshSession() {
    const next = await api.session();
    setCsrfToken(next.csrf_token);
    setSession(
      next.mode === 'official'
        ? next
        : {
            ...next,
            settings: {
              ...next.settings,
              theme: initialSettings().theme,
              font_scale: initialSettings().font_scale,
            },
          },
    );
    return next;
  }

  async function receiveReports(next: ReportResponse) {
    setReport(next);
    setAssessmentStale(false);
    setError('');
    resetFilters();
    const results = await Promise.allSettled([refreshSession(), api.catalog()]);
    if (results[0].status === 'rejected')
      setError(`資料已取得，但使用階段狀態無法更新。${readableError(results[0].reason)}`);
    if (results[1].status === 'fulfilled') setCatalog(results[1].value);
    else setError(`資料已取得，但參考項目目錄無法更新。${readableError(results[1].reason)}`);
  }

  async function loggedIn(next: Session) {
    setSession(next);
    setCsrfToken(next.csrf_token);
    setReport(EMPTY_REPORT);
    setDetail(null);
    setBaseline(null);
    clearSessionCatalog();
    setLoginOpen(false);
    navigate('overview');
    await fetchReports(true);
  }

  async function expireOfficialSession() {
    setReport(EMPTY_REPORT);
    setDetail(null);
    setBaseline(null);
    clearSessionCatalog();
    resetFilters();
    setSession((current) => ({
      ...current,
      authenticated: false,
      mode: 'anonymous',
      settings: {
        ...defaultSettings(),
        theme: current.settings.theme,
        font_scale: current.settings.font_scale,
      },
    }));
    try {
      // An expired application cookie may have been replaced by the failed GET.
      const current = await api.session();
      setCsrfToken(current.csrf_token);
      const next = await api.logout();
      setCsrfToken(next.csrf_token);
    } catch {
      /* Local report is already removed; the next login renews its session and CAPTCHA. */
    }
  }

  async function fetchReports(useOfficialDefault = false) {
    const from = useOfficialDefault ? '' : dateFrom;
    const to = useOfficialDefault ? '' : dateTo;
    if ((from || to) && (!from || !to || from > to)) {
      setError('請選擇有效的開始與結束日期，開始日期不可晚於結束日期。');
      return;
    }
    setBusy('正在向健康存摺取得檢驗資料…');
    setError('');
    try {
      const result = await api.reports(from, to);
      await receiveReports(result);
      if (result.official_query) {
        setDateFrom(result.official_query.start_date);
        setDateTo(result.official_query.end_date);
      }
      setRangeOpen(false);
      setMessage('檢驗資料已更新。');
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        await expireOfficialSession();
        setError('官方登入階段已結束，目前報告已清除。請重新連接健康存摺。');
      } else setError(readableError(error));
    } finally {
      setBusy('');
    }
  }

  async function importFile(file: File) {
    const next = await api.importReports(file);
    await receiveReports(next);
    setImportOpen(false);
    navigate('overview');
    setMessage(`已整理 ${next.records.length} 筆檢驗紀錄。`);
  }

  async function openDemo() {
    setBusy('正在載入合成示範資料…');
    setError('');
    try {
      await receiveReports(await api.demo());
      navigate('overview');
    } catch (error) {
      setError(readableError(error));
    } finally {
      setBusy('');
    }
  }

  async function logout() {
    setBusy('正在結束使用階段…');
    setError('');
    try {
      const next = await api.logout();
      setSession({
        ...next,
        settings: { ...next.settings, theme: settings.theme, font_scale: settings.font_scale },
      });
      setCsrfToken(next.csrf_token);
      setReport(EMPTY_REPORT);
      setDetail(null);
      setBaseline(null);
      clearSessionCatalog();
      resetFilters();
      navigate('overview');
      setMessage('已結束使用階段並清除目前報告。');
    } catch (error) {
      setError(readableError(error));
    } finally {
      setBusy('');
    }
  }

  function clearSessionCatalog() {
    setRestoreBaselinesOpen(false);
    setCatalog((current) => ({
      ...current,
      items: current.items.filter((item) => item.builtin).map((item) => ({
        ...item,
        reference_low: null,
        reference_high: null,
      })),
    }));
  }

  async function commitSettings(next: Settings, reassess = false) {
    if (settingsLock.current) throw new Error('上一個設定正在儲存，請稍候再操作。');
    settingsLock.current = true;
    setSettingsBusy(true);
    try {
      const saved = await api.saveSettings(next);
      setSession((current) => ({ ...current, settings: saved }));
      if (reassess && report.records.length) {
        setAssessmentStale(true);
        try {
          const current = await api.currentReports();
          setReport(current);
          setAssessmentStale(false);
        } catch (error) {
          throw new Error(`設定已儲存，但報告顏色尚未重新判讀。${readableError(error)}`, { cause: error });
        }
      }
    } finally {
      settingsLock.current = false;
      setSettingsBusy(false);
    }
  }

  function changeAppearance(patch: Partial<Settings>) {
    void commitSettings({ ...settings, ...patch }).catch((error) => setError(readableError(error)));
  }
  function toggleFavorite(key: string) {
    const favorites = settings.favorites.includes(key)
      ? settings.favorites.filter((value) => value !== key)
      : [...settings.favorites, key];
    void commitSettings({ ...settings, favorites }).catch((error) =>
      setError(readableError(error)),
    );
  }
  async function saveOverride(key: string, value: ItemOverride | null) {
    const overrides = { ...settings.overrides };
    if (value === null) delete overrides[key];
    else overrides[key] = value;
    await commitSettings({ ...settings, overrides }, true);
    setMessage(
      value === null
        ? '此項目已還原預設規則。'
        : persisted
          ? '個人基準已儲存，報告顏色已更新。'
          : '本次使用的個人基準已更新。',
    );
  }
  async function restoreAllBaselines() {
    setRestoreBaselinesError('');
    try {
      await commitSettings({ ...settings, overrides: {} }, true);
      setRestoreBaselinesOpen(false);
      setMessage('所有個人基準已還原為預設規則，關注項目與閱讀設定已保留。');
    } catch (error) {
      setRestoreBaselinesError(readableError(error));
    }
  }

  function exportCsv() {
    const records =
      view === 'trends' && trendGroup
        ? trendGroup.records
        : filteredGroups.flatMap((group) => group.records);
    downloadText(
      toCsv(records),
      `檢驗比較${isDemo ? '-合成示範' : ''}-${today()}.csv`,
      'text/csv;charset=utf-8',
    );
  }

  const toolbar = (
    <div className="filter-bar">
      <label className="search-field">
        <Icon name="search" />
        <span className="sr-only">搜尋檢驗項目或醫令代碼</span>
        <input
          type="search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="搜尋項目、縮寫或醫令代碼"
        />
      </label>
      <label className="select-field">
        <span className="sr-only">檢驗類別</span>
        <select value={category} onChange={(event) => setCategory(event.target.value)}>
          <option value="all">全部類別</option>
          {categories.map((value) => (
            <option key={value} value={value}>
              {categoryLabel(value)}
            </option>
          ))}
        </select>
      </label>
      {view !== 'settings' && (
        <label className="select-field">
          <span className="sr-only">比較年度</span>
          <select value={year} onChange={(event) => setYear(event.target.value)}>
            <option value="all">全部年度</option>
            {years.map((value) => (
              <option key={value} value={value}>
                {yearLabel(value)}
              </option>
            ))}
          </select>
        </label>
      )}
      {view !== 'settings' && (
        <>
          <button
            className={`filter-chip ${onlyFavorites ? 'active' : ''}`}
            aria-pressed={onlyFavorites}
            onClick={() => setOnlyFavorites(!onlyFavorites)}
          >
            <Icon name="star" size={17} />
            關注項目
          </button>
          <button
            className={`filter-chip ${onlyAbnormal ? 'active' : ''}`}
            aria-pressed={onlyAbnormal}
            onClick={() => setOnlyAbnormal(!onlyAbnormal)}
          >
            異常項目
          </button>
        </>
      )}
    </div>
  );

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要內容
      </a>
      <aside className="sidebar" aria-label="主要導覽">
        <div className="brand">
          <div className="brand-mark">
            <Icon name="heart" size={28} />
          </div>
          <div>
            <b>健康映覽</b>
            <span>HEALTHBANK · BY BESTKAKKOII</span>
          </div>
        </div>
        <div className="workspace-label">個人健康工作站</div>
        <nav>
          {NAV.map((item) => (
            <button
              key={item.id}
              className={`nav-item ${view === item.id ? 'active' : ''}`}
              onClick={() => navigate(item.id)}
              aria-current={view === item.id ? 'page' : undefined}
            >
              <Icon name={item.icon} />
              <span>{item.label}</span>
              {view === item.id && <span className="nav-active-dot" />}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="session-card">
            <span className={`status-dot ${persisted ? 'connected' : ''}`} />
            <div>
              <b>
                {isDemo
                  ? '合成示範資料'
                  : persisted
                    ? '已連接健康存摺'
                    : session.mode === 'import'
                      ? '檔案匯入模式'
                      : '尚未連接資料'}
              </b>
              <span>{persisted ? '個人設定跨裝置同步' : '檢驗資料僅供本次瀏覽'}</span>
            </div>
          </div>
          <div className="sidebar-meta">
            <span>bestkakkoii llc.</span>
            <span>{session.version ? `v${session.version.replace(/^v/, '')}` : 'v1'}</span>
          </div>
        </div>
      </aside>

      <div className="main-shell">
        <header className="topbar">
          <div className="mobile-brand">
            <Icon name="heart" size={24} />
            <b>健康映覽</b>
          </div>
          <div className="breadcrumb">
            個人健康工作站 <span>/</span> <b>{nav.label}</b>
          </div>
          <div className="topbar-actions">
            <button
              className="icon-button"
              title={`目前：${settings.theme === 'system' ? '跟隨系統' : settings.theme === 'light' ? '淺色模式' : '深色模式'}；點選切換`}
              aria-label="切換淺色、深色與系統外觀"
              onClick={() =>
                changeAppearance({
                  theme:
                    settings.theme === 'system'
                      ? 'light'
                      : settings.theme === 'light'
                        ? 'dark'
                        : 'system',
                })
              }
              disabled={settingsBusy || booting}
            >
              <Icon
                name={
                  settings.theme === 'system'
                    ? 'monitor'
                    : settings.theme === 'light'
                      ? 'sun'
                      : 'moon'
                }
              />
            </button>
            {session.mode !== 'anonymous' && (
              <button
                className="button quiet topbar-session"
                onClick={() => void logout()}
                disabled={!!busy}
              >
                <Icon name="logout" size={18} />
                <span>{persisted ? '登出' : '結束瀏覽'}</span>
              </button>
            )}
          </div>
        </header>

        <main id="main-content" className="main-content">
          <div className="page-heading">
            <div>
              <span className="eyebrow">{nav.eyebrow}</span>
              <h1 ref={mainHeading} tabIndex={-1}>
                {nav.label}
                <span className="title-dot">.</span>
              </h1>
              <p>
                {view === 'overview'
                  ? '將每一次檢驗，放回完整的時間脈絡。'
                  : view === 'comparison'
                    ? '保留每一筆原始結果，清楚比較年度與各次檢查。'
                    : view === 'trends'
                      ? '從長期變化，理解單次數值所在的位置。'
                      : '依照自己的追蹤需求，調整閱讀與判讀基準。'}
              </p>
            </div>
            <div className="heading-actions">
              {report.records.length > 0 && view !== 'settings' && (
                <>
                  <button
                    className="button secondary"
                    onClick={exportCsv}
                    disabled={assessmentStale}
                  >
                    <Icon name="download" size={18} />
                    <span>匯出 CSV</span>
                  </button>
                  <button
                    className="button secondary print-button"
                    aria-describedby={view === 'comparison' ? 'comparison-print-tip' : undefined}
                    title={
                      view === 'comparison'
                        ? '列印時每段最多 4 個檢驗項目欄，自動分段換頁；可先篩選類別或關注項目。'
                        : '開啟瀏覽器列印，可儲存為 PDF。'
                    }
                    onClick={() => window.print()}
                    disabled={assessmentStale}
                  >
                    <Icon name="print" size={18} />
                    <span>列印／PDF</span>
                  </button>
                </>
              )}
              {persisted ? (
                <button
                  className="button primary"
                  onClick={() => setRangeOpen(!rangeOpen)}
                  disabled={!!busy}
                >
                  <Icon name="refresh" size={18} />
                  更新資料
                </button>
              ) : (
                <button
                  className="button primary"
                  onClick={() => setLoginOpen(true)}
                  disabled={!!busy || booting}
                >
                  <Icon name="login" size={18} />
                  連接健康存摺
                </button>
              )}
            </div>
          </div>

          <div className="print-only print-heading">
            <p>{isDemo ? '合成示範報告 · 非真實醫療資料' : '個人檢驗報告'}</p>
            <p>
              範圍：{year === 'all' ? '全部年度' : `${year} 年`} · 資料取得時間：
              {formatTime(report.fetched_at)}（臺灣時間）
            </p>
          </div>
          {isDemo && (
            <div className="demo-banner">
              <Icon name="info" />
              <div>
                <b>目前為合成示範資料</b>
                <span>內容不屬於任何真實個人；設定只在示範使用階段生效。</span>
              </div>
              <button className="text-button" onClick={() => void logout()} disabled={!!busy}>
                離開示範
                <Icon name="arrow" size={16} />
              </button>
            </div>
          )}
          {error && (
            <div className="dismissible-notice">
              <Alert tone="error">{error}</Alert>
              <button
                className="icon-button"
                aria-label="關閉錯誤訊息"
                onClick={() => setError('')}
              >
                <Icon name="close" size={18} />
              </button>
            </div>
          )}
          {message && (
            <div className="toast" role="status">
              <Icon name="check" size={18} />
              {message}
            </div>
          )}
          {busy && (
            <div className="busy-strip">
              <Loading label={busy} />
            </div>
          )}
          {assessmentStale && (
            <Alert tone="warning">
              設定已儲存，但目前顏色尚未完成重新判讀。
              <button
                className="text-button"
                onClick={() => {
                  setBusy('正在重新套用設定…');
                  void api
                    .currentReports()
                    .then((result) => {
                      setReport(result);
                      setAssessmentStale(false);
                    })
                    .catch((error) => setError(readableError(error)))
                    .finally(() => setBusy(''));
                }}
              >
                重新套用設定
              </button>
            </Alert>
          )}
          {report.official_query && (
            <Alert>
              <strong>本次取得 {report.records.length} 筆原始結果</strong>
              <br />
              官方查詢期間：{dateLabel(report.official_query.start_date)}～
              {dateLabel(report.official_query.end_date)}；
              {report.official_query.reported_count === null
                ? '官方未提供總筆數。'
                : `官方回報 ${report.official_query.reported_count} 筆。`}
              完整性核對適用於這個期間。
            </Alert>
          )}
          {report.warnings
            .filter((warning) => !isDemo || !warning.startsWith('示範模式：'))
            .map((warning, index) => (
              <Alert tone="warning" key={`${warning}-${index}`}>
                {warning}
              </Alert>
            ))}
          {!report.complete && report.records.length > 0 && (
            <Alert tone="warning">
              目前資料可能不完整。比較表僅呈現已取得的紀錄；缺少的日期不表示未曾檢驗。
            </Alert>
          )}
          {rangeOpen && (
            <form
              className="date-range-panel"
              onSubmit={(event) => {
                event.preventDefault();
                void fetchReports();
              }}
            >
              <label className="field">
                開始日期
                <input
                  type="date"
                  value={dateFrom}
                  onChange={(event) => setDateFrom(event.target.value)}
                  max={dateTo}
                  required={!!dateTo}
                />
              </label>
              <span className="date-range-separator">至</span>
              <label className="field">
                結束日期
                <input
                  type="date"
                  value={dateTo}
                  onChange={(event) => setDateTo(event.target.value)}
                  min={dateFrom}
                  max={today()}
                  required={!!dateFrom}
                />
              </label>
              <button className="button primary" type="submit" disabled={!!busy}>
                取得此範圍
              </button>
              <button
                className="button secondary"
                type="button"
                disabled={!!busy}
                onClick={() => void fetchReports(true)}
              >
                官網預設期間
              </button>
            </form>
          )}

          {booting ? (
            <div className="boot-screen">
              <Loading label="正在開啟健康工作站…" />
            </div>
          ) : !report.records.length && view !== 'settings' ? (
            <section className="welcome-panel">
              <div className="welcome-copy">
                <span className="section-kicker">一眼看懂，長期追蹤</span>
                <h2>
                  您的檢驗紀錄，
                  <br />
                  值得更清楚的呈現。
                </h2>
                <p>將健康存摺中逐行列出的檢驗結果，整理成年度比較、個別趨勢與個人化參考基準。</p>
                <div className="welcome-actions">
                  <button
                    className="button primary large-button"
                    onClick={() => {
                      if (persisted) setRangeOpen(true);
                      else setLoginOpen(true);
                    }}
                    disabled={!!busy}
                  >
                    <Icon name={persisted ? 'refresh' : 'login'} />
                    {persisted ? '更新資料' : '連接健康存摺'}
                  </button>
                  <button
                    className="button secondary large-button"
                    onClick={() => setImportOpen(true)}
                    disabled={!!busy}
                  >
                    <Icon name="upload" />
                    匯入已下載網頁
                  </button>
                </div>
                <button
                  className="text-button demo-link"
                  onClick={() => void openDemo()}
                  disabled={!!busy}
                >
                  先看看功能示範 <span>合成資料</span>
                  <Icon name="arrow" size={17} />
                </button>
              </div>
              <div className="welcome-illustration" aria-hidden="true">
                <div className="illustration-top">
                  <div className="illustration-dot" />
                  <span>每一筆，都有脈絡</span>
                  <Icon name="trend" />
                </div>
                <div className="illustration-grid">
                  <span>檢驗項目</span>
                  <span>2024</span>
                  <span>2025</span>
                  <span>2026</span>
                  {['腎功能', '血脂', '血糖', '血液常規'].map((label, index) => (
                    <div className="illustration-row" key={label}>
                      <span>{label}</span>
                      <i className={index === 1 ? 'illustration-warm' : ''} />
                      <i className={index === 2 ? 'illustration-warm' : ''} />
                      <i />
                    </div>
                  ))}
                </div>
                <div className="illustration-bottom">
                  <Icon name="check" size={17} />
                  <span>年度比較 · 趨勢檢視 · 個人基準</span>
                </div>
              </div>
              <div className="welcome-features">
                <div>
                  <Icon name="table" />
                  <b>完整比較</b>
                  <span>同日多筆、不同院所，都保留。</span>
                </div>
                <div>
                  <Icon name="trend" />
                  <b>清晰趨勢</b>
                  <span>依實際日期，呈現數值變化。</span>
                </div>
                <div>
                  <Icon name="settings" />
                  <b>個人基準</b>
                  <span>依實際項目新增，跨裝置同步。</span>
                </div>
              </div>
            </section>
          ) : (
            <>
              {view === 'overview' && (
                <>
                  <div className="summary-grid">
                    <article className="summary-card">
                      <div className="summary-label">
                        <span>最近檢查日期</span>
                        <Icon name="calendar" />
                      </div>
                      <strong className="date-stat">{dateLabel(uniqueDates[0] ?? '')}</strong>
                      <p>共 {uniqueDates.length} 個檢查日期</p>
                    </article>
                    <article className="summary-card">
                      <div className="summary-label">
                        <span>追蹤檢驗項目</span>
                        <Icon name="file" />
                      </div>
                      <strong>
                        {groups.filter((group) => group.category !== 'metadata').length}
                        <small>項</small>
                      </strong>
                      <p>{categories.length} 個檢驗類別</p>
                    </article>
                    <article className={`summary-card ${priorityCount ? 'attention-card' : ''}`}>
                      <div className="summary-label">
                        <span>需要優先檢視</span>
                        <Icon name="alert" />
                      </div>
                      <strong>
                        {priorityCount}
                        <small>筆</small>
                      </strong>
                      <p>各項最近紀錄 · L4／L5</p>
                    </article>
                    <article className="summary-card">
                      <div className="summary-label">
                        <span>資料涵蓋年度</span>
                        <Icon name="table" />
                      </div>
                      <strong>
                        {years.length}
                        <small>年</small>
                      </strong>
                      <p>共 {report.records.length} 筆原始結果</p>
                    </article>
                  </div>
                  <div className="overview-columns">
                    <Panel
                      title="優先檢視"
                      subtitle="各項目最近一次的異常或原院標記"
                      action={
                        <button
                          className="text-button"
                          onClick={() => {
                            setOnlyAbnormal(true);
                            navigate('comparison');
                          }}
                        >
                          查看比較表
                          <Icon name="arrow" size={17} />
                        </button>
                      }
                      className="attention-panel"
                    >
                      {attentionRecords.length ? (
                        <div className="attention-list">
                          {attentionRecords.slice(0, 8).map((record) => (
                            <button
                              className="attention-row"
                              key={record.id}
                              onClick={() => setDetail(record)}
                            >
                              <span
                                className={`attention-marker level-surface-${record.level ?? 0}`}
                              />
                              <span className="attention-name">
                                <b>{record.display_name || record.item_name}</b>
                                <small>
                                  {dateLabel(record.exam_date)} ·{' '}
                                  {record.institution || categoryLabel(record.category)}
                                </small>
                              </span>
                              <span className="attention-value">
                                <strong>{record.result}</strong>
                                <small>{record.unit}</small>
                              </span>
                              <LevelBadge level={record.level} compact />
                              <Icon name="chevron" size={18} />
                            </button>
                          ))}
                        </div>
                      ) : (
                        <div className="quiet-empty">
                          <Icon name="check" size={30} />
                          <h3>目前沒有列入優先檢視的項目</h3>
                          <p>這表示目前取得的最近紀錄未觸發上述條件，仍可在比較表查看全部結果。</p>
                        </div>
                      )}
                      {attentionRecords.length > 8 && (
                        <p className="panel-bottom-note">
                          另有 {attentionRecords.length - 8} 筆，可於年度比較中查看。
                        </p>
                      )}
                    </Panel>
                    <Panel
                      title="資料概況"
                      subtitle="目前工作階段的資料範圍"
                      className="coverage-panel"
                    >
                      <div className="coverage-years">
                        {years.map((value) => (
                          <button
                            className="coverage-year"
                            key={value}
                            onClick={() => {
                              setYear(value);
                              navigate('comparison');
                            }}
                          >
                            <div>
                              <b>
                                {value}
                                <span>年</span>
                              </b>
                              <small>民國 {Number(value) - 1911} 年</small>
                            </div>
                            <div>
                              <strong>
                                {
                                  report.records.filter((record) =>
                                    record.exam_date.startsWith(value),
                                  ).length
                                }
                              </strong>
                              <span>筆結果</span>
                            </div>
                            <Icon name="chevron" size={18} />
                          </button>
                        ))}
                      </div>
                      <div className="coverage-meta">
                        <span>資料取得時間（臺灣時間）</span>
                        <b>{formatTime(report.fetched_at)}</b>
                        <span>
                          {persisted
                            ? '健康存摺已驗證登入 · 設定同步中'
                            : isDemo
                              ? '合成示範資料 · 不屬於真實個人'
                              : '匯入資料 · 設定僅供本次使用'}
                        </span>
                      </div>
                      <button
                        className="button secondary full-width"
                        onClick={() => setImportOpen(true)}
                      >
                        <Icon name="upload" size={18} />
                        重新匯入已下載網頁
                      </button>
                    </Panel>
                  </div>
                  <Panel
                    title="關注項目"
                    subtitle="以星號選取需要長期追蹤的檢驗項目"
                    action={
                      <button className="text-button" onClick={() => navigate('comparison')}>
                        管理關注
                        <Icon name="arrow" size={17} />
                      </button>
                    }
                  >
                    {groups.some((group) => settings.favorites.includes(group.configurationKey)) ? (
                      <div className="favorite-grid">
                        {groups
                          .filter((group) => settings.favorites.includes(group.configurationKey))
                          .map((group) => (
                            <button
                              className="favorite-card"
                              key={group.id}
                              onClick={() => openTrend(group.id)}
                            >
                              <span className="favorite-card-title">
                                <Icon name="star" size={17} />
                                <b>{group.name}</b>
                                <Icon name="trend" size={19} />
                              </span>
                              <span className="favorite-value">
                                {group.records[0].result}
                                <small>{group.unit}</small>
                              </span>
                              <span className="favorite-bottom">
                                <LevelBadge level={group.records[0].level} compact />
                                <small>
                                  {dateLabel(group.records[0].exam_date)}
                                  {group.records.filter(
                                    (record) => record.exam_date === group.records[0].exam_date,
                                  ).length > 1
                                    ? ' · 同日多筆'
                                    : ''}
                                </small>
                              </span>
                            </button>
                          ))}
                      </div>
                    ) : (
                      <div className="favorite-empty">
                        <Icon name="star" size={27} />
                        <div>
                          <b>把重要項目放在這裡</b>
                          <p>在比較表按下項目旁的星號，下次就能快速開啟它的長期趨勢。</p>
                        </div>
                        <button className="button secondary" onClick={() => navigate('comparison')}>
                          選擇關注項目
                        </button>
                      </div>
                    )}
                  </Panel>
                  <section className="interpretation-note">
                    <Icon name="info" />
                    <div>
                      <b>理解顏色，也保留原始脈絡</b>
                      <p>
                        分級依適用來源及個人設定顯示；未具足夠依據的項目保留「未判定」。原院高低標記不因個人基準調整而消失。
                      </p>
                      <Legend />
                    </div>
                  </section>
                </>
              )}

              {view === 'comparison' && (
                <Panel
                  title="歷年檢驗數值"
                  subtitle={`${filteredGroups.length} 個項目 · ${filteredGroups.reduce((sum, group) => sum + group.records.length, 0)} 筆結果 · 日期為列，檢驗項目為欄`}
                  action={
                    <div className="segmented-control" aria-label="日期列分組方式">
                      <button
                        aria-pressed={granularity === 'year'}
                        className={granularity === 'year' ? 'selected' : ''}
                        onClick={() => setGranularity('year')}
                      >
                        年度分組
                      </button>
                      <button
                        aria-pressed={granularity === 'date'}
                        className={granularity === 'date' ? 'selected' : ''}
                        onClick={() => setGranularity('date')}
                      >
                        日期清單
                      </button>
                    </div>
                  }
                  className="comparison-panel"
                >
                  {toolbar}
                  <div className="table-meta">
                    <Legend />
                    <span>↑／↓ 為參考區間比較；「原院異常」保留官網標記</span>
                  </div>
                  {filteredGroups.length ? (
                    <ComparisonTable
                      groups={filteredGroups}
                      favorites={settings.favorites}
                      granularity={granularity}
                      saving={settingsBusy}
                      configurableKeys={configurableKeys}
                      onFavorite={toggleFavorite}
                      onDetail={setDetail}
                      onTrend={openTrend}
                      onConfigure={setBaseline}
                    />
                  ) : (
                    <div className="quiet-empty">
                      <Icon name="search" size={30} />
                      <h3>沒有符合條件的檢驗項目</h3>
                      <button className="button secondary" onClick={resetFilters}>
                        清除篩選條件
                      </button>
                    </div>
                  )}
                  <div className="table-footer">
                    <span>「—」表示未取得資料；不同日期、同日多筆結果皆分開保留。</span>
                    <Copyright />
                    <span id="comparison-print-tip">
                      列印時每段最多 4 個檢驗項目欄，分段換頁並重複日期與欄名；可先篩選類別或關注項目。
                    </span>
                  </div>
                </Panel>
              )}

              {view === 'trends' && (
                <Panel
                  title="單項數值趨勢"
                  subtitle="同一項目與單位，按實際時間比較"
                  action={
                    selectedGroup &&
                    configurableKeys.includes(selectedGroup.configurationKey) && (
                      <button
                        className="button secondary"
                        onClick={() => setBaseline(selectedGroup)}
                      >
                        <Icon name="settings" size={18} />
                        設定此項基準
                      </button>
                    )
                  }
                >
                  <div className="trend-selectors">
                    <label className="field">
                      檢驗項目
                      <select
                        value={selectedGroup?.id ?? ''}
                        onChange={(event) => setSelectedGroupId(event.target.value)}
                      >
                        {categories.map((value) => (
                          <optgroup key={value} label={categoryLabel(value)}>
                            {groups
                              .filter((group) => group.category === value)
                              .map((group) => (
                                <option key={group.id} value={group.id}>
                                  {group.name} · {group.unit}
                                </option>
                              ))}
                          </optgroup>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      顯示年度
                      <select value={year} onChange={(event) => setYear(event.target.value)}>
                        <option value="all">全部年度</option>
                        {years.map((value) => (
                          <option key={value} value={value}>
                            {yearLabel(value)}
                          </option>
                        ))}
                      </select>
                    </label>
                  </div>
                  {trendGroup?.records.length ? (
                    <>
                      <div className="trend-item-heading">
                        <div>
                          <span>{categoryLabel(trendGroup.category)}</span>
                          <h3>{trendGroup.name}</h3>
                          <p>
                            {trendGroup.abbreviation} · {trendGroup.unit}
                          </p>
                        </div>
                        <button
                          className={`button ${settings.favorites.includes(trendGroup.configurationKey) ? 'favorite-active' : 'secondary'}`}
                          onClick={() => toggleFavorite(trendGroup.configurationKey)}
                          disabled={
                            settingsBusy || !configurableKeys.includes(trendGroup.configurationKey)
                          }
                        >
                          <Icon name="star" size={18} />
                          {settings.favorites.includes(trendGroup.configurationKey)
                            ? '已加入關注'
                            : '加入關注'}
                        </button>
                      </div>
                      <TrendChart
                        key={trendGroup.id}
                        group={trendGroup}
                        override={settings.overrides[trendGroup.configurationKey]}
                        onDetail={setDetail}
                      />
                    </>
                  ) : (
                    <div className="quiet-empty">
                      <Icon name="calendar" size={30} />
                      <h3>選擇的年度沒有此項檢驗資料</h3>
                      <button className="button secondary" onClick={() => setYear('all')}>
                        查看全部年度
                      </button>
                    </div>
                  )}
                </Panel>
              )}
            </>
          )}

          {!booting && view === 'settings' && (
            <>
              <div className="settings-columns">
                <Panel title="閱讀與外觀" subtitle="依使用裝置與閱讀習慣調整">
                  <div className="setting-block">
                    <h3>顯示模式</h3>
                    <div className="theme-choices">
                      {(
                        [
                          { value: 'light', label: '淺色', icon: 'sun' },
                          { value: 'dark', label: '深色', icon: 'moon' },
                          { value: 'system', label: '跟隨系統', icon: 'monitor' },
                        ] as { value: Theme; label: string; icon: IconName }[]
                      ).map((option) => (
                        <button
                          className={`theme-choice ${settings.theme === option.value ? 'selected' : ''}`}
                          key={option.value}
                          onClick={() => changeAppearance({ theme: option.value })}
                          aria-pressed={settings.theme === option.value}
                          disabled={settingsBusy}
                        >
                          <Icon name={option.icon} size={24} />
                          <span>{option.label}</span>
                          {settings.theme === option.value && <Icon name="check" size={15} />}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="setting-block">
                    <h3>閱讀字級</h3>
                    <div className="font-choices">
                      {[
                        { scale: 1, label: '標準' },
                        { scale: 1.1, label: '較大' },
                        { scale: 1.2, label: '加大' },
                        { scale: 1.5, label: '特大（150%）' },
                        { scale: 2, label: '最大（200%）' },
                      ].map((option) => (
                        <button
                          className={settings.font_scale === option.scale ? 'selected' : ''}
                          key={option.scale}
                          style={{ fontSize: `${option.scale}rem` }}
                          onClick={() => changeAppearance({ font_scale: option.scale })}
                          aria-pressed={settings.font_scale === option.scale}
                          disabled={settingsBusy}
                        >
                          Aa <span>{option.label}</span>
                        </button>
                      ))}
                    </div>
                  </div>
                </Panel>
                <Panel title="資料與同步" subtitle="設定與檢驗結果分別處理">
                  <dl className="storage-description">
                    <div>
                      <dt>個人基準、關注與外觀</dt>
                      <dd>
                        {persisted
                          ? '已透過官網身分驗證，設定會儲存並跨裝置同步。'
                          : '尚未透過官網身分驗證，設定僅供目前使用階段。'}
                      </dd>
                    </div>
                    <div>
                      <dt>檢驗結果</dt>
                      <dd>只在目前使用階段保留；登出或使用階段結束後清除。</dd>
                    </div>
                    <div>
                      <dt>登入欄位</dt>
                      <dd>證號與健保卡號依您的選擇記在這部裝置；密碼由瀏覽器密碼管理器處理。</dd>
                    </div>
                  </dl>
                  <button
                    className="button secondary"
                    onClick={() => {
                      clearRememberedLogin();
                      setMessage('這部裝置記住的證號與健保卡號已清除。');
                    }}
                  >
                    <Icon name="trash" size={18} />
                    清除本機記住的登入欄位
                  </button>
                </Panel>
              </div>
              <Panel
                title="個人檢驗基準"
                subtitle={`內建 ${builtinCount} 組，合計 ${configuredGroups.length} 組設定。未匯入資料也可查看與調整；新項目會自動補入。`}
                action={
                  <button
                    className="button secondary"
                    disabled={settingsBusy || customBaselineCount === 0}
                    onClick={() => {
                      setRestoreBaselinesError('');
                      setRestoreBaselinesOpen(true);
                    }}
                  >
                    還原全部預設基準
                    {customBaselineCount > 0 && <span>（{customBaselineCount} 項）</span>}
                  </button>
                }
              >
                <div className="settings-persistence">
                  <Icon name={persisted ? 'check' : 'info'} size={18} />
                  {persisted
                    ? '設定已連結已驗證的登入身分，手機與電腦共用同一套設定。'
                    : '目前設定尚未跨裝置保存。透過健康存摺登入後，可儲存個人設定。'}
                </div>
                {toolbar}
                {visibleConfigurationGroups.length ? (
                  <div className="configuration-list">
                    {visibleConfigurationGroups.map((group) => {
                      const item = catalogByKey.get(group.configurationKey);
                      const rule = rulePreview(item?.default_rule);
                      const template = item?.default_reference;
                      const bounds = [...new Set(rule.bands.flatMap((band) => [band.lower, band.upper]))]
                        .filter((value): value is string => value !== null);
                      const override = settings.overrides[group.configurationKey];
                      return (
                        <div className="configuration-row" key={group.configurationKey}>
                          <div className="configuration-name">
                            <span>{categoryLabel(group.category)} · {item?.builtin ? '內建項目' : '新增項目'}</span>
                            <b>{group.name}</b>
                            <small>
                              {group.abbreviation} · {group.unit}
                              {!!item?.order_codes.length && ` · ${item.order_codes.join('／')}`}
                            </small>
                            <div
                              className={`configuration-status ${override?.enabled ? 'customized' : ''}`}
                            >
                              {override?.enabled ? '使用個人設定' : override ? '個人設定已停用，使用預設規則' : '使用預設規則'}
                            </div>
                          </div>
                          <div className="configuration-reference">
                            <span>
                              {rule.bands.length ? '預設來源分類' : template ? '內建參考範本' : '預設判讀'}
                            </span>
                            <b>
                              {rule.bands.length
                                ? `${rule.bands.length} 個分級區間`
                                : template
                                  ? referenceTemplateLabel(template)
                                  : rule.kind === 'qualitative_reference'
                                    ? '定性／半定量標記'
                                    : rule.kind === 'unclassified'
                                      ? '保留原始結果，不強制分級'
                                      : '依各筆原院參考值'}
                            </b>
                            {rule.bands.length > 0 && <small>分類界限：{bounds.join('、')} {rule.unit || group.unit}</small>}
                            {!rule.bands.length && template && !template.automatic_fallback_allowed && (
                              <small>參考範本，須確認適用條件；不自動取代原院基準。</small>
                            )}
                            {!!item?.alert_rules.length && <small>另有 {item.alert_rules.length} 組獨立警示</small>}
                            {override?.enabled && (
                              <small>
                                <strong>目前個人設定：</strong>
                                {override.bands?.length
                                  ? `${override.bands.length} 個自訂分級區間`
                                  : referenceLabel({
                                      reference_low: override.reference_low ?? null,
                                      reference_high: override.reference_high ?? null,
                                      reference_text: '',
                                    })}
                              </small>
                            )}
                            <small>
                              {group.records[0]
                                ? `最近原院參考值：${referenceLabel(group.records[0])}`
                                : '本次尚無檢驗結果'}
                            </small>
                          </div>
                          <div className="configuration-actions">
                            <button
                              className="button secondary"
                              onClick={() => setBaseline(group)}
                              disabled={settingsBusy}
                              aria-label={`檢視與編輯${group.name}基準`}
                            >
                              <Icon name="settings" size={17} />
                              檢視／編輯
                            </button>
                            {override && (
                              <button
                                className="button quiet"
                                onClick={() => void saveOverride(group.configurationKey, null)
                                  .catch((error) => setError(readableError(error)))}
                                disabled={settingsBusy}
                                aria-label={`還原${group.name}預設規則`}
                              >
                                還原預設
                              </button>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <div className="quiet-empty">
                    <Icon name="settings" size={30} />
                    <h3>
                      {configuredGroups.length
                        ? '沒有符合條件的項目'
                        : '內建項目目錄尚未載入'}
                    </h3>
                    <p>
                      {configuredGroups.length
                        ? '相同項目與單位沿用原設定，重新匯入不會重設您已調整的基準值。'
                        : '請查看上方錯誤訊息並重新載入網頁；顯示內建項目不需要先匯入資料。'}
                    </p>
                    {configuredGroups.length ? (
                      <button className="button secondary" onClick={resetFilters}>
                        清除篩選
                      </button>
                    ) : null}
                  </div>
                )}
              </Panel>
            </>
          )}

          <div className="print-only print-running-footer">
            <Copyright />
          </div>
          <footer className="page-footer">
            <div>
              <span>健康映覽</span>
              <span>檢驗資料整理與長期追蹤</span>
            </div>
            <Copyright />
            <span className="footer-note">顏色為整理提示，請結合完整病史與專業判斷。</span>
          </footer>
        </main>
      </div>

      <nav className="mobile-nav" aria-label="行動版主要導覽">
        {NAV.map((item) => (
          <button
            key={item.id}
            onClick={() => navigate(item.id)}
            className={view === item.id ? 'active' : ''}
            aria-current={view === item.id ? 'page' : undefined}
          >
            <Icon name={item.icon} size={21} />
            <span>{item.short}</span>
          </button>
        ))}
      </nav>
      {loginOpen && <LoginDialog onClose={() => setLoginOpen(false)} onLogin={loggedIn} />}
      {importOpen && <ImportDialog onClose={() => setImportOpen(false)} onImport={importFile} />}
      {detail && (
        <DetailDialog
          record={detail}
          catalog={catalog}
          canConfigure={configurableKeys.includes(detail.configuration_key)}
          onClose={() => setDetail(null)}
          onTrend={() => {
            const group = groups.find((group) =>
              group.records.some((record) => record.id === detail.id),
            );
            if (group) openTrend(group.id);
          }}
          onConfigure={() => {
            const group = groups.find((group) =>
              group.records.some((record) => record.id === detail.id),
            );
            setDetail(null);
            if (group) setBaseline(group);
          }}
        />
      )}
      {baseline && (
        <BaselineDialog
          group={baseline}
          catalog={catalog}
          override={settings.overrides[baseline.configurationKey]}
          onSave={saveOverride}
          onClose={() => setBaseline(null)}
          persisted={persisted}
        />
      )}
      {restoreBaselinesOpen && (
        <Modal
          title="還原全部預設基準"
          subtitle={`將清除 ${customBaselineCount} 項個人基準調整`}
          onClose={() => setRestoreBaselinesOpen(false)}
          busy={settingsBusy}
        >
          <p className="helper-text">
            所有項目會重新使用內建分類、適用的獨立警示及各筆原院參考值。已匯入的檢驗結果、關注項目、顯示模式與閱讀字級均會保留。
          </p>
          {restoreBaselinesError && <Alert tone="error">{restoreBaselinesError}</Alert>}
          <div className="dialog-actions">
            <button className="button secondary" onClick={() => setRestoreBaselinesOpen(false)} disabled={settingsBusy}>
              保留個人基準
            </button>
            <button className="button primary" onClick={() => void restoreAllBaselines()} disabled={settingsBusy || customBaselineCount === 0}>
              {settingsBusy ? <Loading label="還原中…" /> : '還原全部基準'}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
