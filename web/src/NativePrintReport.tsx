// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useLayoutEffect } from 'react';
import type { ReportResponse, SessionMode } from './api';
import type { DataCategory, HealthDataSnapshot } from './health-data';
import { createPrintReport, DEFAULT_PRINT_OPTIONS, PRINT_REPORT_STYLE, renderPrintReport } from './print-report';

// Browser-menu printing does not pass through our toolbar/keyboard handler.
// Prepare a separate data-generated document synchronously in beforeprint; keep
// the React workspace mounted so cancelling print does not reset its context.
export function NativePrintReport({ report, snapshot, categories, mode, version, reportOpen }: {
  report: ReportResponse; snapshot: HealthDataSnapshot; categories: DataCategory[];
  mode: SessionMode; version: string; reportOpen: boolean;
}) {
  useLayoutEffect(() => {
    let host: HTMLDivElement | null = null;
    const clear = () => { host?.remove(); host = null; };
    const prepare = () => {
      clear();
      host = document.createElement('div');
      host.id = 'healthbank-native-print';
      host.style.display = 'none';
      const style = document.createElement('style');
      style.media = 'print';
      style.textContent = `${PRINT_REPORT_STYLE}\n@media print {
        body > :not(#healthbank-native-print) { display: none !important; }
        #healthbank-native-print { display: block !important; }
        html, body { height: auto !important; min-height: 0 !important; overflow: visible !important; }
      }`;
      host.append(style);
      try {
        // srcDoc is our escaped, script-free document, never source-provided HTML.
        // An open sharing dialog must use its exact scope, even for native print.
        const selected = reportOpen ? document.querySelector<HTMLIFrameElement>('iframe.report-preview')?.srcdoc : undefined;
        if (reportOpen && !selected) throw new Error('No prepared sharing document');
        const html = selected ?? renderPrintReport(createPrintReport({ report, snapshot, categories,
          selectedLaboratory: report.records, mode, version, generatedAt: new Date().toISOString(), options: DEFAULT_PRINT_OPTIONS }));
        const prepared = new DOMParser().parseFromString(html, 'text/html');
        for (const node of Array.from(prepared.body.childNodes)) host.append(node);
      } catch {
        // Never silently fall back to a viewport print or broaden a failed scope.
        const message = document.createElement('p');
        message.textContent = '專用報表尚未準備完成。請取消列印，回到「製作列印／PDF 報表」確認範圍後重試。Copyright © 2026 bestkakkoii llc.';
        host.append(message);
      }
      document.body.append(host);
    };
    window.addEventListener('beforeprint', prepare);
    window.addEventListener('afterprint', clear);
    return () => {
      window.removeEventListener('beforeprint', prepare);
      window.removeEventListener('afterprint', clear);
      clear();
    };
  }, [report, snapshot, categories, mode, version, reportOpen]);
  return null;
}
