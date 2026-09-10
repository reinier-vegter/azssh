package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"azssh/internal/cache"
)

func TestCheckLatestUsesFreshCachedResult(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore("updates")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	userAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		userAgent = request.Header.Get("User-Agent")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v0.0.2"}`))
	}))
	defer server.Close()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	result, err := checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now }, server.URL)
	if err != nil || !result.Available || result.LatestVersion != "v0.0.2" {
		t.Fatalf("first result = %#v, %v", result, err)
	}
	if userAgent != "azssh-update-check" {
		t.Fatalf("user agent = %q", userAgent)
	}
	result, err = checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now.Add(30 * time.Minute) }, server.URL)
	if err != nil || !result.Available || requests != 1 {
		t.Fatalf("cached result = %#v, %v; requests = %d", result, err, requests)
	}
}

func TestCheckLatestThrottlesFailedRequest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore("updates")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	result, err := checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now }, server.URL)
	if err != nil || result.Available {
		t.Fatalf("failed request result = %#v, %v", result, err)
	}
	result, err = checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now.Add(30 * time.Minute) }, server.URL)
	if err != nil || result.Available || requests != 1 {
		t.Fatalf("throttled result = %#v, %v; requests = %d", result, err, requests)
	}
}

func TestIsNewer(t *testing.T) {
	for _, test := range []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "v1.0.0", current: "v0.9.9", want: true},
		{candidate: "v1.1.0", current: "v1.0.9", want: true},
		{candidate: "v1.0.1", current: "v1.0.0", want: true},
		{candidate: "v1.0.0", current: "v1.0.0", want: false},
		{candidate: "v1.0.0", current: "v1.0.1", want: false},
		{candidate: "v1.0.1", current: "dev", want: false},
	} {
		t.Run(test.candidate+"/"+test.current, func(t *testing.T) {
			if got := isNewer(test.candidate, test.current); got != test.want {
				t.Fatalf("isNewer(%q, %q) = %v, want %v", test.candidate, test.current, got, test.want)
			}
		})
	}
}
