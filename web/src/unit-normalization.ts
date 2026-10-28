// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// UCUM 2.2 allows l/L for liter, but m/M and other prefixes remain distinct.
// This finite spelling table is not a general UCUM parser or unit conversion.
const aliases: Readonly<Record<string, string>> = {
  'mg/dl': 'mg/dL', 'g/dl': 'g/dL', 'mg/l': 'mg/L', 'ng/l': 'ng/L',
  'ng/ml': 'ng/mL', 'pg/ml': 'pg/mL', fl: 'fL', 'U/l': 'U/L',
  'mmol/l': 'mmol/L', 'umol/l': 'umol/L',
  'x10^3/ul': 'x10^3/uL', 'x10^6/ul': 'x10^6/uL',
  'ml/min/1.73m2': 'mL/min/1.73m2',
  'ml/min/1.73m^2': 'mL/min/1.73m2', 'mL/min/1.73m^2': 'mL/min/1.73m2',
};
export function canonicalUnit(value: string): string {
  const spelling = value.trim().replaceAll('μ', 'u').replaceAll('µ', 'u')
    .replaceAll('²', '2').replaceAll('³', '3').replaceAll(' ', '').replaceAll('／', '/');
  return Object.hasOwn(aliases, spelling) ? aliases[spelling] ?? spelling : spelling;
}
