// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// ImageInfo describes a decoded CAPTCHA without retaining its pixels or answer.
type ImageInfo struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
}

// InspectChallengeImage checks the same bounded raster data for both the live
// connector and the diagnostic command. A recognizable header alone does not
// establish that the user's browser can display the challenge.
func InspectChallengeImage(data []byte) (*ImageInfo, error) {
	invalid := func() (*ImageInfo, error) {
		return nil, failure("CAPTCHA_INVALID_IMAGE", "官方回傳圖片未通過完整解碼檢查，請重新取得圖形驗證碼。")
	}
	if len(data) == 0 || len(data) > maxImageBytes {
		return invalid()
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || configuration.Width <= 0 || configuration.Height <= 0 ||
		configuration.Width > 2048 || configuration.Height > 2048 {
		return invalid()
	}
	switch format {
	case "png", "jpeg", "gif":
	default:
		return invalid()
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || decoded.Bounds().Dx() != configuration.Width ||
		decoded.Bounds().Dy() != configuration.Height {
		return invalid()
	}
	return &ImageInfo{Format: format, Width: configuration.Width, Height: configuration.Height, Bytes: len(data)}, nil
}
