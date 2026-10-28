// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package store

import "context"

// Ready checks connectivity and the required settings table without reading any
// profile or creating a synthetic configuration row in the real database.
func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT revision FROM profile_settings WHERE 1 = 0`)
	if err != nil {
		return err
	}
	return rows.Close()
}
