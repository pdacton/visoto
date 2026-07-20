package main

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// souinFlushURL is Caddy's internal souin admin endpoint, reached over the
// visoto-network Docker bridge (never published externally — see Caddyfile's
// @souinAdminExternal matcher). The Host header must match the Caddyfile's
// site block so Caddy routes the request to the right server; the container
// DNS name "caddy" doesn't match that hostname's TLS certificate, so
// verification is skipped for this internal, same-trust-boundary call.
const souinFlushURL = "https://caddy/souin-api/souin/flush"
const souinHost = "visoto.hutzli.org"

var cachePurgeClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		// ServerName pins the TLS SNI to the site hostname rather than the "caddy"
		// DNS name in souinFlushURL. Caddy has no certificate for "caddy" and aborts
		// the handshake with "tls: internal error" if the SNI doesn't match a site
		// block; setting req.Host below only fixes the HTTP Host header, not the SNI.
		// InsecureSkipVerify still skips client-side cert validation for this
		// internal, same-trust-boundary call (see souinFlushURL comment).
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: souinHost}, //nolint:gosec // internal Docker-network call, see souinFlushURL comment
	},
}

// cachePurgeHandler serves POST /api/cache/purge — the "Clear cache" action in
// the Settings menu. It mediates the browser -> souin purge call: the browser
// only ever talks to this route, never to souin's admin API directly.
func cachePurgeHandler(c *gin.Context) {
	// PURGE is souin's non-standard cache-invalidation method (not a net/http
	// MethodXxx constant).
	req, err := http.NewRequestWithContext(c.Request.Context(), "PURGE", souinFlushURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	req.Host = souinHost

	resp, err := cachePurgeClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": resp.Status})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
