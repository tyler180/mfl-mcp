package mfl

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testHTTPClient(handler roundTripFunc) *http.Client {
	return &http.Client{Transport: handler}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestExportBuildsAuthenticatedJSONRequest(t *testing.T) {
	t.Parallel()

	httpClient := testHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/2026/export" {
			t.Errorf("path = %q", r.URL.Path)
		}
		want := map[string]string{
			"TYPE": "rosters", "JSON": "1", "L": "12345", "W": "3", "APIKEY": "secret-key",
		}
		for key, value := range want {
			if got := r.URL.Query().Get(key); got != value {
				t.Errorf("query %s = %q, want %q", key, got, value)
			}
		}
		if got := r.UserAgent(); got != "mfl-mcp-test" {
			t.Errorf("User-Agent = %q", got)
		}
		cookie, err := r.Cookie("MFL_USER_ID")
		if err != nil || cookie.Value != "cookie-value" {
			t.Errorf("cookie = %#v, err = %v", cookie, err)
		}
		return response(http.StatusOK, `{"rosters":{"franchise":[]}}`), nil
	})

	client, err := NewClient(Config{
		BaseURL: "https://example.test", HTTPClient: httpClient, APIKey: "secret-key",
		UserCookie: "cookie-value", UserAgent: "mfl-mcp-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := client.Export(context.Background(), 2026, "12345", "rosters", url.Values{"W": {"3"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), `{"rosters":{"franchise":[]}}`; got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

func TestExportReturnsMFLAPIError(t *testing.T) {
	t.Parallel()

	httpClient := testHTTPClient(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{"error":{"$t":"Invalid league ID 80000"}}`), nil
	})

	client, err := NewClient(Config{BaseURL: "https://example.test", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Export(context.Background(), 2026, "80000", "league", nil)
	if err == nil || !strings.Contains(err.Error(), "Invalid league ID 80000") {
		t.Fatalf("Export error = %v", err)
	}
}

func TestExportRejectsNonJSONResponse(t *testing.T) {
	t.Parallel()

	httpClient := testHTTPClient(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, "not json"), nil
	})

	client, err := NewClient(Config{BaseURL: "https://example.test", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Export(context.Background(), 2026, "12345", "league", nil); err == nil {
		t.Fatal("Export succeeded, want invalid JSON error")
	}
}

func TestExportRequestPacingHonorsCancellation(t *testing.T) {
	t.Parallel()

	requests := 0
	httpClient := testHTTPClient(func(*http.Request) (*http.Response, error) {
		requests++
		return response(http.StatusOK, `{"ok":true}`), nil
	})
	client, err := NewClient(Config{
		BaseURL: "https://example.test", HTTPClient: httpClient, MinInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Export(context.Background(), 2026, "12345", "league", nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Export(ctx, 2026, "12345", "rosters", nil); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Export error = %v, want context cancellation", err)
	}
	if requests != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests)
	}
}

func TestNewClientRejectsNegativeRequestInterval(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(Config{BaseURL: "https://example.test", MinInterval: -time.Second}); err == nil {
		t.Fatal("NewClient succeeded with a negative request interval")
	}
}

func TestLiveDraftResultsResolvesAndCachesLeagueHost(t *testing.T) {
	t.Parallel()

	var paths []string
	httpClient := testHTTPClient(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/2026/export":
			if got := request.URL.Query().Get("TYPE"); got != "league" {
				t.Errorf("TYPE = %q, want league", got)
			}
			result := response(http.StatusOK, `{"league":{"id":"79286"}}`)
			result.Request = &http.Request{URL: &url.URL{Scheme: "https", Host: "league.test"}}
			return result, nil
		case "/fflnetdynamic2026/79286_LEAGUE_draft_results.xml":
			if request.URL.Host != "league.test" {
				t.Errorf("live draft host = %q, want league.test", request.URL.Host)
			}
			return response(http.StatusOK, `<draftResults><draftUnit unit="LEAGUE"/></draftResults>`), nil
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
			return nil, nil
		}
	})
	client, err := NewClient(Config{BaseURL: "https://example.test", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		payload, err := client.LiveDraftResults(context.Background(), 2026, "79286", "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(payload), `unit="LEAGUE"`) {
			t.Fatalf("payload = %q", payload)
		}
	}
	want := []string{
		"/2026/export",
		"/fflnetdynamic2026/79286_LEAGUE_draft_results.xml",
		"/fflnetdynamic2026/79286_LEAGUE_draft_results.xml",
	}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %q, want %q", paths, want)
	}
}

func TestLiveDraftResultsRejectsUnsafeUnit(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.LiveDraftResults(context.Background(), 2026, "79286", "../draft"); err == nil {
		t.Fatal("LiveDraftResults succeeded with an unsafe unit")
	}
}
