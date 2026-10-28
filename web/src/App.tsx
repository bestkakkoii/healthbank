// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  api,
  ApiError,
  defaultSettings,
  isSessionInvalidation,
  readableError,
  setCsrfToken,
  type CatalogResponse,
  type DataFieldTerm,
  type ItemOverride,
  type LabRecord,
  type LiveReportResponse,
  type ReportQueryMode,
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
import { ClinicalComparison } from './ClinicalComparison';
import { AnalysisExport, MultiIndicator, LipidCalculation, PortableReportExport } from './AnalysisTools';
import { PersonalRecords } from './PersonalRecords';
import { SupplementaryData, CoverageSummary } from './SupplementaryData';
import { DatasetTools } from './DatasetTools';
import { DeviceHealthCache } from './DeviceHealthCache';
import { useDeviceHealthCache } from './use-device-health-cache';
import { inspectDeviceCache, sameDeviceCacheLease, verifiedCacheProfile, type DeviceCacheLease, type DeviceCacheMetadata } from './device-health-cache';
import { cachedSourceSnapshot } from './cache-provenance';
import { acquireHealthData } from './data-acquisition';
import { createTerminologyIndex, groupTerminology, groupTerminologySearchText, recordTerminology } from './terminology-display';
import { GradingContextLabel, GradingContextNotes } from './GradingContextNotes';
import type { CollectionRestoration } from './collection-restoration';
import { PrintReport } from './PrintReport';
import { NativePrintReport } from './NativePrintReport';
import { laboratoryCoverageStatus, laboratoryDateSummary } from './coverage-status';
import type { ImportedDataset, LaboratoryMetadata, DatasetProducer } from './health-dataset';
import { emptyHealthData, type HealthDataSnapshot, type DownloadedAttachment, type DataCategory } from './health-data';
import { AnalysisProvider, AnalysisStamp } from './AnalysisContext';
import { PrivacyNotice } from './PrivacyNotice';
import {
  categoryLabel,
  dateLabel,
  downloadText,
  formatTime,
  groupRecords,
  isObject,
  referenceLabel,
  referenceTemplateLabel,
  rulePreview,
  toCsv,
  yearLabel,
  type MeasurementGroup,
} from './model';
import { Alert, Copyright, Icon, Legend, LevelBadge, Loading, Modal, type IconName } from './ui';

