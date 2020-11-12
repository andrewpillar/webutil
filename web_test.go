package webutil

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"
)

func Test_BaseAddress(t *testing.T) {
	tests := []struct {
		request  *http.Request
		expected string
	}{
		{
			&http.Request{
				Header: http.Header{
					"X-Forwarded-Proto": []string{"https"},
					"X-Forwarded-Host":  []string{"example.com"},
				},
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
			},
			"https://example.com",
		},
		{

			&http.Request{
				Header: http.Header{
					"X-Forwarded-Host":  []string{"example.com"},
				},
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
				TLS: &tls.ConnectionState{},
			},
			"https://example.com",
		},
		{

			&http.Request{
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
				Host: "localhost:8080",
			},
			"http://localhost:8080",
		},
	}

	for i, test := range tests {
		if addr := BaseAddress(test.request); addr != test.expected {
			t.Errorf("tests[%d] - unexpected base address, expected=%q, got=%q\n", i, test.expected, addr)
		}
	}
}

func Test_BasePath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"", "/"},
		{"/", "/"},
		{"/////", "/"},
		{"\\\\///", "/"},
		{"/api/files/1/download", "download"},
	}

	for i, test := range tests {
		if path := BasePath(test.path); path != test.expected {
			t.Errorf("tests[%d] - unexpected base path, expected=%q, got=%q\n", i, test.expected, path)
		}
	}
}
