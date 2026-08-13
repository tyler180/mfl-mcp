package mfl

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 25 << 20

type Client struct {
	baseURL     *url.URL
	http        *http.Client
	apiKey      string
	userCookie  string
	userAgent   string
	minInterval time.Duration
	paceMu      sync.Mutex
	nextRequest time.Time
	hostMu      sync.RWMutex
	leagueHosts map[string]url.URL
}

type Config struct {
	BaseURL     string
	HTTPClient  *http.Client
	APIKey      string
	UserCookie  string
	UserAgent   string
	MinInterval time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse MFL base URL: %w", err)
	}
	if baseURL.Scheme != "https" && baseURL.Scheme != "http" {
		return nil, errors.New("MFL base URL must use http or https")
	}
	if baseURL.Host == "" {
		return nil, errors.New("MFL base URL must include a host")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.MinInterval < 0 {
		return nil, errors.New("MFL minimum request interval must not be negative")
	}

	return &Client{
		baseURL:     baseURL,
		http:        cfg.HTTPClient,
		apiKey:      cfg.APIKey,
		userCookie:  cfg.UserCookie,
		userAgent:   cfg.UserAgent,
		minInterval: cfg.MinInterval,
		leagueHosts: make(map[string]url.URL),
	}, nil
}

// Export calls an MFL export endpoint and returns its JSON response without
// imposing a brittle schema over MFL's season-specific payloads.
func (c *Client) Export(ctx context.Context, year int, leagueID, exportType string, params url.Values) (json.RawMessage, error) {
	if year < 2000 || year > 2100 {
		return nil, errors.New("year must be between 2000 and 2100")
	}
	if strings.TrimSpace(exportType) == "" {
		return nil, errors.New("export type is required")
	}
	if err := c.waitForRequestSlot(ctx); err != nil {
		return nil, err
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strconv.Itoa(year) + "/export"
	query := cloneValues(params)
	query.Set("TYPE", exportType)
	query.Set("JSON", "1")
	if leagueID != "" {
		query.Set("L", leagueID)
	}
	if c.apiKey != "" {
		query.Set("APIKEY", c.apiKey)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create MFL request: %w", err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.userCookie != "" {
		req.AddCookie(&http.Cookie{Name: "MFL_USER_ID", Value: c.userCookie})
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request MFL %s export: %w", exportType, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read MFL response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("MFL response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MFL returned %s: %s", resp.Status, abbreviated(body, 4096))
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("MFL returned invalid JSON: %s", abbreviated(body, 4096))
	}
	if message := apiError(body); message != "" {
		return nil, fmt.Errorf("MFL API error: %s", message)
	}
	if leagueID != "" {
		responseURL := &endpoint
		if resp.Request != nil && resp.Request.URL != nil {
			responseURL = resp.Request.URL
		}
		c.rememberLeagueHost(year, leagueID, responseURL)
	}
	return json.RawMessage(body), nil
}

// LiveDraftResults gets MFL's near-real-time XML draft feed. Unlike export
// requests, this feed must use the league's actual host and does not redirect.
func (c *Client) LiveDraftResults(ctx context.Context, year int, leagueID, unit string) ([]byte, error) {
	if year < 2000 || year > 2100 {
		return nil, errors.New("year must be between 2000 and 2100")
	}
	if !numericID(leagueID, 1, 8) {
		return nil, errors.New("league ID must be a numeric string")
	}
	unit = strings.ToUpper(strings.TrimSpace(unit))
	if unit == "" {
		unit = "LEAGUE"
	}
	if !pathToken(unit, 1, 32) {
		return nil, errors.New("draft unit must contain only letters, digits, or underscores")
	}

	origin, ok := c.leagueHost(year, leagueID)
	if !ok {
		if _, err := c.Export(ctx, year, leagueID, "league", nil); err != nil {
			return nil, fmt.Errorf("resolve MFL league host: %w", err)
		}
		origin, ok = c.leagueHost(year, leagueID)
		if !ok {
			return nil, errors.New("resolve MFL league host: export response did not identify a host")
		}
	}

	endpoint := origin
	endpoint.Path = fmt.Sprintf("/fflnetdynamic%d/%s_%s_draft_results.xml", year, leagueID, unit)
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create MFL live draft request: %w", err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.userCookie != "" {
		req.AddCookie(&http.Cookie{Name: "MFL_USER_ID", Value: c.userCookie})
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request MFL live draft results: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read MFL live draft response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("MFL response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MFL returned %s: %s", resp.Status, abbreviated(body, 4096))
	}
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		if _, err := decoder.Token(); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("MFL returned invalid live draft XML: %w", err)
		}
	}
	return body, nil
}

func (c *Client) rememberLeagueHost(year int, leagueID string, endpoint *url.URL) {
	origin := url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}
	c.hostMu.Lock()
	c.leagueHosts[leagueHostKey(year, leagueID)] = origin
	c.hostMu.Unlock()
}

func (c *Client) leagueHost(year int, leagueID string) (url.URL, bool) {
	c.hostMu.RLock()
	origin, ok := c.leagueHosts[leagueHostKey(year, leagueID)]
	c.hostMu.RUnlock()
	return origin, ok
}

func leagueHostKey(year int, leagueID string) string {
	return strconv.Itoa(year) + ":" + leagueID
}

func numericID(value string, minLength, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func pathToken(value string, minLength, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		character := value[i]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func (c *Client) waitForRequestSlot(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}

	c.paceMu.Lock()
	now := time.Now()
	requestAt := now
	if c.nextRequest.After(requestAt) {
		requestAt = c.nextRequest
	}
	c.nextRequest = requestAt.Add(c.minInterval)
	c.paceMu.Unlock()

	delay := time.Until(requestAt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for MFL request slot: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func apiError(body []byte) string {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Error) == 0 || string(envelope.Error) == "null" {
		return ""
	}

	var detail struct {
		Text string `json:"$t"`
	}
	if json.Unmarshal(envelope.Error, &detail) == nil && detail.Text != "" {
		return detail.Text
	}
	var message string
	if json.Unmarshal(envelope.Error, &message) == nil && message != "" {
		return message
	}
	return abbreviated(envelope.Error, 4096)
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}

func abbreviated(value []byte, limit int) string {
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) <= limit {
		return string(value)
	}
	return string(value[:limit]) + "..."
}
