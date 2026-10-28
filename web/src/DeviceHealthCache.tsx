// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { useDeviceHealthCache } from './use-device-health-cache';
import { formatTime } from './model';
import { Alert } from './ui';

export function DeviceHealthCache({ cache, disabled }: { cache: ReturnType<typeof useDeviceHealthCache>; disabled: boolean }) {
  const metadata = cache.state.metadata;
  return <details className="panel compact-disclosure device-health-cache" id="device-health-cache">
    <summary>裝置健康資料快取 <span className="small">{cache.state.enabled ? metadata ? '已保存' : '已同意，尚未保存資料' : '未啟用'}</span></summary>
    <p>健康資料只在明確同意後保存在這部裝置；每位使用者依本次官方驗證身分分開。密碼、登入憑證及官方下載權限不會存入快取。</p>
    <p className="small">僅在自己可控的私人裝置啟用。資料以未加密的瀏覽器網站儲存空間保存；假名化索引不是加密，共用瀏覽器設定檔或可存取裝置的人仍可能讀取資料。</p>
    <label className="checkbox-label"><input type="checkbox" checked={cache.state.enabled} disabled={!cache.official || disabled || cache.busy}
      onChange={event => { if (event.target.checked) void cache.enable(); else void cache.clear(); }} />
      我同意此裝置保存目前使用者已取得的健康資料，供之後登入直接開啟；取消同意會清除快取與工作區。</label>
    {!cache.official && <p className="small">完成官方登入後，才會開啟或啟用這位使用者的快取。匯入檔案與合成示範不會寫入個人快取。</p>}
    {metadata && <p className="small">保存於 {formatTime(metadata.saved_at)}；{metadata.records} 筆檢驗、{metadata.sources} 個已取得來源，約 {(metadata.size / 1024 ** 2).toFixed(2)} MiB。原始取得時間與完整性仍依各來源記錄。</p>}
    <div className="button-row">
      {cache.official && <button type="button" className="button secondary" disabled={disabled || cache.busy} onClick={() => { void cache.refresh(); }}>清除舊資料並重新取得全部來源</button>}
      {cache.official && <button type="button" className="button secondary" disabled={disabled || cache.busy} onClick={() => { void cache.clear(); }}>清除此使用者快取</button>}
      <button type="button" className="button secondary" disabled={disabled || cache.busy} onClick={() => { void cache.clear(true); }}>清除此裝置所有健康快取</button>
      {cache.phase === 'refreshing' && <button type="button" className="button secondary" onClick={cache.cancel}>停止重新取得</button>}
    </div>
    {cache.notice && <p role="status">{cache.notice}</p>}{cache.error && <Alert tone="error">{cache.error}</Alert>}
    <p className="small">重新取得會先移除舊資料，再查詢檢驗、各來源清單及明細；原始檔產製／下載由完整取得流程另行處理。快取包含已保留的小型附件，大型原始檔請另存 ZIP。瀏覽器清除網站資料或儲存空間不足時，快取可能無法保留；長期保存請另外匯出資料包。</p>
  </details>;
}
