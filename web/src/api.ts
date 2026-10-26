// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT

export type Level = 1 | 2 | 3 | 4 | 5;
export type Theme = 'light' | 'dark' | 'system';
export type SessionMode = 'official' | 'import' | 'demo' | 'anonymous';
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
  theme: Theme;
  font_scale: number;
  favorites: string[];
  overrides: Record<string, ItemOverride>;
  known_items?: Record<
    string,
    { id: string; configuration_key: string; name: string; unit: string; category: string }
  >;
}

export interface Session {
  authenticated: boolean;
  mode: SessionMode;
  settings: Settings;
  version: string;
  csrf_token?: string;
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

export interface ReportResponse {
  records: LabRecord[];
  fetched_at: string;
  complete: boolean;
  warnings: string[];
  official_query?: {
    start_date: string;
    end_date: string;
    reported_count: number | null;
  };
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

export class ApiError extends Error {
  constructor(
    message: string,
    public code: string,
    public status: number,
    public requestId?: string,
    public upstreamStatus?: number,
    public upstreamCode?: string,
    public incompleteCounts?: { expected: number; received: number },
  ) {
    super(message);
    this.name = 'ApiError';
  }
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
  if (csrfToken && options.method && options.method !== 'GET')
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

export const api = {
  session: (signal?: AbortSignal) => request<Session>('/api/session', { signal }),
  captcha: (signal?: AbortSignal) => request<CaptchaResponse>('/api/auth/captcha', { signal }),
  login: (body: LoginInput) =>
    request<Session>('/api/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request<Session>('/api/auth/logout', { method: 'POST', body: '{}' }),
  reports: (from: string, to: string) =>
    request<ReportResponse>(`/api/reports?${new URLSearchParams({ from, to })}`),
  currentReports: (signal?: AbortSignal) =>
    request<ReportResponse>('/api/reports/current', { signal }),
  importReports: (file: File) => {
    const body = new FormData();
    body.append('file', file);
    return request<ReportResponse>('/api/reports/import', { method: 'POST', body });
  },
  demo: () => request<ReportResponse>('/api/reports/demo', { method: 'POST', body: '{}' }),
  catalog: (signal?: AbortSignal) => request<CatalogResponse>('/api/catalog', { signal }),
  saveSettings: (settings: Settings) =>
    request<Settings>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({
        revision: settings.revision,
        theme: settings.theme,
        font_scale: settings.font_scale,
        favorites: settings.favorites,
        overrides: settings.overrides,
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
      details.push(`已收到 ${error.incompleteCounts.received} 筆／官方回報 ${error.incompleteCounts.expected} 筆`);
    if (error.requestId) details.push(`查詢代碼：${error.requestId}`);
    return error.message + (details.length ? `（${details.join('；')}）` : '');
  }
  if (error instanceof TypeError) return '目前無法連線到服務，請確認網路後重試。';
  return error instanceof Error ? error.message : '操作未完成，請稍後再試。';
}
