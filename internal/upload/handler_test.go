package upload

import "testing"

func TestValidateRemoteURL(t *testing.T) {
	cases := []struct {
		name         string
		url          string
		allowPrivate bool
		wantErr      bool
	}{
		{"public IP allowed", "http://1.1.1.1/data.ttl", false, false},
		{"loopback IP blocked", "http://127.0.0.1:8060/data.ttl", false, true},
		{"localhost blocked", "http://localhost:8060/data.ttl", false, true},
		{"private IP blocked", "http://192.168.1.10/data.ttl", false, true},
		{"link-local blocked", "http://169.254.169.254/latest/meta-data/", false, true},
		{"unspecified blocked", "http://0.0.0.0/data.ttl", false, true},
		{"loopback allowed when enabled", "http://localhost:8060/data.ttl", true, false},
		{"ftp scheme blocked", "ftp://example.com/data.ttl", false, true},
		{"file scheme blocked even when private allowed", "file:///etc/passwd", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRemoteURL(tc.url, tc.allowPrivate)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateRemoteURL(%q, %v) error = %v, wantErr %v", tc.url, tc.allowPrivate, err, tc.wantErr)
			}
		})
	}
}
