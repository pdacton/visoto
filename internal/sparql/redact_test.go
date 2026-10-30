package sparql

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRedactTransportError: the message names neither URL, host nor IP, and
// the original error stays reachable for errors.Is.
func TestRedactTransportError(t *testing.T) {
	cases := map[string]struct {
		url     string
		timeout time.Duration
		want    string
	}{
		"dns":     {"http://qlever.invalid:7001/query", time.Second, "DNS lookup failed"},
		"refused": {"http://127.0.0.1:1/query", time.Second, "connection failed"},
		"timeout": {"http://10.255.255.1:7001/query", time.Millisecond, "timeout"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tc.url, nil)
			_, err := (&http.Client{Timeout: tc.timeout}).Do(req)
			if err == nil {
				t.Skip("request unexpectedly succeeded")
			}
			red := RedactTransportError(err)
			if got := red.Error(); got != tc.want {
				t.Errorf("RedactTransportError(%q) = %q, want %q", err, got, tc.want)
			}
			for _, leak := range []string{"qlever", "127.0.0.1", "10.255", ":7001", "http"} {
				if strings.Contains(red.Error(), leak) {
					t.Errorf("redacted error %q leaks %q", red, leak)
				}
			}
			if !errors.Is(red, err) {
				t.Error("redacted error no longer wraps the original")
			}
		})
	}
	if RedactTransportError(nil) != nil {
		t.Error("RedactTransportError(nil) != nil")
	}
}
