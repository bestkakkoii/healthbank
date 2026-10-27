// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState, type SubmitEvent, type DragEvent } from 'react';
import {
  api,
  readableError,
  setCsrfToken,
  type CaptchaResponse,
  type CatalogResponse,
  type ItemOverride,
  type LabRecord,
  type Level,
  type LevelBand,
  type LoginInput,
  type Session,
} from './api';
import {
  categoryLabel,
  dateLabel,
  LEVELS,
  isObject,
  referenceLabel,
  referenceTemplateLabel,
  ruleCategoryLabel,
  rulePreview,
  ruleRangeLabel,
  sourceFlag,
  type MeasurementGroup,
  type RuleBand,
} from './model';
import { Alert, Icon, LevelBadge, Loading, Modal } from './ui';

const REMEMBER_KEY = 'bestkakkoi.healthbank.login-fields';

function rememberedFields(): { identity: string; card: string; remember: boolean } {
  try {
    const raw = localStorage.getItem(REMEMBER_KEY);
    if (!raw) return { identity: '', card: '', remember: true };
    const value: unknown = JSON.parse(raw);
    if (!isObject(value)) return { identity: '', card: '', remember: false };
    return {
      identity: typeof value['identity'] === 'string' ? value['identity'] : '',
      card: typeof value['card'] === 'string' ? value['card'] : '',
      remember: true,
    };
  } catch {
    return { identity: '', card: '', remember: false };
  }
}

export function clearRememberedLogin(): void {
  try {
    localStorage.removeItem(REMEMBER_KEY);
  } catch {
    /* Private browsing may disallow storage. */
  }
}

