// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT
import type { AnalysisPreferences } from './clinical-comparison';
import type { PackageReceipt, PackageRequest, PreparationResult } from './full-acquisition';

export type Level = 1 | 2 | 3 | 4 | 5;
export type Theme = 'light' | 'dark' | 'system';
export type SessionMode = 'official' | 'import' | 'demo' | 'anonymous';
export type ReportQueryMode = 'all_history' | 'custom';
export type ReferenceStatus = 'within' | 'high' | 'low' | 'unknown';

export interface LevelBand {
  lower: string | null;
  upper: string | null;
  lower_inclusive: boolean;
  upper_inclusive: boolean;
  level: Level;
}

export interface ItemOverride {
  reference_low?: string;
  reference_high?: string;
  bands?: LevelBand[];
  enabled: boolean;
}

export interface Settings {
  revision: number;
  analysis?: AnalysisPreferences | null;
  theme: Theme;
  font_scale: number;
  favorites: string[];
  overrides: Record<string, ItemOverride>;
  known_items?: Record<
    string,
    { id: string; configuration_key: string; name: string; unit: string; category: string }
  >;
}

export interface PrivacyInfo {
  operator: string; contact: string; processing_region: string; backup_retention: string;
  session_idle_minutes: number; cleanup_interval_seconds: number; privacy_revision: string; settings_retention: string;
}
export interface Session {
  authenticated: boolean;
  mode: SessionMode;
  settings: Settings;
  version: string;
  csrf_token?: string;
  profile_key?: string;
}

export interface LabRecord {
  id: string;
  item_id: string;
  configuration_key: string;
  item_name: string;
  display_name: string;
  category: string;
  exam_date: string;
  visit_date: string;
  raw_check_date?: string;
  raw_visit_date?: string;
  order_code: string;
  order_name: string;
  result: string;
  unit: string;
  reference_low: string | null;
  reference_high: string | null;
  reference_text: string;
  institution: string;
  source: string;
  level: Level | null;
  reference_status: ReferenceStatus;
  interpretation: string;
  numeric_value: number | null;
  value_kind: string;
  rule_source: string;
  official_flag?: string;
}

export interface MedicineOrder {
  code: string;
  name: string;
  quantity: string;
  days: string;
}
export interface DocumentAttachment {
  id: string;
  name: string;
  content_type: string;
  size?: number;
}
export interface DocumentAction {
  id: string;
  name: string;
  kind: 'prepare_jpeg' | 'prepare_dicom' | 'prepare_export';
}
export interface DataCategory {
  id: string;
  label: string;
  group: string;
  range_mode: 'date_search' | 'source';
  has_details?: boolean;
}
export type SourceDateRole = 'examination' | 'specimen_collection' | 'report' | 'encounter' | 'admission' | 'discharge' | 'claim_start' | 'claim_end' | 'prescription' | 'dispensing' | 'recorded' | 'uploaded' | 'valid_from' | 'valid_to' | 'period_start' | 'period_end' | 'unknown';
export interface SupplementField {
  name: string; value: string; unit?: string; code?: string;
  role?: 'result' | 'unit' | 'reference' | 'method' | 'specimen' | 'date' | 'medication_code' | 'medication_name' | 'prescribed_days' | 'quantity';
  unit_basis?: 'source' | 'public_contract';
}
export interface SourceDiagnostic {
  code: 'section_missing' | 'section_optional_absent' | 'section_null' | 'section_empty' | 'known_result_missing';
  severity: 'info' | 'warning'; section: string; count?: number; message: string;
}
export interface SupplementEntry {
  date: string; institution: string; title: string; source: string;
  fields: SupplementField[];
  date_role?: SourceDateRole;
  end_date_role?: SourceDateRole;
  origin?: 'institutional' | 'self_entered' | 'device' | 'official_summary' | 'mixed' | 'unknown';
  contract_id?: string;
  detail_id?: string;
  key?: string;
  kind?: string;
  end_date?: string;
  medicine?: MedicineOrder;
  attachments?: DocumentAttachment[];
  document_actions?: DocumentAction[];
}
export interface SupplementReport {
  category: string; entries: SupplementEntry[]; coverage: string; fetched_at: string;
  start_date?: string; end_date?: string; complete: boolean;
  contract_revision?: string;
  diagnostics?: SourceDiagnostic[];
}
export interface ReportResponse {
  // Live API generation proof only; never part of a portable medical document.
  session_token?: string;
  records: LabRecord[];
  fetched_at: string;
  complete: boolean;
  warnings: string[];
  official_query?: {
    mode: ReportQueryMode;
    start_date: string;
    end_date: string;
    reported_count: number | null;
  };
}

