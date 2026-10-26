// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

// This command deliberately runs before configuration, database and instance
// locking. It can inspect a deployment's outbound path beside a running server.
// It never accepts credentials or an arbitrary upstream URL.
func diagnoseNHI(out io.Writer) error {
	started := time.Now()
	result := diagnosticResult{
		Version: version, Commit: commit, GoVersion: runtime.Version(),
		StartedUTC: started.UTC().Format(time.RFC3339),
		Operation:  "login_document_then_captcha", Attempts: 1,
		Copyright: "Copyright (c) 2026 bestkakkoii llc.",
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	connector, err := nhi.New()
	if err == nil {
		defer connector.Close()
		var challenge nhi.Challenge
		challenge, err = connector.Challenge(ctx)
		result.Diagnostics = connector.Snapshot()
		if err == nil {
			result.Image, err = inspectChallenge(challenge.Image)
			clear(challenge.Image)
			result.ImageDecoded = err == nil
		}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = &diagnosticFailure{Code: "CHECK_FAILED", Message: "官方連線診斷未完成。"}
		var failure *nhi.Error
		if errors.As(err, &failure) {
			result.Error.Code = failure.Code
			result.Error.Message = failure.Message
			result.Error.UpstreamStatus = failure.HTTPStatus
		}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if encodeErr := encoder.Encode(result); encodeErr != nil {
		return errors.New("cannot write NHI diagnostic result")
	}
	if err != nil {
		return errors.New("NHI diagnostic did not obtain a valid CAPTCHA; see the JSON result")
	}
	return nil
}

type diagnosticResult struct {
	Version        string             `json:"version"`
	Commit         string             `json:"commit"`
	GoVersion      string             `json:"go_version"`
	StartedUTC     string             `json:"started_utc"`
	Operation      string             `json:"operation"`
	Attempts       int                `json:"attempts"`
	ElapsedMS      int64              `json:"elapsed_ms"`
	ImageDecoded   bool               `json:"image_decoded"`
	LoginAttempted bool               `json:"login_attempted"`
	Image          *diagnosticImage   `json:"image,omitempty"`
	Error          *diagnosticFailure `json:"error,omitempty"`
	Diagnostics    *nhi.Diagnostics   `json:"diagnostics,omitempty"`
	Copyright      string             `json:"copyright"`
}

type diagnosticFailure struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	UpstreamStatus int    `json:"upstream_status,omitempty"`
}

type diagnosticImage struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
}

func inspectChallenge(data []byte) (*diagnosticImage, error) {
	invalid := func() (*diagnosticImage, error) {
		return nil, &nhi.Error{Code: "CAPTCHA_INVALID_IMAGE", Message: "官方回傳圖片未通過完整解碼檢查。"}
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || configuration.Width <= 0 || configuration.Height <= 0 ||
		configuration.Width > 2048 || configuration.Height > 2048 {
		return invalid()
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || decoded.Bounds().Dx() != configuration.Width ||
		decoded.Bounds().Dy() != configuration.Height {
		return invalid()
	}
	return &diagnosticImage{Format: format, Width: configuration.Width, Height: configuration.Height, Bytes: len(data)}, nil
}