export function LoginDialog({
  onClose,
  onLogin,
}: {
  onClose: () => void;
  onLogin: (session: Session) => Promise<void>;
}) {
  const [initial] = useState(rememberedFields);
  const [identity, setIdentity] = useState(initial.identity);
  const [card, setCard] = useState(initial.card);
  const [remember, setRemember] = useState(initial.remember);
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [captcha, setCaptcha] = useState('');
  const [challenge, setChallenge] = useState<CaptchaResponse | null>(null);
  const [captchaBusy, setCaptchaBusy] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const captchaRequest = useRef<AbortController | null>(null);

  async function loadCaptcha() {
    captchaRequest.current?.abort();
    const controller = new AbortController();
    captchaRequest.current = controller;
    const { signal } = controller;
    setCaptchaBusy(true);
    setCaptcha('');
    setChallenge(null);
    try {
      const current = await api.session(signal);
      signal.throwIfAborted();
      setCsrfToken(current.csrf_token);
      const next = await api.captcha(signal);
      signal.throwIfAborted();
      setChallenge(next);
    } catch (error) {
      if (!signal.aborted)
        setError((previous) =>
          previous ? `${previous} ${readableError(error)}` : readableError(error),
        );
    } finally {
      if (!signal.aborted) setCaptchaBusy(false);
    }
  }

  useEffect(() => {
    void loadCaptcha();
    return () => captchaRequest.current?.abort();
  }, []);

  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!challenge || busy) return;
    const compactCard = card.replace(/\s/g, '');
    if (!/^\d{8}(?:\d{4})?$/.test(compactCard)) {
      setError('健保卡號請輸入 8 碼或完整 12 碼數字。');
      return;
    }
    const body: LoginInput = {
      identity_number: identity.trim().toUpperCase(),
      card_number: compactCard.length === 8 ? `0000${compactCard}` : compactCard,
      password,
      captcha: captcha.trim(),
      challenge_id: challenge.challenge_id,
    };
    setBusy(true);
    setError('');
    try {
      const session = await api.login(body);
      if (remember) {
        try {
          localStorage.setItem(
            REMEMBER_KEY,
            JSON.stringify({ identity: body.identity_number, card: compactCard }),
          );
        } catch {
          /* Login remains usable when browser storage is disabled. */
        }
      } else clearRememberedLogin();
      setPassword('');
      await onLogin(session);
    } catch (error) {
      setError(readableError(error));
      setPassword('');
      await loadCaptcha();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title="連接健康存摺"
      subtitle="使用您本人的健保署網路服務註冊資料登入"
      onClose={onClose}
      busy={busy}
    >
      <div className="official-service">
        <span className="service-symbol">
          <Icon name="heart" size={26} />
        </span>
        <div>
          <b>衛生福利部中央健康保險署</b>
          <span>健康存摺 · 其他檢驗資料</span>
        </div>
        <a
          href="https://myhealthbank.nhi.gov.tw/IHKE3000/IHKE3099S01"
          target="_blank"
          rel="noreferrer"
          aria-label="在新分頁開啟健保署健康存摺官網"
        >
          <Icon name="external" />
        </a>
      </div>
      <form onSubmit={(event) => { void submit(event); }} className="form-stack">
        <label className="field">
          身分證號／居留證號
          <input
            name="username"
            autoComplete="username"
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
            value={identity}
            onChange={(e) => setIdentity(e.target.value.toUpperCase())}
            maxLength={12}
            required
            placeholder="請輸入您的證號"
            disabled={busy}
          />
        </label>
        <label className="field">
          健保卡號<span className="field-helper">可直接輸入後 8 碼，自動補上前 4 碼 0000</span>
          <input
            name="health-card-number"
            autoComplete="off"
            inputMode="numeric"
            value={card}
            onChange={(e) => setCard(e.target.value.replace(/\s/g, ''))}
            pattern="[0-9]{8}([0-9]{4})?"
            maxLength={12}
            required
            placeholder="8 碼或完整 12 碼"
            disabled={busy}
          />
          {card.length === 8 && /^\d+$/.test(card) && (
            <span className="field-hint">將使用 0000 {card}</span>
          )}
        </label>
        <label className="field">
          註冊密碼
          <span className="input-with-button">
            <input
              name="password"
              type={showPassword ? 'text' : 'password'}
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              placeholder="可使用瀏覽器儲存的密碼"
              disabled={busy}
            />
            <button
              type="button"
              className="icon-button"
              aria-label={showPassword ? '隱藏密碼' : '顯示密碼'}
              aria-pressed={showPassword}
              onClick={() => setShowPassword(!showPassword)}
            >
              <Icon name={showPassword ? 'eyeOff' : 'eye'} />
            </button>
          </span>
        </label>
        <div className="field">
          <label htmlFor="captcha-input">圖形驗證碼</label>
          <div className="captcha-row">
            <div className="captcha-image">
              {captchaBusy ? (
                <Loading label="載入中" />
              ) : challenge ? (
                <img src={challenge.image_url} alt="請辨識圖片中的驗證碼，並輸入右方欄位" />
              ) : (
                <span>圖片尚未取得</span>
              )}
            </div>
            <input
              id="captcha-input"
              name="captcha"
              autoComplete="off"
              value={captcha}
              onChange={(e) => setCaptcha(e.target.value)}
              maxLength={12}
              required
              disabled={busy || !challenge}
              placeholder="手動輸入"
            />
            <button
              type="button"
              className="icon-button"
              aria-label="更換驗證碼"
              onClick={() => {
                setError('');
                void loadCaptcha();
              }}
              disabled={busy || captchaBusy}
            >
              <Icon name="refresh" />
            </button>
          </div>
        </div>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => {
              setRemember(e.target.checked);
              if (!e.target.checked) clearRememberedLogin();
            }}
            disabled={busy}
          />
          <span>在這部裝置記住證號與健保卡號</span>
        </label>
        <p className="helper-text">
          密碼由瀏覽器或 iOS
          密碼管理器處理，本程式不儲存密碼。個人基準值於登入後同步；檢驗結果只保留在目前使用階段。
        </p>
        {error && <Alert tone="error">{error}</Alert>}
        <button className="button primary full-width" type="submit" disabled={busy || !challenge}>
          {busy ? (
            <Loading label="正在連接並取得報告…" />
          ) : (
            <>
              <Icon name="login" />
              登入並取得檢驗資料
            </>
          )}
        </button>
      </form>
    </Modal>
  );
}