export interface LiveReportResponse extends ReportResponse {
  session_token: string;
}

export interface CatalogItem {
  id: string;
  configuration_key: string;
  display_name: string;
  category: string;
  unit: string;
  reference_low: string | null;
  reference_high: string | null;
  default_rule: unknown;
  description: string;
  sources: string[];
  builtin: boolean;
  aliases: string[];
  order_codes: string[];
  value_kind?: string;
  default_reference: CatalogReference | null;
  alert_rules: unknown[];
}

export interface CatalogReference {
  lower: string | null;
  upper: string | null;
  lower_inclusive: boolean;
  upper_inclusive: boolean;
  expected_values?: string[];
  source: string;
  scope?: string;
  automatic_fallback_allowed: boolean;
}

export interface Source {
  id: string;
  title: string;
  url: string;
  edition?: string;
}

export interface CatalogResponse {
  items: CatalogItem[];
  sources: Source[];
  provenance?: { catalog_id: string; catalog_revision: string; evidence_revision: string };
}

export interface LoginInput {
  identity_number: string;
  card_number: string;
  password: string;
  captcha: string;
  challenge_id: string;
}

export interface CaptchaResponse {
  image_url: string;
  challenge_id: string;
}

const connectionPhaseLabels = {
  unknown: '無法確認',
  prepare: '建立請求',
  dns: 'DNS 解析',
  tcp_connect: 'TCP 連線',
  proxy_connect: '代理連線',
  tls_handshake: 'TLS 握手',
  connection_ready: '建立連線後',
  request_write: '傳送請求',
  response_headers: 'HTTP 回應標頭',
  response_body: '讀取回應內容',
  complete: 'HTTP 回應讀取完成',
} as const;
type ConnectionPhase = keyof typeof connectionPhaseLabels;

