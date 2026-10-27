// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useState } from 'react';
import { api, readableError, type PrivacyInfo } from './api';
import { Modal } from './ui';
export function PrivacyNotice() {
  const [open, setOpen] = useState(false);
  const [information, setInformation] = useState<PrivacyInfo | null>(null);
  const [error, setError] = useState('');
  async function show() { setOpen(true); setError(''); try { setInformation(await api.privacy()); } catch (failure) { setError(readableError(failure)); } }
  return <>
    <button type="button" className="button secondary" onClick={() => void show()}>隱私與資料處理</button>
    {open && <Modal title="隱私與資料處理" subtitle="軟體行為與實際部署環境分開說明" onClose={() => setOpen(false)}>
      <div className="form-stack">
        {error && <p role="alert">營運資訊載入失敗：{error}</p>}
        {information && <section><h3>本部署營運資訊</h3><p>營運者：{information.operator || '尚未設定，請部署者補齊'}</p><p>權利行使／聯絡：{information.contact || '尚未設定'}</p><p>處理地區：{information.processing_region || '尚未確認'}</p><p>備份保存期限：{information.backup_retention || '尚未設定，不宣稱零備份'}</p><p>{information.settings_retention}</p></section>}
        <p>Healthbank 為獨立開源資料整理工具，非健保署官方服務，不取代醫療專業判斷。</p>
        <p>應用程式不將原始證號、卡號、密碼、官方 Cookie／權杖、檢驗結果寫入資料庫或報告檔案。登入與查詢仍會經過伺服器記憶體，僅用於當次官方認證、查詢、整理與比較。</p>
        <p>為跨裝置同步設定，資料庫保存經官方驗證後產生的 HMAC 假名化識別碼、介面偏好、關注項目、檢驗項目定義、個人基準、選擇同步的判讀情境與設定版本。這些資料可連結同一使用者，不是完全匿名。</p>
        <p>本機記住的證號／卡號由您選擇保存及清除；密碼僅由您使用的瀏覽器或系統密碼管理員保存，可能依該產品設定同步。網站清除按鈕不會刪除密碼管理員的密碼。</p>
        <p>年齡／性別比較在前端即時運算；只有按「儲存判讀情境」才將明列的情境設定同步至伺服器。選填生日只留在當頁，不上傳或持久保存。選配個人品項紀錄可由您選擇留在此瀏覽器，不傳送伺服器。自行下載的檔案由您保管。應用程式沒有廣告追蹤、第三方分析或模型訓練上傳功能。</p>
        <p>官方工作階段與報告於登出、服務停止，或閒置超過30分鐘後釋放；週期清理每分鐘執行。關閉分頁不等於伺服器立即清除。記憶體釋放不代表底層位元已經不可恢復。</p>
        <p>反向代理、CDN、主機記錄與備份由實際營運者管理。營運者須另行明示名稱、聯絡方式、處理地區、保存及刪除程序；未查核該部署前，不承諾整個環境絕對零留存。</p>
        <p>原始碼採 MIT License；第三方資料及元件依各自權利規範。使用者資料不因本程式開源而成為公開資料。相關說明不排除依法不得排除的責任或權利。</p>
      </div>
    </Modal>}
  </>;
}
