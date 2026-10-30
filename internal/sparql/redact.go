package sparql

import (
	"context"
	"errors"
	"net"
)

// RedactTransportError hides what Go's HTTP client puts into a request error:
// a *url.Error quotes the request URL, and dial and DNS errors name the host
// and its IP. Endpoint URLs are server-side only, and these errors reach the
// browser (table error cards, the page's raw-data dump, JSON error fields), so
// any error from an endpoint request goes through here before it is returned.
// The original stays reachable through errors.Is / errors.As; log it before
// redacting if the detail matters.
func RedactTransportError(err error) error {
	if err == nil {
		return nil
	}
	return &transportError{err: err}
}

type transportError struct{ err error }

func (e *transportError) Error() string { return TransportErrorReason(e.err) }
func (e *transportError) Unwrap() error { return e.err }

// TransportErrorReason classifies a request error without naming the endpoint.
func TransportErrorReason(err error) string {
	var netErr net.Error
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.As(err, &dnsErr):
		return "DNS lookup failed"
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return "connection failed"
	default:
		return "request failed"
	}
}
