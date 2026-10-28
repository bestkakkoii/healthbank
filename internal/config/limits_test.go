// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package config

import "testing"

func TestResourceLimitsRejectUnsafeOrIncompatibleOverrides(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"HEALTHBANK_MAX_SESSIONS", "0"}, {"HEALTHBANK_MAX_REQUESTS", "999999999999999999999"},
		{"HEALTHBANK_SESSION_QUEUE", "-1"}, {"HEALTHBANK_HEAVY_REQUESTS", "0"},
		{"HEALTHBANK_ANONYMOUS_SESSIONS", "64"}, {"HEALTHBANK_WORKING_MEMORY_MIB", "191"},
		{"HEALTHBANK_RETAINED_MEMORY_MIB", "32"}, {"HEALTHBANK_SESSION_MEMORY_MIB", "512"},
		{"HEALTHBANK_MAX_CONNECTIONS", "1"}, {"HEALTHBANK_WORKING_MEMORY_MIB", "1.5"},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if _, err := LoadResourceLimits(); err == nil {
				t.Fatal("unsafe resource override accepted")
			}
		})
	}
	if err := DefaultResourceLimits().Validate(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEALTHBANK_MAX_REQUESTS", "8")
	t.Setenv("HEALTHBANK_SESSION_QUEUE", "0")
	t.Setenv("HEALTHBANK_HEAVY_REQUESTS", "1")
	got, err := LoadResourceLimits()
	if err != nil || got.Requests != 8 || got.SessionQueue != 0 || got.HeavyRequests != 1 {
		t.Fatal("valid bounded operator limits were not applied")
	}
}
