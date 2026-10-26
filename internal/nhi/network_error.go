// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"syscall"
)

var errRedirectRejected = errors.New("official-site redirect rejected")

// networkFailure classifies typed causes without rendering their Error text.
// The wrapped error can contain credentials, proxy URLs, IPs or CAPTCHA IDs.
func networkFailure(err error) *Error {
	if errors.Is(err, context.Canceled) {
		return failure("REQUEST_CANCELED", "本次官方連線已取消，請重新取得驗證碼或資料。")
	}
	if errors.Is(err, errRedirectRejected) {
		return failure("UPSTREAM_REDIRECT_REJECTED", "官方連線要求非預期的重新導向，未繼續傳送資料。請網站管理者確認官方登入流程。")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return failure("DNS_FAILED", "網站伺服器的對外連線無法完成 DNS 解析，請網站管理者檢查主機的 DNS 與網路設定。")
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return failure("TIMEOUT", "網站伺服器與官方服務之間的請求已逾時，請稍後再試。")
	}
	var certificate *tls.CertificateVerificationError
	var invalidCertificate x509.CertificateInvalidError
	var hostname x509.HostnameError
	var authority x509.UnknownAuthorityError
	var roots x509.SystemRootsError
	if errors.As(err, &certificate) || errors.As(err, &invalidCertificate) || errors.As(err, &hostname) || errors.As(err, &authority) || errors.As(err, &roots) {
		return failure("TLS_CERTIFICATE", "網站伺服器無法驗證官方連線的 TLS 憑證，請網站管理者檢查主機時間與 CA 憑證。")
	}
	var record tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &record) || errors.As(err, &alert) {
		return failure("TLS_FAILED", "網站伺服器未能建立官方服務的 TLS 安全連線，請網站管理者檢查對外 HTTPS 連線。")
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return failure("CONNECTION_REFUSED", "網站伺服器的對外連線被拒絕，請網站管理者檢查主機的對外連線。")
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return failure("CONNECTION_RESET", "網站伺服器與官方服務之間的連線被重設，請稍後再試。")
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		// crypto/tls wraps its unexported TCP alert errors in these operations.
		// Never inspect or expose the underlying alert's text.
		if operation.Net == "" && (operation.Op == "remote error" || operation.Op == "local error") {
			return failure("TLS_FAILED", "網站伺服器未能建立官方服務的 TLS 安全連線，請網站管理者檢查對外 HTTPS 連線。")
		}
		if operation.Op == "dial" {
			return failure("CONNECTION_FAILED", "網站伺服器無法建立對外連線，請網站管理者檢查主機的網路與路由設定。")
		}
	}
	return failure("NETWORK", "網站伺服器暫時無法完成官方連線，請稍後再試。")
}

func responseReadFailure(err error, status int) *Error {
	failure := networkFailure(err)
	if failure.Code == "NETWORK" {
		failure = &Error{Code: "RESPONSE_INTERRUPTED", Message: "官方回傳的資料未能完整接收，請重新取得驗證碼或資料。"}
	}
	failure.HTTPStatus = status
	return failure
}
