// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { AcquisitionRestoreSelection } from './acquisition-restore';
import type { BatchCollectionSelection } from './batch-collection';

// File handles belong only to this page lifetime. An unrelated session change
// clears this context; an explicitly restored member carries it to its new view.
export type CollectionRestoration = AcquisitionRestoreSelection | BatchCollectionSelection;