export class ApiError extends Error {
  constructor(
    message: string,
    public code: string,
    public status: number,
    public requestId?: string,
    public upstreamStatus?: number,
    public upstreamCode?: string,
    public incompleteCounts?: { expected: number; received: number },
    public connectionPhase?: ConnectionPhase,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// These responses explicitly retire the caller's session generation. Other
// forbidden/conflict responses (origin, validation or revision conflicts) do not.
export function isSessionInvalidation(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 401 ||
    error.status === 403 && error.code === 'csrf_mismatch' ||
    error.status === 409 && error.code === 'session_changed' ||
    error.status === 500 && error.code === 'session_rotate' ||
    error.code === 'invalid_report_session');
}

let csrfToken = '';
export function setCsrfToken(token?: string) {
  csrfToken = token ?? '';
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set('Accept', 'application/json');
  if (options.body && !(options.body instanceof FormData))
    headers.set('Content-Type', 'application/json');
  // Read requests also belong to the page's current generation. A shared
  // browser cookie may have changed in another tab since this UI was rendered.
  // /api/session is the explicit operation that reconciles such a change.
  if (csrfToken && path !== '/api/session')
    headers.set('X-CSRF-Token', csrfToken);
  const response = await fetch(path, {
    ...options,
    headers,
    credentials: 'same-origin',
    cache: 'no-store',
  });
  const contentType = response.headers.get('content-type') ?? '';
  let payload: unknown = null;
  if (contentType.includes('application/json')) {
    try {
      payload = await response.json();
    } catch {
      throw new ApiError(
        '服務回傳的資料格式不正確，請稍後再試。',
        'invalid_response',
        response.status,
      );
    }
  }
  if (!response.ok) {
    const detail = (
      payload as {
        error?: {
          message?: string;
          code?: string;
          request_id?: string;
          upstream_status?: number;
          upstream_code?: string;
          expected_count?: number;
          received_count?: number;
          diagnostics?: { phase?: unknown };
        };
      } | null
    )?.error;
    const upstreamStatus = detail?.upstream_status;
    const expectedCount = detail?.expected_count;
    const receivedCount = detail?.received_count;
    const incompleteCounts =
      detail?.code === 'INCOMPLETE_RESPONSE' &&
      typeof expectedCount === 'number' && Number.isSafeInteger(expectedCount) &&
      typeof receivedCount === 'number' && Number.isSafeInteger(receivedCount) &&
      expectedCount > 0 && expectedCount <= 50000 &&
      receivedCount >= 0 && receivedCount < expectedCount
        ? { expected: expectedCount, received: receivedCount }
        : undefined;
    const phase = detail?.diagnostics?.phase;
    const connectionPhase =
      typeof phase === 'string' && Object.hasOwn(connectionPhaseLabels, phase)
        ? phase as ConnectionPhase
        : undefined;
    throw new ApiError(
      detail?.message ?? `服務暫時無法完成要求（HTTP ${response.status}）。`,
      detail?.code ?? 'http_error',
      response.status,
      detail?.request_id,
      typeof upstreamStatus === 'number' && Number.isInteger(upstreamStatus) &&
        upstreamStatus >= 100 && upstreamStatus <= 599
        ? upstreamStatus
        : undefined,
      typeof detail?.upstream_code === 'string' &&
        /^(?:[A-Z][0-9]{3}[A-Z]?|[0-9]{4})$/.test(detail.upstream_code)
        ? detail.upstream_code
        : undefined,
      incompleteCounts,
      connectionPhase,
    );
  }
  if (payload === null)
    throw new ApiError(
      '服務回傳的資料格式不正確，請稍後再試。',
      'invalid_response',
      response.status,
    );
  return payload as T;
}

async function requestReport(path: string, options: RequestInit = {}): Promise<LiveReportResponse> {
  const report = await request<ReportResponse>(path, options);
  if (typeof report.session_token !== 'string' || !/^[a-f0-9]{64}$/.test(report.session_token))
    throw new ApiError('伺服器未提供有效的資料使用階段識別，未套用這份報告。請重新載入頁面。', 'invalid_report_session', 200);
  return { ...report, session_token: report.session_token };
}

async function fetchDataCatalog(signal?: AbortSignal): Promise<{ categories: DataCategory[] }> {
  const result = await request<{ categories: unknown }>('/api/data/catalog', { signal: signal ?? null });
  if (!Array.isArray(result.categories) || result.categories.length === 0 || result.categories.length > 128)
    throw new ApiError('資料來源目錄格式不正確，請重新取得。', 'invalid_catalog', 200);
  const categories: DataCategory[] = [];
  const seen = new Set<string>();
  const values: unknown[] = result.categories;
  for (const item of values) {
    if (typeof item !== 'object' || item === null || !('id' in item) || !('label' in item) ||
      !('group' in item) || !('range_mode' in item) || typeof item.id !== 'string' ||
      !/^[a-z][a-z0-9_]{0,63}$/.test(item.id) || seen.has(item.id) || typeof item.label !== 'string' ||
      !item.label.trim() || item.label.length > 240 || typeof item.group !== 'string' ||
      !item.group.trim() || item.group.length > 240 ||
      item.range_mode !== 'date_search' && item.range_mode !== 'source' ||
      'has_details' in item && typeof item.has_details !== 'boolean')
      throw new ApiError('資料來源目錄格式不正確，請重新取得。', 'invalid_catalog', 200);
    seen.add(item.id);
    categories.push({ id: item.id, label: item.label, group: item.group, range_mode: item.range_mode,
      ...('has_details' in item && typeof item.has_details === 'boolean' ? { has_details: item.has_details } : {}) });
  }
  return { categories };
}

function invalidAcquisitionResponse(): never {
  throw new ApiError('完整取得服務回傳的狀態格式不正確；未宣稱檔案已完成。', 'invalid_response', 200);
}

function acquisitionObject(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return invalidAcquisitionResponse();
  return value as Record<string, unknown>;
}

function acquisitionCount(value: unknown, maximum: number): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > maximum) return invalidAcquisitionResponse();
  return value;
}

const packageStates = ['ready', 'streaming', 'completed', 'partial', 'failed', 'cancelled'] as const;
function packageIdentity(value: Record<string, unknown>) {
  const id = value['id'], expires = value['expires_at'];
  const state = packageStates.find(item => item === value['state']);
  if (typeof id !== 'string' || !/^[a-f0-9]{48}$/.test(id) || !state || typeof expires !== 'string' ||
    !Number.isFinite(Date.parse(expires))) return invalidAcquisitionResponse();
  return { id, state, expires_at: expires };
}

