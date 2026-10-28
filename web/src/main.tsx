// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT
import { StrictMode, Component, type ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import { COPYRIGHT_YEAR } from './model';
import './styles.css';

class ApplicationBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override render() {
    if (this.state.failed)
      return (
        <main className="fatal-screen">
          <h1>頁面暫時無法顯示</h1>
          <p>請重新整理頁面。尚未匯出的報告可能需要重新取得。</p>
          <p>錯誤代碼：APPLICATION_RENDER_FAILED</p>
          <button className="button primary" onClick={() => window.location.reload()}>
            重新開啟工作站
          </button>
          <small>© {COPYRIGHT_YEAR} bestkakkoii llc.</small>
        </main>
      );
    return this.props.children;
  }
}

const root = document.getElementById('root');
if (!root) throw new Error('Application root element is missing.');
createRoot(root, {
  // React's default handlers may print the original Error and component stack.
  // Keep failures traceable without logging input, medical state or credentials.
  onCaughtError: () => console.error('APPLICATION_RENDER_FAILED'),
  onUncaughtError: () => console.error('APPLICATION_UNCAUGHT_RENDER_FAILED'),
  onRecoverableError: () => console.warn('APPLICATION_RENDER_RECOVERED'),
}).render(
  <StrictMode>
    <ApplicationBoundary>
      <App />
    </ApplicationBoundary>
  </StrictMode>,
);
