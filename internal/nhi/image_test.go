// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestChallengeRejectsUndecodableRasterBeforeIssuingIdentifier(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 160, 60))); err != nil {
		t.Fatal(err)
	}
	complete := encoded.Bytes()
	var oversized bytes.Buffer
	if err := png.Encode(&oversized, image.NewRGBA(image.Rect(0, 0, 2049, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"only_png_header", complete[:33]},
		{"truncated_png", complete[:len(complete)-20]},
		{"oversized_dimensions", oversized.Bytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Close)
			// Only the image step is under test; this transport cannot access a
			// network, read credentials or send any fixture to the official host.
			session.bootstrapped = true
			session.challengeID = "synthetic-old-challenge"
			session.challengeExpiry = time.Now().Add(time.Minute)
			calls := 0
			session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != captchaPath || r.Method != http.MethodGet {
					t.Fatal("unexpected request in synthetic image fixture")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(tc.body)), Request: r}, nil
			})
			challenge, err := session.Challenge(context.Background())
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != "CAPTCHA_INVALID_IMAGE" {
				t.Fatal("undecodable or oversized image was issued as a usable CAPTCHA")
			}
			if calls != 1 || session.challengeID != "" || challenge.ID != "" || len(challenge.Image) != 0 {
				t.Fatal("failed CAPTCHA retained challenge state or was retried")
			}
		})
	}
}