async function prepareDocument(category: string, id: string, signal?: AbortSignal): Promise<PreparationResult> {
  const value = acquisitionObject(await request<unknown>('/api/documents/prepare', {
    method: 'POST', body: JSON.stringify({ category, id }), signal: signal ?? null,
  }));
  const refresh = acquisitionObject(value['refresh']);
  const refreshCategory = refresh['category'], detail = refresh['detail'], correlation = refresh['correlation'];
  if (value['status'] !== 'requested' || typeof value['message'] !== 'string' || value['message'].length > 4000 ||
    typeof refreshCategory !== 'string' || !/^[a-z][a-z0-9_]{0,63}$/.test(refreshCategory) ||
    detail !== undefined && (typeof detail !== 'string' || !/^[a-f0-9]{48}$/.test(detail)) ||
    correlation !== 'same_record' && correlation !== 'unavailable' ||
    correlation === 'same_record' && (refreshCategory !== category || detail === undefined) ||
    correlation === 'unavailable' && (refreshCategory !== 'official_files' || detail !== undefined)) return invalidAcquisitionResponse();
  return { status: 'requested', message: value['message'], refresh: { category: refreshCategory, correlation,
    ...(typeof detail === 'string' ? { detail } : {}) } };
}

async function createAcquisitionPackage(input: PackageRequest, signal?: AbortSignal): Promise<PackageReceipt> {
  const { dataset, ...metadata } = input;
  if (Boolean(dataset) === Boolean(metadata.external_dataset)) throw new Error('原始檔包必須指定一份完整資料包或已保存的外部資料包。');
  const serialized = JSON.stringify(metadata);
  // dataset is produced by the strict v2 exporter. Insert its JSON object
  // directly rather than allocating another fully escaped medical string.
  const body = dataset ? `${serialized.slice(0, -1)},"dataset":${dataset.trim()}}` : serialized;
  const value = acquisitionObject(await request<unknown>('/api/acquisition/packages', { method: 'POST', body, signal: signal ?? null }));
  const receipt = acquisitionPackageReceipt(value);
  assertPackageReferences(receipt, input.files);
  return receipt;
}

async function acquisitionPackageStatus(identifier: { id: string } | { plan: string; part: number }, signal?: AbortSignal): Promise<PackageReceipt> {
  const query = 'id' in identifier ? new URLSearchParams({ id: identifier.id })
    : new URLSearchParams({ plan: identifier.plan, part: String(identifier.part) });
  const value = acquisitionObject(await request<unknown>(`/api/acquisition/packages/status?${query}`, { signal: signal ?? null }));
  const receipt = acquisitionPackageReceipt(value);
  if ('id' in identifier && receipt.id !== identifier.id) return invalidAcquisitionResponse();
  return receipt;
}

export function assertPackageReferences(receipt: PackageReceipt, files: readonly { archive_id: string }[]): void {
  if (receipt.total !== files.length || receipt.files.some((file, index) => file.archive_id !== files[index]?.archive_id))
    invalidAcquisitionResponse();
}

function acquisitionPackageReceipt(value: Record<string, unknown>): PackageReceipt {
  const identity = packageIdentity(value);
  const total = acquisitionCount(value['total'], 1000), completed = acquisitionCount(value['completed'], total), failed = acquisitionCount(value['failed'], total);
  if (completed + failed > total || !Array.isArray(value['files']) || value['files'].length !== total) return invalidAcquisitionResponse();
  const fileStates = ['queued', 'streaming', 'completed', 'failed', 'cancelled', 'skipped'] as const;
  const seen = new Set<string>();
  const files = (value['files'] as unknown[]).map(item => {
    const file = acquisitionObject(item), archiveID = file['archive_id'], digest = file['sha256'], code = file['code'];
    const state = fileStates.find(item => item === file['state']);
    if (typeof archiveID !== 'string' || !/^archive_[1-9][0-9]{0,8}$/.test(archiveID) || seen.has(archiveID) || !state ||
      digest !== undefined && (typeof digest !== 'string' || !/^[a-f0-9]{64}$/.test(digest)) ||
      state === 'completed' && digest === undefined ||
      code !== undefined && (typeof code !== 'string' || !/^[A-Za-z0-9_]{1,100}$/.test(code))) return invalidAcquisitionResponse();
    seen.add(archiveID);
    return { archive_id: archiveID, state, bytes: acquisitionCount(file['bytes'], 500 * 1024 * 1024),
      ...(typeof digest === 'string' ? { sha256: digest } : {}), ...(typeof code === 'string' ? { code } : {}) };
  });
  const digest = value['package_sha256'];
  if (digest !== undefined && (typeof digest !== 'string' || !/^[a-f0-9]{64}$/.test(digest))) return invalidAcquisitionResponse();
  if (files.filter(file => file.state === 'completed').length !== completed || files.filter(file => file.state === 'failed').length !== failed ||
    identity.state === 'completed' && completed !== total ||
    (identity.state === 'completed' || identity.state === 'partial') && (digest === undefined || value['package_bytes'] === undefined) ||
    digest !== undefined && identity.state !== 'completed' && identity.state !== 'partial') return invalidAcquisitionResponse();
  const bytes = acquisitionCount(value['bytes'], 8 * 1024 * 1024 * 1024);
  if (bytes !== files.reduce((sum, file) => sum + file.bytes, 0)) return invalidAcquisitionResponse();
  return { ...identity, total, completed, failed, bytes, files,
    ...(typeof digest === 'string' ? { package_sha256: digest } : {}),
    ...(value['package_bytes'] === undefined ? {} : { package_bytes: acquisitionCount(value['package_bytes'], 9 * 1024 * 1024 * 1024) }) };
}

