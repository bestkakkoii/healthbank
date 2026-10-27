// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestDiagnosticRequiresCompleteRasterDecode(t *testing.T) {
	var encoded bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 160, 60))
	picture.Set(0, 0, color.RGBA{R: 30, G: 80, B: 110, A: 255})
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	complete := encoded.Bytes()
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"complete_png", complete, true},
		{"only_png_header", complete[:33], false},
		{"truncated_png", complete[:len(complete)-20], false},
		{"html_instead_of_image", []byte("<!doctype html><title>Unavailable</title>"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := nhi.InspectChallengeImage(tc.data)
			if tc.valid {
				if err != nil || result == nil || result.Width != 160 || result.Height != 60 || result.Format != "png" || result.Bytes != len(complete) {
					t.Fatalf("valid image was not measured: %#v, %v", result, err)
				}
				return
			}
			var failure *nhi.Error
			if result != nil || !errors.As(err, &failure) || failure.Code != "CAPTCHA_INVALID_IMAGE" {
				t.Fatal("invalid image was reported as a successful CAPTCHA")
			}
		})
	}
}
