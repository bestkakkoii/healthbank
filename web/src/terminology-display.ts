// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { CatalogItem, CatalogResponse, LabRecord, SourceNameTerminology } from './api';
import type { MeasurementGroup } from './model';

/** Display indexes never participate in grouping, clinical matching or export. */
export interface TerminologyIndex {
  readonly byKey: ReadonlyMap<string, CatalogItem>;
  readonly byID: ReadonlyMap<string, CatalogItem>;
  readonly bySourceName: ReadonlyMap<string, SourceNameTerminology>;
}

export interface TerminologyDisplay {
  name: string;
  englishName: string;
  originalNames: string[];
  note: string;
  sourceIds: string[];
  basis: 'catalog' | 'source_name' | 'raw';
}

export function createTerminologyIndex(
  catalog: Pick<CatalogResponse, 'items' | 'sources' | 'source_name_glossary'>,
): TerminologyIndex {
  const byKey = new Map<string, CatalogItem>();
  const byID = new Map<string, CatalogItem>();
  const bySourceName = new Map<string, SourceNameTerminology>();
  const sourceIds = new Set(catalog.sources.map(source => source.id));
  for (const item of catalog.items) {
    if (item.configuration_key) byKey.set(item.configuration_key, item);
    if (item.id) byID.set(item.id, item);
  }
  for (const term of catalog.source_name_glossary ?? []) {
    const attributed = term.terminology_basis === 'primary_source_terms'
      && Boolean(term.terminology_sources?.length)
      && term.terminology_sources?.every(id => sourceIds.has(id));
    const literal = term.terminology_basis === 'literal_source_translation'
      && Boolean(term.terminology_note?.trim());
    if (!term.display_name || !term.display_name_en || (!attributed && !literal)) continue;
    for (const name of term.source_names) {
      if (name && name === name.trim()) bySourceName.set(name, term);
    }
  }
  return { byKey, byID, bySourceName };
}

function catalogItem(record: LabRecord, index?: TerminologyIndex): CatalogItem | undefined {
  if (!index || record.value_kind === 'metadata' || record.category === 'metadata') return undefined;
  return (record.configuration_key ? index.byKey.get(record.configuration_key) : undefined)
    ?? (record.item_id ? index.byID.get(record.item_id) : undefined);
}

function uniqueText(values: readonly string[]): string[] {
  return [...new Set(values.filter(value => value !== ''))];
}

export function recordTerminology(record: LabRecord, index?: TerminologyIndex): TerminologyDisplay {
  const raw: TerminologyDisplay = {
    name: record.display_name || record.item_name,
    englishName: '', originalNames: record.item_name ? [record.item_name] : [],
    note: '', sourceIds: [], basis: 'raw',
  };
  const item = catalogItem(record, index);
  if (item?.display_name_en) {
    return {
      ...raw,
      // Shared ACR/protein settings do not replace a channel-specific Chinese
      // record name with the other channel's name. Exact IDs may use corrected
      // current names without modifying the archived/source record itself.
      name: !item.builtin || item.id === record.item_id ? item.display_name || raw.name : raw.name,
      englishName: item.display_name_en, note: item.terminology_note ?? '',
      sourceIds: [...(item.terminology_sources ?? [])], basis: 'catalog',
    };
  }
  // Only an explicitly reviewed spelling can describe metadata, missing-only
  // rows and other source names absent from settings. No case-folded lookup,
  // billing-code expansion or approximate alias matching is performed here.
  const term = index?.bySourceName.get(record.item_name.trim());
  if (!term) return raw;
  return {
    ...raw, name: term.display_name, englishName: term.display_name_en,
    note: term.terminology_note ?? '', sourceIds: [...(term.terminology_sources ?? [])], basis: 'source_name',
  };
}

export function groupTerminology(group: MeasurementGroup, index?: TerminologyIndex): TerminologyDisplay {
  const first = group.records[0];
  if (first) {
    return { ...recordTerminology(first, index), originalNames: uniqueText(group.records.map(record => record.item_name)) };
  }
  const item = index?.byKey.get(group.configurationKey);
  return {
    name: group.name,
    englishName: item?.display_name_en ?? '', originalNames: [],
    note: item?.terminology_note ?? '', sourceIds: [...(item?.terminology_sources ?? [])],
    basis: item?.display_name_en ? 'catalog' : 'raw',
  };
}

/** Case folding is for text search only and never changes an item identity. */
export function groupTerminologySearchText(group: MeasurementGroup, index?: TerminologyIndex): string {
  const display = groupTerminology(group, index);
  const item = index?.byKey.get(group.configurationKey);
  const values = [group.name, group.abbreviation, group.unit, display.name, display.englishName,
    ...(item?.aliases ?? []), ...(item?.order_codes ?? [])];
  for (const record of group.records) {
    const names = recordTerminology(record, index);
    values.push(record.item_name, record.display_name, record.order_name, record.order_code, names.name, names.englishName);
  }
  return uniqueText(values).join(' ').toLocaleLowerCase();
}