type View = 'overview' | 'data' | 'comparison' | 'trends' | 'settings';
const NAV: { id: View; label: string; short: string; icon: IconName; eyebrow: string }[] = [
  {
    id: 'overview',
    label: '檢驗總覽',
    short: '總覽',
    icon: 'overview',
    eyebrow: 'YOUR HEALTH, IN PERSPECTIVE',
  },
  {
    id: 'data',
    label: '醫療紀錄',
    short: '紀錄',
    icon: 'file',
    eyebrow: 'YOUR RECORDS, YOUR PERSPECTIVE',
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
const sessionKey = (value: Session) => `${value.mode}:${value.csrf_token ?? ''}`;
const SESSION_CHANGED_MESSAGE = '使用階段已到期或更換，先前報告與醫療資料已清除。請確認目前使用者後重新取得資料。';
function viewFromURL(): View {
  const requested = new URL(window.location.href).searchParams.get('view');
  return NAV.find(item => item.id === requested)?.id ?? 'overview';
}

function initialSettings(): Settings {
  const settings = defaultSettings();
  try {
    const raw = localStorage.getItem(APPEARANCE_KEY);
    if (raw) {
      const value: unknown = JSON.parse(raw);
      if (isObject(value)) {
        const theme = value['theme'];
        const scale = value['font_scale'];
        if (theme === 'light' || theme === 'dark' || theme === 'system') settings.theme = theme;
        if (typeof scale === 'number' && [1, 1.1, 1.2, 1.5, 2].includes(scale)) settings.font_scale = scale;
      }
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
  const [dataCatalog, setDataCatalog] = useState<DataCategory[]>([]);
  const [dataFieldTerms, setDataFieldTerms] = useState<DataFieldTerm[]>([]);
  const [dataCatalogError, setDataCatalogError] = useState('');
  const [dataCatalogLoading, setDataCatalogLoading] = useState(false);
  const [dataSnapshot, setDataSnapshot] = useState<HealthDataSnapshot>(emptyHealthData);
  const [dataAttachments, setDataAttachments] = useState<DownloadedAttachment[]>([]);
  const dataAttachmentsRef = useRef<DownloadedAttachment[]>([]);
  const [importedDataCatalog, setImportedDataCatalog] = useState<DataCategory[]>([]);
  const [importedLaboratoryMetadata, setImportedLaboratoryMetadata] = useState<LaboratoryMetadata | null>(null);
  const [importedDatasetProducer, setImportedDatasetProducer] = useState<DatasetProducer | null>(null);
  const [restorationContext, setRestorationContext] = useState<CollectionRestoration | null>(null);
  const [cacheWorkspace, setCacheWorkspace] = useState(false);
  const [cacheLoginRequest, setCacheLoginRequest] = useState(0);
  const [datasetVersion, setDatasetVersion] = useState(0);
  const datasetVersionRef = useRef(0);
  const sessionIdentity = useRef('anonymous:');
  const [dataBusy, setDataBusy] = useState(false);
  const [dataBatchActive, setDataBatchActive] = useState(false);
  const [view, setView] = useState<View>(viewFromURL);
  const [booting, setBooting] = useState(true);
  const [busy, setBusy] = useState('');
  const [settingsBusy, setSettingsBusy] = useState(false);
  const settingsLock = useRef(false);
  const sessionGeneration = useRef(0);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [assessmentStale, setAssessmentStale] = useState(false);
  const [loginOpen, setLoginOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [printOpen, setPrintOpen] = useState(false);
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
  const workBusy = booting || settingsBusy || dataBusy || dataBatchActive || busy !== '';
  const workspaceCatalog = useMemo(() => {
    const combined = new Map(dataCatalog.map(item => [item.id, item]));
    for (const item of importedDataCatalog) if (!combined.has(item.id)) combined.set(item.id, item);
    for (const id of Object.keys(dataSnapshot.reports)) {
      if (!combined.has(id)) combined.set(id, { id, label: id, group: '資料包其他來源', range_mode: 'source' });
    }
    return [...combined.values()];
  }, [dataCatalog, importedDataCatalog, dataSnapshot.reports]);
  const archivedAttachments = useMemo(() => Object.fromEntries(dataAttachments.map(attachment =>
    [`${attachment.category}:${attachment.id}`, { name: attachment.name, content_type: attachment.content_type, bytes: attachment.bytes }])), [dataAttachments]);
  const datasetExportReport = importedLaboratoryMetadata
    ? { ...report, ...importedLaboratoryMetadata, records: report.records }
    : report;
  const datasetProducer = useMemo<DatasetProducer>(() => ({
    application: 'Healthbank', version: session.version || 'unknown', connector_revision: 'nhi-spa.2026-10-28.3',
    clinical_catalog_id: catalog.provenance?.catalog_id ?? 'unknown',
    clinical_catalog_revision: catalog.provenance?.catalog_revision ?? 'unknown',
    clinical_evidence_revision: catalog.provenance?.evidence_revision ?? 'unknown',
  }), [session.version, catalog.provenance]);
  const deviceCache = useDeviceHealthCache({ session, ready: !booting, blocked: workBusy, cacheView: cacheWorkspace,
    loginRequest: cacheLoginRequest, report: datasetExportReport, snapshot: dataSnapshot, attachments: dataAttachments,
    categories: workspaceCatalog, producer: datasetProducer, restore: restoreCachedWorkspace, clearWorkspace: clearCachedWorkspace,
    initialFetch: signal => fetchReports('all_history', signal), refresh: refreshAllWorkspace });
  const controlsBusy = workBusy || deviceCache.busy;

  useEffect(() => {
    const restoreView = () => {
      setView(viewFromURL());
      setRangeOpen(false);
      window.requestAnimationFrame(() => mainHeading.current?.focus({ preventScroll: true }));
    };
    window.addEventListener('popstate', restoreView);
    return () => window.removeEventListener('popstate', restoreView);
  }, []);

  useEffect(() => {
    const preparePrint = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'p' && !controlsBusy &&
        (report.records.length || Object.keys(dataSnapshot.reports).length)) {
        event.preventDefault(); setPrintOpen(true);
      }
    };
    window.addEventListener('keydown', preparePrint);
    return () => window.removeEventListener('keydown', preparePrint);
  }, [controlsBusy, report.records.length, dataSnapshot.reports]);

  useEffect(() => {
    const controller = new AbortController();
    async function initialize() {
      try {
        // Establish the session cookie before issuing other session-bearing requests.
        const fetched = await api.session(controller.signal);
        controller.signal.throwIfAborted();
        sessionIdentity.current = `${fetched.mode}:${fetched.csrf_token ?? ''}`;
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
          api.dataCatalog(controller.signal),
        ]);
        controller.signal.throwIfAborted();
        // Initial parallel reads may straddle expiry or another tab's login.
        // Even an anonymous catalog can become session-derived during that gap.
        const verified = await api.session(controller.signal);
        controller.signal.throwIfAborted();
        if (sessionKey(verified) !== sessionKey(fetched) ||
          results[0].status === 'rejected' && isSessionInvalidation(results[0].reason) ||
          results[1].status === 'rejected' && isSessionInvalidation(results[1].reason) ||
          fetched.mode !== 'anonymous' && results[1].status === 'fulfilled' && results[1].value.session_token !== verified.csrf_token) {
          sessionIdentity.current = sessionKey(verified);
          setCsrfToken(verified.csrf_token);
          setSession(verified.mode === 'official' ? verified : { ...verified,
            settings: { ...verified.settings, theme: initialSettings().theme, font_scale: initialSettings().font_scale } });
          setError(SESSION_CHANGED_MESSAGE);
          if (results[2].status === 'fulfilled') { setDataCatalog(results[2].value.categories); setDataFieldTerms(results[2].value.field_terms ?? []); }
          else setDataCatalogError(`醫療資料來源目錄無法載入。${readableError(results[2].reason)}`);
          return;
        }
        if (results[0].status === 'fulfilled') setCatalog(results[0].value);
        else setError(`參考項目目錄暫時無法載入。${readableError(results[0].reason)}`);
        if (results[1].status === 'fulfilled') {
          const current = results[1].value;
          // A retained server copy of a device cache is not redisplayed before
          // the current device consent/lease has been checked again.
          setReport(current.cache_provenance ? EMPTY_REPORT : current);
          if (current.official_query) {
            setDateFrom(current.official_query.start_date);
            setDateTo(current.official_query.end_date);
          }
        } else setError(readableError(results[1].reason));
        if (results[2].status === 'fulfilled') { setDataCatalog(results[2].value.categories); setDataFieldTerms(results[2].value.field_terms ?? []); }
        else setDataCatalogError(`醫療資料來源目錄無法載入。${readableError(results[2].reason)}`);
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
      document.documentElement.dataset['theme'] = activeTheme;
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
  const terminology = useMemo(() => createTerminologyIndex(catalog), [catalog]);
  const groupNames = useMemo(() => new Map(groups.map(group => [group.id, groupTerminology(group, terminology)])), [groups, terminology]);
  const years = useMemo(
    () =>
      [...new Set(report.records.map((record) => record.exam_date.slice(0, 4)).filter(Boolean))]
        .sort()
        .reverse(),
    [report.records],
  );
  const datedGroups = useMemo(() => groups.map((group) => ({
        ...group,
        records: group.records.filter(
          (record) => year === 'all' || record.exam_date.startsWith(`${year}-`),
        ),
      })), [groups, year]);
  const groupSearch = useMemo(() => new Map(datedGroups.map(group => [group.id, groupTerminologySearchText(group, terminology)])), [datedGroups, terminology]);
  const filteredGroups = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return datedGroups.filter(
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
            groupSearch.get(group.id)?.includes(normalized)),
      );
  }, [datedGroups, category, onlyFavorites, onlyAbnormal, settings.favorites, query, groupSearch]);
  const selectedGroup = groups.find((group) => group.id === selectedGroupId) ?? groups[0];
  const selectedExportRecords = useMemo(() => view === 'trends' && selectedGroup
    ? selectedGroup.records.filter(record => year === 'all' || record.exam_date.startsWith(`${year}-`))
    : filteredGroups.flatMap(group => group.records), [view, selectedGroup, year, filteredGroups]);
  const trendGroup = selectedGroup
    ? {
        ...selectedGroup,
        records: selectedGroup.records.filter(
          (record) => year === 'all' || record.exam_date.startsWith(`${year}-`),
        ),
      }
    : undefined;
  const uniqueDates = [...new Set(report.records.map((record) => record.exam_date).filter(Boolean))]
    .sort()
    .reverse();
  const unknownDateCount = report.records.filter(record => !record.exam_date).length;
  const latestRecords = groups.flatMap((group) => {
    const latest = group.records[0];
    return latest ? group.records.filter((record) => record.exam_date === latest.exam_date) : [];
  });
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
  const configurationSearch = useMemo(() => new Map(configuredGroups.map(group => [group.id,
    `${groupTerminologySearchText(group, terminology)} ${catalogByKey.get(group.configurationKey)?.aliases.join(' ') ?? ''} ${catalogByKey.get(group.configurationKey)?.order_codes.join(' ') ?? ''}`.toLocaleLowerCase(),
  ])), [configuredGroups, terminology, catalogByKey]);
  const visibleConfigurationGroups = configuredGroups.filter(
    (group) =>
      (category === 'all' || group.category === category) &&
      configurationSearch.get(group.id)?.includes(query.trim().toLocaleLowerCase()),
  );
  const nav = NAV.find((item) => item.id === view);
  if (!nav) throw new Error('Unknown application view.');

  function navigate(next: View) {
    if (next !== view) {
      const url = new URL(window.location.href);
      url.searchParams.set('view', next);
      url.hash = '';
      window.history.pushState(null, '', url);
    }
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

  function invalidateDatasetOperations() {
    setPrintOpen(false);
    setDataBatchActive(false);
    // Retire captured archive callbacks before any session-changing request can
    // await. Keep successful data available if that request subsequently fails.
    datasetVersionRef.current += 1;
    setDatasetVersion(datasetVersionRef.current);
  }

  function clearMedicalDataset() {
    setPrintOpen(false);
    invalidateDatasetOperations();
    setDataSnapshot(emptyHealthData());
    dataAttachmentsRef.current = [];
    setDataAttachments([]);
    setImportedDataCatalog([]);
    setImportedLaboratoryMetadata(null);
    setImportedDatasetProducer(null);
    setRestorationContext(null);
    setCacheWorkspace(false);
    setDataBusy(false);
  }

  function clearSessionRecords() {
    clearMedicalDataset();
    setReport(EMPTY_REPORT);
    setDetail(null);
    setBaseline(null);
    clearSessionCatalog();
    clearReportRange();
    resetFilters();
    setAssessmentStale(false);
  }

  function installSession(next: Session) {
    sessionIdentity.current = sessionKey(next);
    setCsrfToken(next.csrf_token);
    setSession(next.mode === 'official' ? next : {
      ...next,
      settings: { ...next.settings, theme: initialSettings().theme, font_scale: initialSettings().font_scale },
    });
  }

  function reconcileObservedSession(next: Session) {
    // The login dialog refreshes CAPTCHA sessions too. It must not replace the
    // shared request token while leaving this page's medical identity unchanged.
    if (sessionKey(next) !== sessionIdentity.current) {
      sessionGeneration.current += 1;
      clearSessionRecords();
      setBusy('');
      setError(SESSION_CHANGED_MESSAGE);
    }
    installSession(next);
  }

  function enterUnverifiedMode(mode: 'import' | 'demo', token: string) {
    // A successful import/demo request has already ended official auth on the
    // server. A failed follow-up session GET must not leave the UI claiming it.
    sessionIdentity.current = `${mode}:${token}`;
    setCsrfToken(token);
    setSession(current => ({ authenticated: false, mode, csrf_token: token, version: current.version,
      settings: { ...defaultSettings(), theme: current.settings.theme, font_scale: current.settings.font_scale } }));
  }

  function openLogin() {
    invalidateDatasetOperations();
    setLoginOpen(true);
  }

  function openImport() {
    invalidateDatasetOperations();
    setImportOpen(true);
  }

  async function reloadDataCatalog() {
    setDataCatalogLoading(true); setDataCatalogError('');
    try { const next = await api.dataCatalog(); setDataCatalog(next.categories); setDataFieldTerms(next.field_terms ?? []); }
    catch (failure) {
      if (isSessionInvalidation(failure)) await expireOfficialSession();
      setDataCatalogError(`醫療資料來源目錄無法載入。${readableError(failure)}`);
    }
    finally { setDataCatalogLoading(false); }
  }

  function retainAttachment(attachment: DownloadedAttachment) {
    if (datasetVersion !== datasetVersionRef.current) throw new Error('使用階段已變更，這份附件不會加入新的使用者資料。');
    const others = dataAttachmentsRef.current.filter(value => value.category !== attachment.category || value.id !== attachment.id);
    if (others.reduce((sum, value) => sum + value.bytes.byteLength, 0) + attachment.bytes.byteLength > 32 * 1024 * 1024)
      throw new Error('資料包內的附件合計已達 32 MiB 上限；請使用「另存原始檔」下載這份附件。');
    const next = [...others, { ...attachment, bytes: new Uint8Array(attachment.bytes) }];
    dataAttachmentsRef.current = next;
    setDataAttachments(next);
  }

  async function fetchLaboratoryForWorkspace(signal: AbortSignal, query = { from: '', to: '' }): Promise<ReportResponse> {
    const generation = sessionGeneration.current;
    const result = await api.reports(query.from, query.to, signal);
    signal.throwIfAborted();
    if (generation !== sessionGeneration.current) throw new DOMException('使用階段已變更。', 'AbortError');
    await receiveReports(result, generation, undefined, signal);
    signal.throwIfAborted();
    if (generation !== sessionGeneration.current) throw new DOMException('使用階段已變更。', 'AbortError');
    if (result.official_query) {
      setDateFrom(result.official_query.start_date); setDateTo(result.official_query.end_date);
    }
    return result;
  }

  async function restoreCachedWorkspace(dataset: ImportedDataset, metadata: DeviceCacheMetadata, lease: DeviceCacheLease, signal: AbortSignal) {
    const generation = sessionGeneration.current, expectedIdentity = sessionIdentity.current;
    const current = await api.session(signal); signal.throwIfAborted();
    if (generation !== sessionGeneration.current || sessionKey(current) !== expectedIdentity || verifiedCacheProfile(current) !== lease.profile_key)
      throw new DOMException('目前驗證身分已變更，沒有開啟先前使用者的快取。', 'AbortError');
    if (typeof current.report_revision !== 'number' || !Number.isSafeInteger(current.report_revision) || current.report_revision < 0)
      throw new Error('目前伺服器尚未提供快取還原版本防護，請更新伺服器後再開啟本機快取。');
    const existing = await api.currentReports(signal); signal.throwIfAborted();
    if (existing.session_token !== current.csrf_token) throw new Error(SESSION_CHANGED_MESSAGE);
    const original = dataset.laboratoryMetadata.cache_provenance;
    const sourceQuery = original?.source_query ?? dataset.laboratoryMetadata.official_query;
    // Reloading a page must not replace a newer in-memory official laboratory
    // read with an older local snapshot. Its source snapshots remain labelled.
    const result = existing.fetched_at && !existing.cache_provenance ? existing : await api.restoreCachedReports({
      expected_profile_key: lease.profile_key, expected_report_revision: current.report_revision,
      saved_at: metadata.saved_at, source_fetched_at: dataset.laboratoryMetadata.fetched_at,
      source_complete: original?.source_complete ?? dataset.laboratoryMetadata.complete,
      ...(sourceQuery === undefined ? {} : { source_query: sourceQuery }), document: JSON.parse(dataset.laboratoryJSON) as unknown,
    }, signal);
    const [verified, nextCatalog, consent] = await Promise.all([api.session(signal), api.catalog(signal), inspectDeviceCache(lease.profile_key)]);
    signal.throwIfAborted();
    if (generation !== sessionGeneration.current || sessionKey(verified) !== expectedIdentity || verifiedCacheProfile(verified) !== lease.profile_key || result.session_token !== verified.csrf_token)
      throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError');
    if (!consent.lease || !sameDeviceCacheLease(lease, consent.lease))
      throw new Error('快取在還原期間已更新或清除，沒有把舊資料放回工作區。');
    invalidateDatasetOperations();
    installSession(verified); setCatalog(nextCatalog); setReport(result); setAssessmentStale(false); resetFilters();
    const source = result.cache_provenance?.source_query ?? result.official_query;
    setDateFrom(source?.start_date ?? ''); setDateTo(source?.end_date ?? '');
    setDataSnapshot(cachedSourceSnapshot(dataset.snapshot, metadata.saved_at));
    dataAttachmentsRef.current = dataset.attachments; setDataAttachments(dataset.attachments);
    setImportedDataCatalog(dataset.catalog); setImportedDatasetProducer(dataset.producer); setImportedLaboratoryMetadata(null);
    setRestorationContext(null); setCacheWorkspace(true);
  }

  async function clearCachedWorkspace(profileKey: string, scope: 'cache_only' | 'all_reports', signal: AbortSignal) {
    if (verifiedCacheProfile(session) !== profileKey) throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError');
    const clearActive = scope === 'all_reports' || cacheWorkspace || Boolean(report.cache_provenance);
    const generation = clearActive ? ++sessionGeneration.current : sessionGeneration.current;
    if (clearActive) {
      const retained = scope === 'cache_only' && !report.cache_provenance ? report : null;
      clearSessionRecords(); setReport(retained ?? { records: [], fetched_at: '', complete: false, warnings: [] });
    }
    try {
      await api.clearCachedReports(profileKey, scope, signal);
      const next = await api.session(signal); signal.throwIfAborted();
      if (generation !== sessionGeneration.current) throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError');
      if (verifiedCacheProfile(next) !== profileKey || sessionKey(next) !== sessionIdentity.current) {
        await expireOfficialSession(); throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError');
      }
      installSession(next);
    } catch (failure) {
      if (isSessionInvalidation(failure)) await expireOfficialSession();
      throw failure;
    }
  }

  async function refreshAllWorkspace(signal: AbortSignal) {
    const generation = sessionGeneration.current;
    setBusy('正在重新取得全部可讀取來源…'); setError('');
    let snapshot = emptyHealthData();
    try {
      const currentCatalog = await api.dataCatalog(signal); signal.throwIfAborted();
      if (generation !== sessionGeneration.current) throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError');
      setDataCatalog(currentCatalog.categories);
      setDataFieldTerms(currentCatalog.field_terms ?? []);
      try { await fetchLaboratoryForWorkspace(signal); }
      catch (failure) {
        signal.throwIfAborted();
        if (isSessionInvalidation(failure)) throw failure;
        setReport({ records: [], fetched_at: '', complete: false, warnings: [`本輪檢驗尚未取得：${readableError(failure)}`] });
      }
      const acquired = await acquireHealthData({ snapshot, categories: currentCatalog.categories.map(category => category.id),
        query: { from: '', to: '' }, mode: 'refresh', signal,
        fetchReport: (category, reference, query, signal) => api.supplementary(category, query.from, query.to, reference, signal),
        classifyFailure: failure => ({ message: readableError(failure), expired: isSessionInvalidation(failure),
          retryable: failure instanceof TypeError || failure instanceof ApiError && [502, 503, 504].includes(failure.status) }),
        onUpdate: next => { snapshot = next; if (generation === sessionGeneration.current) setDataSnapshot(next); },
        onProgress: progress => { if (generation === sessionGeneration.current) setBusy(`正在取得${progress.phase === 'lists' ? '來源清單' : '明細'}：${progress.done}/${progress.total}；未成功 ${progress.failed}`); },
      });
      if (acquired.expired) { await expireOfficialSession(); throw new DOMException(SESSION_CHANGED_MESSAGE, 'AbortError'); }
      signal.throwIfAborted();
    } catch (failure) {
      if (signal.aborted && generation === sessionGeneration.current) {
        setDataSnapshot({ ...snapshot, states: Object.fromEntries(Object.entries(snapshot.states).map(([id, state]) =>
          [id, state.status === 'loading' ? { ...state, status: 'cancelled' } : state])) });
      } else if (isSessionInvalidation(failure)) await expireOfficialSession();
      throw failure;
    } finally {
      if (generation === sessionGeneration.current) {
        // This acquisition runs outside SupplementaryData's local controller.
        // Install its completed/partial snapshot in a fresh view as well as the
        // export/cache model, and retire callbacks captured by the empty view.
        invalidateDatasetOperations(); setBusy('');
      }
    }
  }

  async function restoreDataset(dataset: ImportedDataset, collection?: CollectionRestoration) {
    if (datasetVersion !== datasetVersionRef.current || controlsBusy)
      throw new Error('使用階段已變更或仍在處理資料，請重新選取資料包。');
    const generation = ++sessionGeneration.current;
    invalidateDatasetOperations();
    setBusy('正在還原完整資料包…');
    try {
      const next = await api.importReports(new File([dataset.laboratoryJSON], 'healthbank-labs.json', { type: 'application/json' }));
      if (generation !== sessionGeneration.current) return;
      clearSessionRecords();
      enterUnverifiedMode('import', next.session_token);
      await receiveReports(next, generation, 'import');
      if (generation !== sessionGeneration.current) return;
      // Keep the server's import-mode laboratory assessment. Historical query
      // metadata belongs to the archive, never to current authentication proof.
      // Session refresh may already have mounted an empty workspace while the
      // catalog was loading. Install the archive and its new mount key together.
      invalidateDatasetOperations();
      setDataSnapshot(dataset.snapshot);
      dataAttachmentsRef.current = dataset.attachments;
      setDataAttachments(dataset.attachments);
      setImportedDataCatalog(dataset.catalog);
      setImportedLaboratoryMetadata(dataset.laboratoryMetadata);
      setImportedDatasetProducer(dataset.producer);
      setRestorationContext(collection ?? null);
      setImportOpen(false); navigate('data');
      setMessage('完整資料包已還原至獨立匯入模式；檔案來源及完整性描述未經本次官方連線驗證。');
    } catch (failure) {
      if (generation !== sessionGeneration.current) return;
      await reconcileFailedSessionMutation(failure, generation);
      throw failure;
    } finally { if (generation === sessionGeneration.current) setBusy(''); }
  }

  async function refreshSession(generation: number, expectedMode?: 'import' | 'demo', signal?: AbortSignal, expectedToken?: string) {
    const next = await api.session(signal);
    signal?.throwIfAborted();
    if (generation !== sessionGeneration.current) return;
    const changed = (expectedMode ? next.mode !== expectedMode : sessionKey(next) !== sessionIdentity.current) ||
      expectedToken !== undefined && next.csrf_token !== expectedToken;
    if (changed) {
      // A cookie can change in another tab while a successful read is in flight.
      // Do not combine that response with a different person's session/settings.
      sessionGeneration.current += 1;
      clearSessionRecords();
      installSession(next);
      setBusy('');
      setError(SESSION_CHANGED_MESSAGE);
      return;
    }
    if (sessionKey(next) !== sessionIdentity.current) clearMedicalDataset();
    installSession(next);
    return next;
  }

  async function receiveReports(next: LiveReportResponse, generation: number, expectedMode?: 'import' | 'demo', signal?: AbortSignal) {
    signal?.throwIfAborted();
    if (generation !== sessionGeneration.current) return;
    const results = await Promise.allSettled([refreshSession(generation, expectedMode, signal, next.session_token), api.catalog(signal)]);
    signal?.throwIfAborted();
    if (generation !== sessionGeneration.current) return;
    if (results.some(result => result.status === 'rejected' && isSessionInvalidation(result.reason))) {
      await expireOfficialSession(); return;
    }
    if (results[0].status === 'rejected') {
      const message = `資料已取得，但使用階段狀態無法更新。${readableError(results[0].reason)}`;
      // Successful explicit import/demo already establishes an isolated local
      // mode. Ordinary reads must await matching-session proof before display.
      if (!expectedMode) throw new Error(message);
      setError(message);
    } else setError('');
    setReport(next);
    if (!next.official_query) clearReportRange();
    setAssessmentStale(false);
    resetFilters();
    if (results[1].status === 'fulfilled') setCatalog(results[1].value);
    else setError(`資料已取得，但參考項目目錄無法更新。${readableError(results[1].reason)}`);
  }

  function loggedIn(next: Session, notice = ''): Promise<void> {
    sessionGeneration.current += 1;
    clearMedicalDataset();
    sessionIdentity.current = `${next.mode}:${next.csrf_token ?? ''}`;
    setSession(next);
    setCsrfToken(next.csrf_token);
    setReport(EMPTY_REPORT);
    setDetail(null);
    setBaseline(null);
    clearSessionCatalog();
    clearReportRange();
    setLoginOpen(false);
    navigate('overview');
    setCacheLoginRequest(current => current + 1);
    if (notice) setMessage(notice);
    return Promise.resolve();
  }

  async function expireOfficialSession() {
    const generation = ++sessionGeneration.current;
    clearSessionRecords();
    sessionIdentity.current = 'anonymous:';
    setCsrfToken();
    setBusy('正在確認新的使用階段…');
    setError(SESSION_CHANGED_MESSAGE);
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
      // Reconcile the browser's current session without logging out a different
      // person who may have connected in another tab after this caller expired.
      const current = await api.session();
      if (generation !== sessionGeneration.current) return;
      installSession(current);
    } catch {
      /* Local report is already removed; the next login renews its session and CAPTCHA. */
    } finally {
      if (generation === sessionGeneration.current) setBusy('');
    }
  }

  async function reconcileFailedSessionMutation(failure: unknown, generation: number) {
    if (generation !== sessionGeneration.current) return;
    if (isSessionInvalidation(failure)) { await expireOfficialSession(); return; }
    try {
      // A request may have committed even when its response was lost. Read the
      // current generation without replaying login/import/logout/deletion.
      await refreshSession(generation);
    } catch {
      if (generation === sessionGeneration.current) await expireOfficialSession();
    }
  }

  async function fetchReports(mode: ReportQueryMode, signal?: AbortSignal) {
    const generation = sessionGeneration.current;
    const from = mode === 'all_history' ? '' : dateFrom;
    const to = mode === 'all_history' ? '' : dateTo;
    if (mode === 'custom' && (!from || !to || from > to)) {
      setError('請選擇有效的開始與結束日期，開始日期不可晚於結束日期。');
      return;
    }
    setBusy(mode === 'all_history'
      ? '正在向健康存摺查詢全部歷史檢驗資料…'
      : '正在向健康存摺查詢指定期間的檢驗資料…');
    setError('');
    setMessage('');
    try {
      const result = await api.reports(from, to, signal);
      signal?.throwIfAborted();
      await receiveReports(result, generation, undefined, signal);
      if (generation !== sessionGeneration.current) return;
      if (result.official_query) {
        setDateFrom(result.official_query.start_date);
        setDateTo(result.official_query.end_date);
      }
      setRangeOpen(false);
      setMessage(`已取得本次查詢回傳的 ${result.records.length} 筆檢驗結果。`);
    } catch (error) {
      if (signal?.aborted) return;
      if (generation !== sessionGeneration.current) return;
      if (isSessionInvalidation(error)) {
        await expireOfficialSession();
      } else setError(readableError(error));
    } finally {
      if (generation === sessionGeneration.current) setBusy('');
    }
  }

  async function importFile(file: File) {
    const generation = ++sessionGeneration.current;
    invalidateDatasetOperations();
    try {
      const next = await api.importReports(file);
      if (generation !== sessionGeneration.current) return;
      clearSessionRecords();
      enterUnverifiedMode('import', next.session_token);
      await receiveReports(next, generation, 'import');
      if (generation !== sessionGeneration.current) return;
      setImportOpen(false);
      navigate('overview');
      setMessage(`已整理 ${next.records.length} 筆檢驗紀錄。`);
    } catch (failure) {
      if (generation !== sessionGeneration.current) return;
      await reconcileFailedSessionMutation(failure, generation);
      throw failure;
    }
  }

  async function openDemo() {
    const generation = ++sessionGeneration.current;
    clearMedicalDataset();
    setBusy('正在載入合成示範資料…');
    setError('');
    try {
      const next = await api.demo();
      if (generation !== sessionGeneration.current) return;
      clearSessionRecords();
      enterUnverifiedMode('demo', next.session_token);
      await receiveReports(next, generation, 'demo');
      if (generation !== sessionGeneration.current) return;
      navigate('overview');
    } catch (error) {
      if (generation === sessionGeneration.current) {
        await reconcileFailedSessionMutation(error, generation);
        if (generation === sessionGeneration.current) setError(readableError(error));
      }
    } finally {
      if (generation === sessionGeneration.current) setBusy('');
    }
  }

  async function logout(deleteRemote = false) {
    const generation = ++sessionGeneration.current;
    invalidateDatasetOperations();
    setBusy('正在結束使用階段…');
    setError('');
    try {
      const next = deleteRemote ? await api.deleteSettings(settings.revision) : await api.logout();
      if (generation !== sessionGeneration.current) return;
      clearMedicalDataset();
      sessionIdentity.current = `${next.mode}:${next.csrf_token ?? ''}`;
      setSession({
        ...next,
        settings: { ...next.settings, theme: settings.theme, font_scale: settings.font_scale },
      });
      setCsrfToken(next.csrf_token);
      setReport(EMPTY_REPORT);
      setDetail(null);
      setBaseline(null);
      clearSessionCatalog();
      clearReportRange();
      resetFilters();
      navigate('overview');
      setMessage(deleteRemote ? '伺服器現行設定已刪除，本站所有裝置工作階段已失效；本機保存及營運者備份須另行管理。' : '已結束使用階段並清除目前報告。');
    } catch (error) {
      if (generation === sessionGeneration.current) {
        await reconcileFailedSessionMutation(error, generation);
        if (generation === sessionGeneration.current) setError(readableError(error));
      }
    } finally {
      if (generation === sessionGeneration.current) setBusy('');
    }
  }

  function clearReportRange() {
    setDateFrom('');
    setDateTo('');
    setRangeOpen(false);
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
    if (booting || dataBusy || busy !== '') throw new Error('資料正在更新，請稍候再修改設定。');
    if (settingsLock.current) throw new Error('上一個設定正在儲存，請稍候再操作。');
    const generation = sessionGeneration.current;
    settingsLock.current = true;
    setSettingsBusy(true);
    try {
      const saved = await api.saveSettings(next);
      if (generation !== sessionGeneration.current) return false;
      setSession((current) => ({ ...current, settings: saved }));
      if (reassess && report.records.length) {
        setAssessmentStale(true);
        try {
          const current = await api.currentReports();
          await refreshSession(generation, undefined, undefined, current.session_token);
          if (generation !== sessionGeneration.current) return false;
          setReport(current);
          setAssessmentStale(false);
        } catch (error) {
          if (generation !== sessionGeneration.current) return false;
          throw new Error(`設定已儲存，但報告顏色尚未重新判讀。${readableError(error)}`, { cause: error });
        }
      }
      return true;
    } catch (failure) {
      if (generation !== sessionGeneration.current) return false;
      if (isSessionInvalidation(failure)) { await expireOfficialSession(); return false; }
      if (failure instanceof Error && isSessionInvalidation(failure.cause)) { await expireOfficialSession(); return false; }
      throw failure;
    } finally {
      settingsLock.current = false;
      setSettingsBusy(false);
    }
  }

  function changeAppearance(patch: Partial<Settings>) {
    const generation = sessionGeneration.current;
    void commitSettings({ ...settings, ...patch }).catch((error: unknown) => {
      if (generation === sessionGeneration.current) setError(readableError(error));
    });
  }
  function toggleFavorite(key: string) {
    const generation = sessionGeneration.current;
    const favorites = settings.favorites.includes(key)
      ? settings.favorites.filter((value) => value !== key)
      : [...settings.favorites, key];
    void commitSettings({ ...settings, favorites }).catch((error: unknown) => {
      if (generation === sessionGeneration.current) setError(readableError(error));
    });
  }
  async function saveOverride(key: string, value: ItemOverride | null) {
    const overrides = value === null
      ? Object.fromEntries(Object.entries(settings.overrides).filter(([entryKey]) => entryKey !== key))
      : { ...settings.overrides, [key]: value };
    const applied = await commitSettings({ ...settings, overrides }, true);
    if (!applied) return;
    setMessage(
      value === null
        ? '此項目已還原預設規則。'
        : persisted
          ? '個人基準已儲存，報告顏色已更新。'
          : '本次使用的個人基準已更新。',
    );
  }
  async function restoreAllBaselines() {
    const generation = sessionGeneration.current;
    setRestoreBaselinesError('');
    try {
      const applied = await commitSettings({ ...settings, overrides: {} }, true);
      if (!applied) return;
      setRestoreBaselinesOpen(false);
      setMessage('所有個人基準已還原為預設規則，關注項目與閱讀設定已保留。');
    } catch (error) {
      if (generation === sessionGeneration.current) setRestoreBaselinesError(readableError(error));
    }
  }

  async function reassessReports() {
    const generation = sessionGeneration.current;
    setBusy('正在重新套用設定…');
    try {
      const result = await api.currentReports();
      await refreshSession(generation, undefined, undefined, result.session_token);
      if (generation !== sessionGeneration.current) return;
      setReport(result);
      setAssessmentStale(false);
    } catch (error) {
      if (generation === sessionGeneration.current) {
        if (isSessionInvalidation(error)) await expireOfficialSession();
        else setError(readableError(error));
      }
    } finally {
      if (generation === sessionGeneration.current) setBusy('');
    }
  }

  function exportCsv() {
    downloadText(
      toCsv(selectedExportRecords),
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
    <AnalysisProvider key={`${session.mode}:${session.csrf_token ?? ''}`} initial={settings.analysis} persisted={persisted}
      onSave={analysis => commitSettings({ ...settings, analysis })}>
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
              <span>{persisted ? '個人設定跨裝置同步' : '醫療資料僅供本次瀏覽'}</span>
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
              disabled={controlsBusy}
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
                disabled={controlsBusy}
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
              <span className="eyebrow workspace-eyebrow">健康映覽 · {session.mode === 'official' ? '官方授權資料' : session.mode === 'import' ? '匯入資料工作區' : session.mode === 'demo' ? '合成示範' : '醫療資料工作區'}</span>
              <h1 ref={mainHeading} tabIndex={-1}>
                {nav.label}
                <span className="title-dot">.</span>
              </h1>
              <p>
                {view === 'overview'
                  ? '將每一次檢驗，放回完整的時間脈絡。'
                  : view === 'data'
                    ? '將各來源紀錄放在一起，依自己的方式查找、比較與保存。'
                  : view === 'comparison'
                    ? '保留每一筆原始結果，清楚比較年度與各次檢查。'
                    : view === 'trends'
                      ? '從長期變化，理解單次數值所在的位置。'
                      : '依照自己的追蹤需求，調整閱讀與判讀基準。'}
              </p>
            </div>
            <div className="heading-actions">
              {report.records.length > 0 && view !== 'settings' && view !== 'data' && (
                <>
                  <button
                    className="button secondary"
                    onClick={exportCsv}
                    title={`目前篩選 ${selectedExportRecords.length} 筆；${laboratoryDateSummary(selectedExportRecords)}`}
                    disabled={assessmentStale}
                  >
                    <Icon name="download" size={18} />
                    <span>匯出篩選的 {selectedExportRecords.length} 筆 CSV</span>
                  </button>
                  <button
                    className="button secondary print-button"
                    title="由全部已取得資料製作專用報表；可另選分享範圍與排版。"
                    onClick={() => setPrintOpen(true)}
                    disabled={assessmentStale}
                  >
                    <Icon name="print" size={18} />
                    <span>列印／PDF</span>
                  </button>
                </>
              )}
              {view === 'data' && <>
                <a className="button secondary" href="#dataset-tools">保存／還原資料包</a>
                <button type="button" className="button secondary" onClick={() => setPrintOpen(true)} disabled={controlsBusy || (!report.records.length && !Object.keys(dataSnapshot.reports).length)}>製作列印／PDF 報表</button>
              </>}
              {persisted ? view !== 'data' ? (
                <>
                  <button
                    className="button primary"
                    onClick={() => { if (cacheWorkspace || deviceCache.state.enabled) void deviceCache.refresh(); else void fetchReports('all_history'); }}
                    disabled={controlsBusy}
                  >
                    <Icon name="refresh" size={18} />
                    {cacheWorkspace || deviceCache.state.enabled ? '清除舊資料並重新取得' : '取得全部歷史檢驗'}
                  </button>
                  <button
                    className="button secondary"
                    aria-expanded={rangeOpen}
                    aria-controls={rangeOpen ? 'report-date-range' : undefined}
                    onClick={() => setRangeOpen(!rangeOpen)}
                    disabled={controlsBusy || cacheWorkspace}
                  >
                    指定期間
                  </button>
                </>
              ) : null : (
                <button
                  className="button primary"
                  onClick={openLogin}
                  disabled={controlsBusy}
                >
                  <Icon name="login" size={18} />
                  連接健康存摺
                </button>
              )}
            </div>
          </div>

          {(view === 'comparison' || view === 'trends' || view === 'settings') && <details className="panel compact-disclosure"><summary>判讀條件</summary><ClinicalComparison records={report.records} /></details>}
          {view !== 'data' && <AnalysisStamp />}
          <DeviceHealthCache cache={deviceCache} disabled={workBusy} />
          {cacheWorkspace && <Alert>{report.cache_provenance
            ? '目前顯示此裝置保存的歷史資料；本次登入沒有重新向官方拉取。'
            : '醫療來源已從此裝置快取開啟；檢驗工作區保留目前另行取得的官方資料。'}各來源保留原始取得時間。需要更新時，請使用「清除舊資料並重新取得全部來源」。</Alert>}
          {view !== 'settings' && view !== 'data' && <CoverageSummary report={report} mode={session.mode} />}
          {rangeOpen && persisted && (
            <form
              id="report-date-range"
              className="date-range-panel"
              aria-label="指定查詢期間"
              onSubmit={(event) => {
                event.preventDefault();
                void fetchReports('custom');
              }}
            >
              <label className="field">
                開始日期
                <input
                  type="date"
                  value={dateFrom}
                  onChange={(event) => setDateFrom(event.target.value)}
                  max={dateTo || today()}
                  required
                  disabled={controlsBusy}
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
                  required
                  disabled={controlsBusy}
                />
              </label>
              <button className="button primary" type="submit" disabled={controlsBusy}>
                取得此範圍
              </button>
            </form>
          )}

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
              <button className="text-button" onClick={() => void logout()} disabled={controlsBusy}>
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
                onClick={() => { void reassessReports(); }}
                disabled={controlsBusy}
              >
                重新套用設定
              </button>
            </Alert>
          )}
          {view !== 'data' && report.official_query && (
            <Alert>
              <strong>本次取得 {report.records.length} 筆原始結果</strong>
              <br />
              查詢方式：{report.official_query.mode === 'all_history' ? '全部歷史' : '自訂期間'}。
              <br />
              實際送出查詢範圍：{dateLabel(report.official_query.start_date)}～
              {dateLabel(report.official_query.end_date)}；
              {laboratoryCoverageStatus(report, session.mode).summary}
              {report.official_query.mode === 'all_history' && (
                <>
                  <br />
                  全部歷史從民國1年起查詢；實際可提供的歷史資料依健康存摺，這不是官方資料最早年度。
                </>
              )}
            </Alert>
          )}
          {view !== 'data' && report.warnings
            .filter((warning) => !isDemo || !warning.startsWith('示範模式：'))
            .map((warning, index) => (
              <Alert tone="warning" key={`${warning}-${index}`}>
                {warning}
              </Alert>
            ))}
          {view !== 'data' && !report.complete && !report.official_query && report.records.length > 0 && (
            <Alert tone="warning">
              目前資料可能不完整。比較表僅呈現已取得的紀錄；缺少的日期不表示未曾檢驗。
            </Alert>
          )}
          {booting ? (
            <div className="boot-screen">
              <Loading label="正在開啟健康工作站…" />
            </div>
          ) : !report.records.length && view !== 'settings' && view !== 'data' ? (
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
                      if (persisted) { if (cacheWorkspace || deviceCache.state.enabled) void deviceCache.refresh(); else void fetchReports('all_history'); }
                      else openLogin();
                    }}
                    disabled={controlsBusy}
                  >
                    <Icon name={persisted ? 'refresh' : 'login'} />
                    {persisted ? '取得全部歷史' : '連接健康存摺'}
                  </button>
                  <button
                    className="button secondary large-button"
                    onClick={openImport}
                    disabled={controlsBusy}
                  >
                    <Icon name="upload" />
                    匯入已下載網頁
                  </button>
                </div>
                <button
                  className="text-button demo-link"
                  onClick={() => void openDemo()}
                  disabled={controlsBusy}
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
                      <p>共 {uniqueDates.length} 個已知檢查日期{unknownDateCount ? `；另有 ${unknownDateCount} 筆日期未提供` : ''}</p>
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
                        <><GradingContextNotes records={attentionRecords} /><div className="attention-list">
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
                                <b>{recordTerminology(record, terminology).name}</b>
                                {recordTerminology(record, terminology).englishName && <span className="terminology-english" lang="en">{recordTerminology(record, terminology).englishName}</span>}
                                <GradingContextLabel record={record} />
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
                        </div></>
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
                        onClick={openImport}
                        disabled={controlsBusy}
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
                          .map((group) => {
                            const latest = group.records[0];
                            if (!latest) return null;
                            return (
                            <button
                              className="favorite-card"
                              key={group.id}
                              onClick={() => openTrend(group.id)}
                            >
                              <span className="favorite-card-title">
                                <Icon name="star" size={17} />
                                <b>{groupNames.get(group.id)?.name ?? group.name}</b>
                                <Icon name="trend" size={19} />
                              </span>
                              {groupNames.get(group.id)?.englishName && <span className="terminology-english" lang="en">{groupNames.get(group.id)?.englishName}</span>}
                              <span className="favorite-value">
                                {latest.result}
                                <small>{group.unit}</small>
                              </span>
                              <span className="favorite-bottom">
                                <span><LevelBadge level={latest.level} compact /><GradingContextLabel record={latest} /></span>
                                <small>
                                  {dateLabel(latest.exam_date)}
                                  {group.records.filter(
                                    (record) => record.exam_date === latest.exam_date,
                                  ).length > 1
                                    ? ' · 同日多筆'
                                    : ''}
                                </small>
                              </span>
                            </button>
                            );
                          })}
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
                    <div className="segmented-control" role="group" aria-label="日期列分組方式">
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
                      terminology={terminology}
                      favorites={settings.favorites}
                      granularity={granularity}
                      saving={controlsBusy}
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
                <><Panel
                  title="單項數值趨勢"
                  subtitle="同一項目與單位，按實際時間比較"
                  action={
                    selectedGroup &&
                    configurableKeys.includes(selectedGroup.configurationKey) && (
                      <button
                        className="button secondary"
                        onClick={() => setBaseline(selectedGroup)}
                        disabled={controlsBusy}
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
                                  {groupNames.get(group.id)?.name ?? group.name}{groupNames.get(group.id)?.englishName ? ` / ${groupNames.get(group.id)?.englishName}` : ''} · {group.unit}
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
                          <h3>{groupNames.get(trendGroup.id)?.name ?? trendGroup.name}</h3>
                          {groupNames.get(trendGroup.id)?.englishName && <p className="terminology-english" lang="en">{groupNames.get(trendGroup.id)?.englishName}</p>}
                          <p>
                            {trendGroup.abbreviation} · {trendGroup.unit}
                          </p>
                        </div>
                        <button
                          className={`button ${settings.favorites.includes(trendGroup.configurationKey) ? 'favorite-active' : 'secondary'}`}
                          onClick={() => toggleFavorite(trendGroup.configurationKey)}
                          disabled={
                            controlsBusy || !configurableKeys.includes(trendGroup.configurationKey)
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
                <MultiIndicator groups={groups.map(group => ({ ...group, records: year === 'all' ? group.records : group.records.filter(record => record.exam_date.startsWith(year)) }))} settings={settings} onDetail={setDetail} />
                <LipidCalculation records={report.records} /></>
              )}
            </>
          )}

          {!booting && view === 'settings' && (
            <>
              <div className="panel analysis-tools"><h2>同步設定管理</h2><p>不包含原始帳密、生日、檢驗結果或個人用藥紀錄；假名化設定仍可連結同一使用者。</p>
                <button className="button secondary" onClick={() => downloadText(JSON.stringify(settings, null, 2), 'healthbank-設定.json', 'application/json')}>匯出目前設定</button>
                {persisted && <button className="button secondary" disabled={controlsBusy} onClick={() => {
                  if (window.confirm('刪除伺服器上此使用者的全部設定，並使本站所有裝置登出？此操作不能復原。瀏覽器記住的資料及營運者備份不會由此一併清除。')) void logout(true);
                }}>刪除伺服器設定並登出所有裝置</button>}
              </div>
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
                          disabled={controlsBusy}
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
                          disabled={controlsBusy}
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
                      <dd>伺服器只在目前使用階段處理。裝置健康快取須另外明確同意；已同意的本機快取會在登出後保留，可在上方個別或全部清除。</dd>
                    </div>
                    <div>
                      <dt>登入欄位</dt>
                      <dd>證號與健保卡號依您的選擇記在這部裝置；密碼由瀏覽器密碼管理器處理。</dd>
                    </div>
                  </dl>
                  <button
                    className="button secondary"
                    onClick={() => {
                      try {
                        clearRememberedLogin();
                        setMessage('這部裝置記住的證號與健保卡號已清除；密碼管理員的密碼不受影響。');
                      } catch (failure) { setError(readableError(failure)); }
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
                    disabled={controlsBusy || customBaselineCount === 0}
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
                {catalog.terminology_notice && <p className="helper-text">{catalog.terminology_notice}</p>}
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
                            {groupTerminology(group, terminology).englishName && <span className="terminology-english" lang="en">{groupTerminology(group, terminology).englishName}</span>}
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
                            {rule.scopeLabel && <small className="grading-context-label">{rule.scopeLabel}</small>}
                            {(rule.conditions.length > 0 || rule.applicabilityNote) && <details className="grading-context-notes"><summary>預設規則適用條件</summary>
                              {rule.applicabilityNote && <p>{rule.applicabilityNote}</p>}
                              {rule.conditions.length > 0 && <ul>{rule.conditions.map((condition, index) => <li key={index}>{condition}</li>)}</ul>}
                            </details>}
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
                              disabled={controlsBusy}
                              aria-label={`檢視／編輯：${group.name}基準`}
                            >
                              <Icon name="settings" size={17} />
                              檢視／編輯
                            </button>
                            {override && (
                              <button
                                className="button quiet"
                                onClick={() => void saveOverride(group.configurationKey, null)
                                  .catch((error: unknown) => setError(readableError(error)))}
                                disabled={controlsBusy}
                                aria-label={`還原預設：${group.name}規則`}
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


          {view !== 'settings' && view !== 'data' && <><AnalysisExport records={view === 'trends' && trendGroup ? trendGroup.records : filteredGroups.flatMap(group => group.records)} /><PortableReportExport records={report.records} /></>}
          <div hidden={view !== 'data'}>
            {dataCatalogError && <Alert tone="error">{dataCatalogError}<button className="button secondary" type="button" disabled={dataCatalogLoading} onClick={() => { void reloadDataCatalog(); }}>{dataCatalogLoading ? '重新取得中…' : '重新取得來源目錄'}</button></Alert>}
            {!booting && <>
              <SupplementaryData key={`${session.mode}:${session.csrf_token ?? ''}:${datasetVersion}`}
                authenticated={persisted && !cacheWorkspace} imported={session.mode === 'import'} cached={cacheWorkspace}
                sessionMode={session.mode}
                disabled={settingsBusy || deviceCache.busy || busy !== '' || dataCatalog.length === 0}
                catalog={workspaceCatalog} fieldTerms={dataFieldTerms} initialDataset={dataSnapshot} archivedAttachments={archivedAttachments} laboratory={report}
                onDatasetChange={next => { if (datasetVersion === datasetVersionRef.current) setDataSnapshot(next); }}
                onAttachmentDownloaded={retainAttachment} onFetchLaboratory={fetchLaboratoryForWorkspace}
                datasetProducer={datasetProducer}
                onBatchActiveChange={value => { if (datasetVersion === datasetVersionRef.current) setDataBatchActive(value); }}
                onBeginBatch={(categories, includeLaboratory) => {
                  if (datasetVersion !== datasetVersionRef.current) return;
                  const selected = new Set(categories);
                  const nextAttachments = dataAttachmentsRef.current.filter(file => !selected.has(file.category));
                  dataAttachmentsRef.current = nextAttachments; setDataAttachments(nextAttachments);
                  if (includeLaboratory) {
                    setReport({ records: [], fetched_at: '', complete: false, warnings: ['本批檢驗尚未取得；不沿用先前期間的結果。'] });
                    clearReportRange();
                  }
                }}
                onBusyChange={value => { if (datasetVersion === datasetVersionRef.current) setDataBusy(value); }}
                onOpenTrends={date => {
                  setYear('all');
                  if (!selectedGroup?.records.some(record => record.exam_date === date)) {
                    const matching = groups.find(group => group.records.some(record => record.exam_date === date));
                    if (matching) setSelectedGroupId(matching.id);
                  }
                  navigate('trends');
                }}
                onSessionExpired={() => { void expireOfficialSession(); }} />
              <NativePrintReport report={datasetExportReport} snapshot={dataSnapshot} categories={workspaceCatalog} mode={session.mode} version={session.version} reportOpen={printOpen} />
              <DatasetTools key={`dataset:${datasetVersion}`} report={datasetExportReport} snapshot={dataSnapshot} attachments={dataAttachments} catalog={workspaceCatalog}
                producer={datasetProducer} importedFrom={importedDatasetProducer} disabled={controlsBusy} collection={restorationContext} onImport={restoreDataset}
                onCollectionChange={next => { if (datasetVersion === datasetVersionRef.current) setRestorationContext(next); }} />
            </>}
          </div>
          <div hidden={view !== 'trends' && view !== 'settings'}><PersonalRecords key={`${session.mode}:${session.profile_key || report.fetched_at}`} profileKey={session.profile_key ?? ''} /></div>
          <div className="print-only print-running-footer">
            <Copyright />
          </div>
          <footer className="page-footer">
            <div>
              <span>健康映覽</span>
              <span>醫療資料整理與長期追蹤</span>
            </div>
            <Copyright />
            <span className="footer-note">顏色為整理提示，請結合完整病史與專業判斷。 · MIT License</span>
            <PrivacyNotice />
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
      {loginOpen && <LoginDialog onClose={() => setLoginOpen(false)} onLogin={loggedIn}
        onSession={reconcileObservedSession}
        onFailure={failure => reconcileFailedSessionMutation(failure, sessionGeneration.current)} />}
      {importOpen && <ImportDialog onClose={() => setImportOpen(false)} onImport={importFile} />}
      {printOpen && <PrintReport key={`print:${session.mode}:${datasetVersion}`} report={datasetExportReport} snapshot={dataSnapshot} categories={workspaceCatalog}
        selectedLaboratory={selectedExportRecords} mode={session.mode} version={session.version} onClose={() => setPrintOpen(false)} />}
      {detail && (
        <DetailDialog
          record={detail}
          catalog={catalog}
          canConfigure={!controlsBusy && configurableKeys.includes(detail.configuration_key)}
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
            <button className="button secondary" onClick={() => setRestoreBaselinesOpen(false)} disabled={controlsBusy}>
              保留個人基準
            </button>
            <button className="button primary" onClick={() => void restoreAllBaselines()} disabled={controlsBusy || customBaselineCount === 0}>
              {settingsBusy ? <Loading label="還原中…" /> : '還原全部基準'}
            </button>
          </div>
        </Modal>
      )}
    </div>
    </AnalysisProvider>
  );
}
