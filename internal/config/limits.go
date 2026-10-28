// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ResourceLimits are admission limits, not a promise of an exact process RSS.
// Working estimates and retained-data accounting exclude runtime/proxy overhead.
type ResourceLimits struct {
	Sessions          int
	AnonymousSessions int
	Requests          int
	SessionQueue      int
	HeavyRequests     int
	Connections       int
	WorkingBytes      int64
	RetainedBytes     int64
	SessionBytes      int64
}

// DefaultResourceLimits leaves finite headroom for a small self-hosted instance.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{Sessions: 64, AnonymousSessions: 16, Requests: 32, SessionQueue: 4, HeavyRequests: 2, Connections: 128, WorkingBytes: 256 << 20, RetainedBytes: 256 << 20, SessionBytes: 64 << 20}
}

// LoadResourceLimits reads bounded operator overrides; malformed values fail startup.
func LoadResourceLimits() (ResourceLimits, error) {
	limits := DefaultResourceLimits()
	for _, option := range []struct {
		name             string
		value            *int
		minimum, maximum int
	}{
		{"HEALTHBANK_MAX_SESSIONS", &limits.Sessions, 2, 256},
		{"HEALTHBANK_ANONYMOUS_SESSIONS", &limits.AnonymousSessions, 1, 64},
		{"HEALTHBANK_MAX_REQUESTS", &limits.Requests, 1, 1024},
		{"HEALTHBANK_SESSION_QUEUE", &limits.SessionQueue, 0, 16},
		{"HEALTHBANK_HEAVY_REQUESTS", &limits.HeavyRequests, 1, 8},
		{"HEALTHBANK_MAX_CONNECTIONS", &limits.Connections, 16, 4096},
	} {
		if raw := strings.TrimSpace(os.Getenv(option.name)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < option.minimum || value > option.maximum {
				return limits, fmt.Errorf("%s must be an integer within %d–%d", option.name, option.minimum, option.maximum)
			}
			*option.value = value
		}
	}
	for _, option := range []struct {
		name    string
		value   *int64
		minimum int64
	}{
		{"HEALTHBANK_WORKING_MEMORY_MIB", &limits.WorkingBytes, 192},
		{"HEALTHBANK_RETAINED_MEMORY_MIB", &limits.RetainedBytes, 16},
		{"HEALTHBANK_SESSION_MEMORY_MIB", &limits.SessionBytes, 4},
	} {
		if raw := strings.TrimSpace(os.Getenv(option.name)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < option.minimum || value > 4096 {
				return limits, fmt.Errorf("%s must be an integer within %d–4096 MiB", option.name, option.minimum)
			}
			*option.value = value << 20
		}
	}
	if err := limits.Validate(); err != nil {
		return limits, err
	}
	return limits, nil
}

// Validate rejects combinations that cannot admit the supported bounded operations.
func (limits ResourceLimits) Validate() error {
	if limits.Sessions < 2 || limits.Sessions > 256 || limits.AnonymousSessions < 1 || limits.AnonymousSessions >= limits.Sessions || limits.AnonymousSessions > 64 {
		return fmt.Errorf("anonymous session allowance must be positive and smaller than the total session allowance")
	}
	if limits.Requests < 1 || limits.Requests > 1024 || limits.SessionQueue < 0 || limits.SessionQueue > 16 || limits.HeavyRequests < 1 || limits.HeavyRequests > 8 || limits.HeavyRequests > limits.Requests || limits.Connections < 16 || limits.Connections > 4096 {
		return fmt.Errorf("request, queue, connection and heavy-operation allowances are incompatible")
	}
	if limits.WorkingBytes < 192<<20 || limits.WorkingBytes > 4096<<20 || limits.RetainedBytes < 16<<20 || limits.RetainedBytes > 4096<<20 || limits.SessionBytes < 4<<20 || limits.SessionBytes > limits.RetainedBytes {
		return fmt.Errorf("working and retained admission budgets are incompatible")
	}
	return nil
}
