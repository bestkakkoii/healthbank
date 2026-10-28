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
        <p>應用程式不將原始證號、卡號、密碼、官方 Cookie／權杖、檢驗、用藥、就醫或原始附件持久寫入伺服器資料庫或報告檔案。登入與查詢仍會經過伺服器記憶體，僅用於當次官方認證、查詢、整理、比較與明確要求的下載。</p>
        <p>為跨裝置同步設定，資料庫保存經官方驗證後產生的 HMAC 假名化識別碼、介面偏好、關注項目、檢驗項目定義、個人基準、選擇同步的判讀情境與設定版本。這些資料可連結同一使用者，不是完全匿名。</p>
        <p>本機記住的證號／卡號由您選擇保存及清除；密碼僅由您使用的瀏覽器或系統密碼管理員保存，可能依該產品設定同步。網站清除按鈕不會刪除密碼管理員的密碼。</p>
        <p>裝置健康資料快取須另外明確同意，才會把已取得的檢驗、醫療清單、明細與已保留的小型附件存入此網站的 IndexedDB。每位使用者以官方驗證後的假名化識別分開，之後須再次完成官方登入才會開啟。這是裝置上的網站儲存，不是加密保險箱；請只在可信任的私人裝置啟用。</p>
        <p>快取不保存密碼、官方 Cookie／權杖或可重用的下載權限。還原檢驗時會送至本站伺服器記憶體重新判讀，不寫入設定資料庫；畫面會保留原始取得時間與快取來源，不當成剛向官方核對的資料。清除單人或全部健康快取會撤回相應保存同意；重新取得會先清除該使用者舊快取與目前工作區，再開始抓取。</p>
        <p>年齡／性別比較在前端即時運算；只有按「儲存判讀情境」才將明列的情境設定同步至伺服器。選填生日只留在當頁，不上傳或持久保存。選配個人品項紀錄可由您選擇留在此瀏覽器，不傳送伺服器。自行下載的檔案由您保管。應用程式沒有廣告追蹤、第三方分析或模型訓練上傳功能。</p>
        <p>官方工作階段與報告於登出、服務停止，或閒置超過30分鐘後釋放；週期清理每分鐘執行。關閉分頁不等於伺服器立即清除。記憶體釋放不代表底層位元已經不可恢復。</p>
        <p>完整資料包與您明確加入的附件在目前頁面記憶體整理，選擇裝置快取後依上述方式保存，下載檔則由您保管；從檔案匯入不證明所屬身分，會使用獨立匯入模式。需要健保署產製原始檔時，您明確按下產製才建立官方暫存工作；官方保存與刪除結果不由本站保證。</p>
        <p>本站的清除功能移除應用管理的快取及引用，並阻止延遲操作把已清除的內容寫回；無法抹除您另存的下載檔、密碼管理員、瀏覽器／作業系統備份或底層儲存殘留。瀏覽器也可能因清理網站資料或容量限制而移除快取，長期保存請另行匯出。</p>
        <p>反向代理、CDN、主機記錄與備份由實際營運者管理。營運者須另行明示名稱、聯絡方式、處理地區、保存及刪除程序；未查核該部署前，不承諾整個環境絕對零留存。</p>
        <p>原始碼採 MIT License；第三方資料及元件依各自權利規範。使用者資料不因本程式開源而成為公開資料。相關說明不排除依法不得排除的責任或權利。</p>
      </div>
    </Modal>}
  </>;
}
