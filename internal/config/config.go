package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.myfantasyleague.com"

// Config contains process-wide settings. Secrets are read from the environment
// and are never accepted as MCP tool arguments, which keeps them out of tool
// call transcripts.
type Config struct {
	BaseURL     string
	Year        int
	LeagueID    string
	APIKey      string
	UserCookie  string
	UserAgent   string
	Timeout     time.Duration
	MinInterval time.Duration
}

func Load() (Config, error) {
	year := time.Now().Year()
	if value := strings.TrimSpace(os.Getenv("MFL_YEAR")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 2000 || parsed > 2100 {
			return Config{}, fmt.Errorf("MFL_YEAR must be a season between 2000 and 2100")
		}
		year = parsed
	}

	timeout := 20 * time.Second
	if value := strings.TrimSpace(os.Getenv("MFL_TIMEOUT")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("MFL_TIMEOUT must be a positive duration")
		}
		timeout = parsed
	}

	minInterval := time.Second
	if value := strings.TrimSpace(os.Getenv("MFL_MIN_INTERVAL")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed < 0 {
			return Config{}, fmt.Errorf("MFL_MIN_INTERVAL must be a non-negative duration")
		}
		minInterval = parsed
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MFL_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	userAgent := strings.TrimSpace(os.Getenv("MFL_USER_AGENT"))
	if userAgent == "" {
		userAgent = "mfl-mcp/0.2.0"
	}

	return Config{
		BaseURL:     baseURL,
		Year:        year,
		LeagueID:    strings.TrimSpace(os.Getenv("MFL_LEAGUE_ID")),
		APIKey:      strings.TrimSpace(os.Getenv("MFL_API_KEY")),
		UserCookie:  strings.TrimSpace(os.Getenv("MFL_USER_COOKIE")),
		UserAgent:   userAgent,
		Timeout:     timeout,
		MinInterval: minInterval,
	}, nil
}
