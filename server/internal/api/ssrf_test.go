package api

import (
	"context"
	"testing"
)

func TestCheckSSRF_PublicURLs(t *testing.T) {
	allowed := []string{
		"https://example.com",
		"https://api.github.com/repos/foo/bar",
		"http://google.com",
		"https://autoconfig.thunderbird.net/v1.1/gmail.com",
	}
	for _, u := range allowed {
		if err := checkSSRF(context.Background(), u); err != nil {
			t.Errorf("public URL %q should be allowed, got: %v", u, err)
		}
	}
}

func TestCheckSSRF_Loopback(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/admin",
		"http://127.0.0.1:8080/secret",
		"http://localhost/",
		"http://[::1]/",
	}
	for _, u := range blocked {
		if err := checkSSRF(context.Background(), u); err == nil {
			t.Errorf("loopback URL %q should be blocked", u)
		}
	}
}

func TestCheckSSRF_PrivateRanges(t *testing.T) {
	blocked := []string{
		"http://10.0.0.1/",
		"http://10.255.255.255/",
		"http://172.16.0.1/",
		"http://172.31.255.255/",
		"http://192.168.0.1/",
		"http://192.168.255.255/",
		"http://169.254.169.254/latest/meta-data/",   // AWS metadata
		"http://169.254.169.254/computeMetadata/v1/", // GCP metadata
	}
	for _, u := range blocked {
		if err := checkSSRF(context.Background(), u); err == nil {
			t.Errorf("private IP URL %q should be blocked", u)
		}
	}
}

func TestCheckSSRF_DisallowedSchemes(t *testing.T) {
	blocked := []string{
		"file:///etc/passwd",
		"ftp://internal.host/secret",
		"gopher://127.0.0.1:25/",
	}
	for _, u := range blocked {
		if err := checkSSRF(context.Background(), u); err == nil {
			t.Errorf("non-http scheme %q should be blocked", u)
		}
	}
}

func TestCheckSSRF_MalformedURLs(t *testing.T) {
	malformed := []string{
		"not-a-url",
		"://missing-scheme",
		"",
	}
	for _, u := range malformed {
		if err := checkSSRF(context.Background(), u); err == nil {
			t.Errorf("malformed URL %q should be blocked", u)
		}
	}
}

func TestCheckSSRF_IPv6Private(t *testing.T) {
	blocked := []string{
		"http://[fc00::1]/",
		"http://[fe80::1]/",
	}
	for _, u := range blocked {
		if err := checkSSRF(context.Background(), u); err == nil {
			t.Errorf("private IPv6 %q should be blocked", u)
		}
	}
}
