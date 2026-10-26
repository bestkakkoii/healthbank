// Copyright (c) 2026 bestkakkoi llc. All rights reserved.
// SPDX-License-Identifier: MIT
import { StrictMode, Component, type ErrorInfo, type ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import './styles.css';

class ApplicationBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(_error: Error, _info: ErrorInfo) {
    /* Do not transmit medical or credential-bearing state to logging services. */
  }
  render() {
    if (this.state.failed)
      return (
        <main className="fatal-screen">
          <h1>頁面暫時無法顯示</h1>
          <p>請重新整理頁面。尚未匯出的報告可能需要重新取得。</p>
          <button className="button primary" onClick={() => window.location.reload()}>
            重新開啟工作站
          </button>
          <small>© {new Date().getFullYear()} bestkakkoi llc. All rights reserved.</small>
        </main>
      );
    return this.props.children;
  }
}

const root = document.getElementById('root');
if (!root) throw new Error('Application root element is missing.');
createRoot(root).render(
  <StrictMode>
    <ApplicationBoundary>
      <App />
    </ApplicationBoundary>
  </StrictMode>,
);
