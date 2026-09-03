// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// newProxyServer returns a *http.Server that listens on proxyAddr and
// reverse-proxies every request to primaryHandler. The proxy preserves
// the original Host header so the upstream handler can branch on it
// (it currently does not, but the architecture leaves the door open).
func newProxyServer(proxyAddr string, primaryHandler http.Handler, logger *slog.Logger) *http.Server {
	target := &url.URL{Scheme: "http", Host: strings.TrimSpace(proxyAddr)}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("proxy error",
			slog.String("host", r.Host),
			slog.String("path", r.URL.Path),
			slog.Any("err", err),
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	return &http.Server{
		Addr:              proxyAddr,
		Handler:           rp,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// validateLoopbackAddr is the same numeric-loopback check applied to
// the canonical address, but as a standalone helper for any second
// listener we add (the friendly-URL proxy today; potentially others
// later). It does not consult the file system or the resolver; it
// only parses the string and verifies the host portion is a numeric
// loopback IP.
func validateLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return &loopbackAddrError{Addr: addr}
	}
	return nil
}

type loopbackAddrError struct{ Addr string }

func (e *loopbackAddrError) Error() string {
	return "address " + e.Addr + " must use a numeric loopback host; TimeKeeper is local-first"
}