export const api = {
  dataCatalog: fetchDataCatalog,
  document: (category: string, id: string, signal: AbortSignal) =>
    fetch(`/api/documents?${new URLSearchParams({ category, id })}`, {
      credentials: 'same-origin', cache: 'no-store', signal,
      headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {},
    }),
  prepareDocument,
  createAcquisitionPackage,
  acquisitionPackageStatus,
  cancelAcquisitionPackage: (id: string, signal?: AbortSignal) => request<{ status: 'cancelled' }>(
    `/api/acquisition/packages?${new URLSearchParams({ id })}`, { method: 'DELETE', signal: signal ?? null }),
  supplementary: (category: string, from: string, to: string, detail: string, signal: AbortSignal) =>
    request<SupplementReport>(`/api/supplementary?${new URLSearchParams({ category, from, to, detail })}`, { signal }),
  privacy: (signal?: AbortSignal) => request<PrivacyInfo>('/api/privacy', signal ? { signal } : {}),
  deleteSettings: (revision: number) => request<Session>('/api/settings', { method: 'DELETE', body: JSON.stringify({ revision }) }),
  session: (signal?: AbortSignal) => request<Session>('/api/session', { signal: signal ?? null }),
  captcha: (signal?: AbortSignal) => request<CaptchaResponse>('/api/auth/captcha', { signal: signal ?? null }),
  login: (body: LoginInput) =>
    request<Session>('/api/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request<Session>('/api/auth/logout', { method: 'POST', body: '{}' }),
  reports: (from: string, to: string, signal?: AbortSignal) =>
    requestReport(`/api/reports?${new URLSearchParams({ from, to })}`, { signal: signal ?? null }),
  currentReports: (signal?: AbortSignal) =>
    requestReport('/api/reports/current', { signal: signal ?? null }),
  importReports: (file: File) => {
    const body = new FormData();
    body.append('file', file);
    return requestReport('/api/reports/import', { method: 'POST', body });
  },
  demo: () => requestReport('/api/reports/demo', { method: 'POST', body: '{}' }),
  catalog: (signal?: AbortSignal) => request<CatalogResponse>('/api/catalog', { signal: signal ?? null }),
  saveSettings: (settings: Settings) =>
    request<Settings>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({
        revision: settings.revision,
        theme: settings.theme,
        font_scale: settings.font_scale,
        favorites: settings.favorites,
        overrides: settings.overrides,
        analysis: settings.analysis ?? null,
      }),
    }),
};

export const defaultSettings = (): Settings => ({
  revision: 0,
  theme: 'system',
  font_scale: 1,
  favorites: [],
  overrides: {},
});

export function readableError(error: unknown): string {
  if (error instanceof ApiError) {
    const details: string[] = [];
    if (error.status === 424 && /^[A-Z_]{1,48}$/.test(error.code))
      details.push(`錯誤代碼：${error.code}`);
    if (error.upstreamStatus) details.push(`連線回應：HTTP ${error.upstreamStatus}`);
    if (error.upstreamCode) details.push(`官方代碼：${error.upstreamCode}`);
    if (error.incompleteCounts)
      details.push(`已收到 ${error.incompleteCounts.received} 筆／已知至少 ${error.incompleteCounts.expected} 筆`);
    if (error.connectionPhase)
      details.push(`最後連線階段：${connectionPhaseLabels[error.connectionPhase]}`);
    if (error.requestId) details.push(`查詢代碼：${error.requestId}`);
    return error.message + (details.length ? `（${details.join('；')}）` : '');
  }
  if (error instanceof TypeError) return '目前無法連線到服務，請確認網路後重試。';
  return error instanceof Error ? error.message : '操作未完成，請稍後再試。';
}
