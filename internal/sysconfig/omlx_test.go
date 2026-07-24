package sysconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLocalOMLXURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://localhost:8000", true},
		{"http://127.0.0.1:8000", true},
		{"http://[::1]:8000", true},
		{"http://localhost:8080", false},
		{"https://embeddings.example.com", false},
		{"not a URL", false},
	}

	for _, test := range tests {
		if got := isLocalOMLXURL(test.url); got != test.want {
			t.Errorf("isLocalOMLXURL(%q) = %t; want %t", test.url, got, test.want)
		}
	}
}

func TestOMLXReachableRejectsHTTPError(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))

		got := omlxReachable(context.Background(), server.URL)
		server.Close()
		if got != (status == http.StatusOK) {
			t.Errorf("omlxReachable returned %t for HTTP %d", got, status)
		}
	}
}

func TestOMLXModelIDMatchesDiscoveryFormats(t *testing.T) {
	tests := []struct {
		configured string
		available  string
		want       bool
	}{
		{"mlx-community/Qwen3-Embedding-0.6B-8bit", "Qwen3-Embedding-0.6B-8bit", true},
		{"mlx-community/Qwen3-Embedding-0.6B-8bit", "mlx-community--Qwen3-Embedding-0.6B-8bit", true},
		{"Qwen3-Embedding-0.6B-8bit", "Qwen3-Embedding-0.6B-8bit", true},
		{"Qwen/Qwen3-Embedding-0.6B", "Qwen3-Embedding-0.6B-8bit", false},
	}

	for _, test := range tests {
		if got := omlxModelIDMatches(test.configured, test.available); got != test.want {
			t.Errorf("omlxModelIDMatches(%q, %q) = %t; want %t", test.configured, test.available, got, test.want)
		}
	}
}