export function ImportDialog({
  onClose,
  onImport,
}: {
  onClose: () => void;
  onImport: (file: File) => Promise<void>;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  function acceptFile(candidate: File | undefined) {
    setError('');
    if (!candidate) return;
    if (!/\.(html?|zip)$/i.test(candidate.name)) {
      setError('請選擇官網另存的 HTML 網頁，或包含 HTML 的 ZIP 壓縮檔。');
      return;
    }
    setFile(candidate);
  }
  function drop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    if (!busy) acceptFile(event.dataTransfer.files[0]);
  }
  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file || busy) return;
    setBusy(true);
    setError('');
    try {
      await onImport(file);
    } catch (error) {
      setError(readableError(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title="匯入已下載的檢驗資料"
      subtitle="讀取健康存摺另存的網頁或 ZIP 壓縮檔"
      onClose={onClose}
      busy={busy}
    >
      <form onSubmit={(event) => { void submit(event); }} className="form-stack">
        <div
          className={`drop-zone ${dragging ? 'drag-active' : ''}`}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={drop}
        >
          <div className="drop-icon">
            <Icon name="upload" size={30} />
          </div>
          <b>{file ? file.name : '選擇檔案，或拖曳到這裡'}</b>
          <span>
            {file ? `${(file.size / 1024 / 1024).toFixed(2)} MB` : '支援 .html、.htm、.zip'}
          </span>
          <label className="button secondary file-button">
            {file ? '重新選擇檔案' : '選擇檔案'}
            <input
              type="file"
              accept=".html,.htm,.zip,text/html,application/zip"
              onChange={(e) => acceptFile(e.target.files?.[0])}
              disabled={busy}
            />
          </label>
        </div>
        <Alert>匯入的資料僅供本次瀏覽。未登入官網時，個人基準值只在目前使用階段生效。</Alert>
        <p className="helper-text">
          匯入會開啟獨立的暫存資料集，取代目前畫面。另存網頁可能只包含當時顯示的那一頁資料；系統會保留原始日期與結果，不會補造其他年度或缺漏的檢驗值。
        </p>
        {error && <Alert tone="error">{error}</Alert>}
        <button type="submit" className="button primary full-width" disabled={!file || busy}>
          {busy ? (
            <Loading label="正在整理檢驗項目…" />
          ) : (
            <>
              <Icon name="file" />
              匯入並開啟報表
            </>
          )}
        </button>
      </form>
    </Modal>
  );
}

export function DetailDialog({
  record,
  catalog,
  canConfigure,
  onClose,
  onTrend,
  onConfigure,
}: {
  record: LabRecord;
  catalog: CatalogResponse;
  canConfigure: boolean;
  onClose: () => void;
  onTrend: () => void;
  onConfigure: () => void;
}) {
  const item = catalog.items.find(
    (i) => i.id === record.item_id || i.configuration_key === record.configuration_key,
  );
  const sources = catalog.sources.filter((s) => item?.sources.includes(s.id));
  return (
    <Modal
      title={record.display_name || record.item_name}
      subtitle={`${record.item_name} · ${categoryLabel(record.category)}`}
      onClose={onClose}
      wide
    >
      <div className="detail-result">
        <div>
          <span>檢查結果</span>
          <strong>
            {record.result || '—'}{' '}
            <small>{record.unit && record.unit !== '無單位' ? record.unit : ''}</small>
          </strong>
        </div>
        <LevelBadge level={record.level} />
      </div>
      <div className="source-flag">
        <Icon name="info" />
        <b>{sourceFlag(record.reference_status)}</b>
        <span>原院參考值：{referenceLabel(record)}</span>
        {record.official_flag === '1' && (
          <strong className="official-flag-badge">原院異常標記 · 不推定高低方向</strong>
        )}
      </div>
      {record.interpretation && (
        <Alert tone={record.level && record.level >= 4 ? 'warning' : 'info'}>
          {record.interpretation}
        </Alert>
      )}
      <dl className="detail-grid">
        <div>
          <dt>檢查日期</dt>
          <dd>{dateLabel(record.exam_date)}</dd>
        </div>
        <div>
          <dt>就醫／入院日期</dt>
          <dd>{dateLabel(record.visit_date)}</dd>
        </div>
        <div>
          <dt>醫事機構</dt>
          <dd>{record.institution || '未提供'}</dd>
        </div>
        <div>
          <dt>資料來源</dt>
          <dd>{record.source || '未提供'}</dd>
        </div>
        <div>
          <dt>醫令代碼</dt>
          <dd>{record.order_code || '未提供'}</dd>
        </div>
        <div>
          <dt>原始參考值</dt>
          <dd>{record.reference_text || referenceLabel(record)}</dd>
        </div>
        <div className="detail-wide">
          <dt>檢驗檢查醫囑名稱</dt>
          <dd>{record.order_name || '未提供'}</dd>
        </div>
        <div className="detail-wide">
          <dt>顏色分級依據</dt>
          <dd>{record.rule_source || '未提供可適用的分級依據'}</dd>
        </div>
      </dl>
      {item?.description && (
        <section className="detail-notes">
          <h3>項目說明</h3>
          <p>{item.description}</p>
        </section>
      )}
      {sources.length > 0 && (
        <section className="detail-notes">
          <h3>參考來源</h3>
          <ul className="source-list">
            {sources.map((source) => (
              <li key={source.id}>
                {source.url ? (
                  <a href={source.url} target="_blank" rel="noreferrer">
                    {source.title}
                    {source.edition ? ` · ${source.edition}` : ''}
                    <Icon name="external" size={16} />
                  </a>
                ) : <span>{source.title}{source.edition ? ` · ${source.edition}` : ''}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}
      <p className="helper-text">
        顏色依目前適用規則與個人設定呈現。原始結果與原院標記始終保留；單一檢驗值不代表完整診斷。
      </p>
      <div className="dialog-actions">
        <button className="button secondary" onClick={onConfigure} disabled={!canConfigure}>
          <Icon name="settings" />
          設定此項基準
        </button>
        <button className="button primary" onClick={onTrend}>
          <Icon name="trend" />
          查看歷年趨勢
        </button>
      </div>
    </Modal>
  );
}

interface EditableBand {
  lower: string;
  upper: string;
  lower_inclusive: boolean;
  upper_inclusive: boolean;
  level: Level;
}

function RuleBandsTable({ bands }: { bands: RuleBand[] }) {
  return (
    <div className="simple-table-scroll">
      <table className="rule-table">
        <thead>
          <tr>
            <th scope="col">來源分類</th>
            <th scope="col">數值範圍</th>
            <th scope="col">顯示分級</th>
          </tr>
        </thead>
        <tbody>
          {bands.map((band, index) => (
            <tr key={index}>
              <td>{ruleCategoryLabel(band.category)}</td>
              <td>{ruleRangeLabel(band)}</td>
              <td><LevelBadge level={band.display_level} compact /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DefaultRuleSummary({
  group,
  catalog,
}: {
  group: MeasurementGroup;
  catalog: CatalogResponse;
}) {
  const item = catalog.items.find(
    (item) =>
      item.id === group.records[0]?.item_id || item.configuration_key === group.configurationKey,
  );
  if (!item) return null;
  const rule = rulePreview(item.default_rule);
  const alerts = item.alert_rules.map(rulePreview).filter((rule) => rule.bands.length > 0);
  const reference = item.default_reference;
  const sourceIds = new Set([
    ...item.sources,
    ...alerts.flatMap((alert) => alert.sources),
    ...(reference?.source ? [reference.source] : []),
  ]);
  const sources = catalog.sources.filter((source) => sourceIds.has(source.id));
  const referenceSource = catalog.sources.find((source) => source.id === reference?.source);
  return (
    <details className="rule-summary" open>
      <summary>預設規則、獨立警示與參考來源</summary>
      <div>
        <p>{item.description}</p>
        {rule.description && rule.description !== item.description && <p>{rule.description}</p>}
        {rule.kind === 'reference_interval' && (
          <p>預設採各筆原院參考值：區間內顯示 L1，超出顯示 L3，資料不足顯示未判定。</p>
        )}
        {rule.kind === 'unclassified' && <p>保留原始結果與原院標記，不為此項目強制建立顏色分級。</p>}
        {reference && (
          <section className="reference-template">
            <h3>內建參考範本</h3>
            <p><strong>{referenceTemplateLabel(reference)}</strong> {group.unit !== '無單位' && group.unit}</p>
            {reference.scope && <p>{reference.scope}</p>}
            <p>
              {reference.automatic_fallback_allowed
                ? '原院未提供可用參考值時，依此範本與適用條件判讀。'
                : '參考範本，須確認適用條件。此範本不會自動取代或補入原院參考值。'}
            </p>
            {referenceSource && <p>範本來源：{referenceSource.title}</p>}
          </section>
        )}
        {rule.kind === 'qualitative_reference' && (
          <div className="qualitative-reference">
            <p>定性結果按文字標記；加號數量不直接對應危險程度。</p>
            {rule.normalValues.length > 0 && <p><strong>L1 基準文字：</strong>{rule.normalValues.join('、')}</p>}
            {rule.positiveValues.length > 0 && <p><strong>L3 異常文字：</strong>{rule.positiveValues.join('、')}</p>}
            {rule.reviewValues.length > 0 && <p><strong>需核對方法，未直接分級：</strong>{rule.reviewValues.join('、')}</p>}
          </div>
        )}
        {rule.bands.length > 0 && (
          <>
            <p className="helper-text">
              適用單位：{rule.unit || group.unit}
              。下列顏色是本應用的顯示分級，並非所有檢驗共用的醫學危險分期；仍須確認檢體與適用條件。
            </p>
            <RuleBandsTable bands={rule.bands} />
          </>
        )}
        {alerts.length > 0 && (
          <section className="independent-alerts">
            <h3>獨立警示（個人設定不會降低）</h3>
            <p>僅在檢體、單位與適用條件可確認時套用；歷史數值的警示不代表目前已發生急症。</p>
            {alerts.map((alert, index) => (
              <div key={index}>
                {alert.description && <p>{alert.description}</p>}
                <p>適用單位：{alert.unit || group.unit}</p>
                <RuleBandsTable bands={alert.bands} />
              </div>
            ))}
          </section>
        )}
        {sources.length > 0 && (
          <ul className="source-list">
            {sources.map((source) => (
              <li key={source.id}>
                {source.url ? (
                  <a href={source.url} target="_blank" rel="noreferrer">
                    {source.title}
                    {source.edition ? ` · ${source.edition}` : ''}
                    <Icon name="external" size={15} />
                  </a>
                ) : <span>{source.title}{source.edition ? ` · ${source.edition}` : ''}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>
    </details>
  );
}

export function BaselineDialog({
  group,
  catalog,
  override,
  onSave,
  onClose,
  persisted,
}: {
  group: MeasurementGroup;
  catalog: CatalogResponse;
  override: ItemOverride | undefined;
  onSave: (key: string, value: ItemOverride | null) => Promise<void>;
  onClose: () => void;
  persisted: boolean;
}) {
  const latest = group.records[0];
  const actualValues = group.records.filter((record) => record.value_kind !== 'missing');
  const catalogItem = catalog.items.find(
    (item) => item.configuration_key === group.configurationKey,
  );
  const catalogRule = catalogItem?.default_rule;
  const isQualitativeRule =
    catalogRule !== null &&
    typeof catalogRule === 'object' &&
    (catalogRule as { kind?: string }).kind === 'qualitative_reference';
  const qualitativeOnly =
    isQualitativeRule ||
    ['text', 'descriptive', 'qualitative', 'qualitative_or_ordinal', 'ordinal', 'categorical', 'metadata']
      .includes(catalogItem?.value_kind ?? '') ||
    (actualValues.length > 0 &&
      actualValues.every((record) =>
        ['text', 'qualitative', 'ordinal', 'categorical', 'metadata'].includes(record.value_kind),
      ));
  const [enabled, setEnabled] = useState(override?.enabled ?? true);
  const [low, setLow] = useState(override?.reference_low ?? '');
  const [high, setHigh] = useState(override?.reference_high ?? '');
  const [bands, setBands] = useState<EditableBand[]>(() =>
    (override?.bands ?? []).map((band) => ({
      ...band,
      lower: band.lower ?? '',
      upper: band.upper ?? '',
    })),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  function updateBand(index: number, patch: Partial<EditableBand>) {
    setBands((current) => current.map((band, i) => (i === index ? { ...band, ...patch } : band)));
  }
  async function save(reset = false) {
    setError('');
    if (!reset) {
      const values = [low, high, ...bands.flatMap((band) => [band.lower, band.upper])];
      if (values.some((value) => value !== '' && !Number.isFinite(Number(value)))) {
        setError('基準值與區間邊界必須是數字，未設定的欄位請留白。');
        return;
      }
      if (low !== '' && high !== '' && Number(low) > Number(high)) {
        setError('基準下限不可大於上限。');
        return;
      }
      if (
        bands.some(
          (band) =>
            band.lower !== '' &&
            band.upper !== '' &&
            (Number(band.lower) > Number(band.upper) ||
              (Number(band.lower) === Number(band.upper) &&
                !(band.lower_inclusive && band.upper_inclusive))),
        )
      ) {
        setError('有一個分級區間沒有可包含的數值，請檢查上下界與是否包含邊界。');
        return;
      }
      for (const [i, a] of bands.entries()) {
        for (const [j, b] of bands.entries()) {
          if (j <= i) continue;
          const aLow = a.lower === '' ? -Infinity : Number(a.lower),
            aHigh = a.upper === '' ? Infinity : Number(a.upper);
          const bLow = b.lower === '' ? -Infinity : Number(b.lower),
            bHigh = b.upper === '' ? Infinity : Number(b.upper);
          const start = Math.max(aLow, bLow),
            end = Math.min(aHigh, bHigh);
          const overlaps =
            start < end ||
            (start === end &&
              (start !== aLow || a.lower_inclusive) &&
              (start !== bLow || b.lower_inclusive) &&
              (end !== aHigh || a.upper_inclusive) &&
              (end !== bHigh || b.upper_inclusive));
          if (overlaps) {
            setError(`第 ${i + 1} 與第 ${j + 1} 個分級區間重疊，請調整後再儲存。`);
            return;
          }
        }
      }
    }
    setBusy(true);
    try {
      const value: ItemOverride | null = reset || (low === '' && high === '' && bands.length === 0)
        ? null
        : {
            enabled,
            ...(low !== '' ? { reference_low: low } : {}),
            ...(high !== '' ? { reference_high: high } : {}),
            bands: bands.map(
              (band): LevelBand => ({
                ...band,
                lower: band.lower === '' ? null : band.lower,
                upper: band.upper === '' ? null : band.upper,
              }),
            ),
          };
      await onSave(group.configurationKey, value);
      onClose();
    } catch (error) {
      setError(readableError(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title="個人基準與顏色分級"
      subtitle={`${group.name} · ${group.unit}`}
      onClose={onClose}
      wide
      busy={busy}
    >
      <div className="baseline-source">
        <span>最近一次原院參考值</span>
        <b>
          {latest ? referenceLabel(latest) : '本次尚未取得報告'} <small>{group.unit}</small>
        </b>
        <span>
          {latest
            ? `${latest.institution || '來源未提供'} · ${dateLabel(latest.exam_date)}`
            : catalogItem?.builtin
              ? '內建項目，可先檢視預設規則並調整個人基準。'
              : '此項目來自先前取得的檢驗項目設定，未儲存歷史結果。'}
        </span>
      </div>
      <p className="helper-text">
        此設定僅適用於「{group.name}」的相同檢體與單位。調整不會改寫原院參考值或原始結果。
        {persisted
          ? '設定會同步至您登入的其他裝置。'
          : '目前未經官網登入，設定只在本次使用階段生效。'}
      </p>
      <DefaultRuleSummary group={group} catalog={catalog} />
      <p className="baseline-mode">
        {override?.enabled
          ? '目前使用個人設定；還原預設規則會移除此項目的個人調整。'
          : override
            ? '目前個人設定已停用，正使用預設規則。'
            : '目前使用預設規則；下方欄位僅供建立個人調整，空白不代表缺少內建基準。'}
      </p>
      {qualitativeOnly && (
        <Alert>
          此項目目前為定性、加號或文字結果，數值上下限不適用。系統保留原始文字與適用的定性標記，不把加號換算成危險程度。
        </Alert>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
        className="form-stack"
      >
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            disabled={busy || qualitativeOnly}
          />
          <span>啟用這個項目的個人設定</span>
        </label>
        <fieldset disabled={busy || !enabled || qualitativeOnly}>
          <legend>個人參考區間</legend>
          <p className="helper-text">兩欄皆留白時沿用預設規則；只填一欄時，另一側不設邊界。</p>
          <div className="two-column-fields">
            <label className="field">
              下限
              <input
                type="number"
                inputMode="decimal"
                step="any"
                value={low}
                onChange={(event) => setLow(event.target.value)}
                placeholder="不設下限"
              />
            </label>
            <label className="field">
              上限
              <input
                type="number"
                inputMode="decimal"
                step="any"
                value={high}
                onChange={(event) => setHigh(event.target.value)}
                placeholder="不設上限"
              />
            </label>
          </div>
        </fieldset>
        <fieldset disabled={busy || !enabled || qualitativeOnly}>
          <legend>
            自訂顏色區間 <span className="optional-label">選填</span>
          </legend>
          <p className="helper-text">
            上下界留白代表無限延伸。區間不可重疊；不必湊滿五級。建立自訂區間後，未涵蓋或跨越多個區間的結果顯示「未判定」。適用的獨立警示仍會保留。
          </p>
          <div className="band-list">
            {bands.map((band, index) => (
              <div className="band-editor" key={index}>
                <div className="band-title">
                  <b>區間 {index + 1}</b>
                  <button
                    type="button"
                    className="icon-button danger-quiet"
                    onClick={() => setBands(bands.filter((_, i) => i !== index))}
                    aria-label={`移除區間 ${index + 1}`}
                  >
                    <Icon name="trash" size={18} />
                  </button>
                </div>
                <label className="field">
                  分級
                  <select
                    value={band.level}
                    onChange={(event) =>
                      updateBand(index, { level: Number(event.target.value) as Level })
                    }
                  >
                    {([1, 2, 3, 4, 5] as Level[]).map((level) => (
                      <option key={level} value={level}>
                        L{level} · {LEVELS[level].label}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="two-column-fields">
                  <div>
                    <label className="field">
                      下界
                      <input
                        type="number"
                        step="any"
                        inputMode="decimal"
                        value={band.lower}
                        placeholder="無下界"
                        onChange={(event) => updateBand(index, { lower: event.target.value })}
                      />
                    </label>
                    <label className="checkbox-label small">
                      <input
                        type="checkbox"
                        checked={band.lower_inclusive}
                        onChange={(event) =>
                          updateBand(index, { lower_inclusive: event.target.checked })
                        }
                      />
                      包含下界
                    </label>
                  </div>
                  <div>
                    <label className="field">
                      上界
                      <input
                        type="number"
                        step="any"
                        inputMode="decimal"
                        value={band.upper}
                        placeholder="無上界"
                        onChange={(event) => updateBand(index, { upper: event.target.value })}
                      />
                    </label>
                    <label className="checkbox-label small">
                      <input
                        type="checkbox"
                        checked={band.upper_inclusive}
                        onChange={(event) =>
                          updateBand(index, { upper_inclusive: event.target.checked })
                        }
                      />
                      包含上界
                    </label>
                  </div>
                </div>
              </div>
            ))}
          </div>
          <button
            type="button"
            className="button secondary"
            onClick={() =>
              setBands([
                ...bands,
                { lower: '', upper: '', lower_inclusive: true, upper_inclusive: false, level: 3 },
              ])
            }
            disabled={bands.length >= 16}
          >
            <Icon name="plus" />
            新增分級區間
          </button>
        </fieldset>
        {error && <Alert tone="error">{error}</Alert>}
        <div className="dialog-actions">
          <button
            type="button"
            className="button quiet"
            onClick={() => void save(true)}
            disabled={busy || (!override && low === '' && high === '' && bands.length === 0)}
          >
            還原預設規則
          </button>
          <button className="button primary" type="submit" disabled={busy || qualitativeOnly}>
            {busy ? (
              <Loading label="儲存中…" />
            ) : (
              <>
                <Icon name="check" />
                儲存並更新顏色
              </>
            )}
          </button>
        </div>
      </form>
    </Modal>
  );
}
