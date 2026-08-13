package tools

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tylermclean/mfl-mcp/internal/mfl"
)

func TestRegisterExposesReadOnlyTools(t *testing.T) {
	t.Parallel()

	mflClient, err := mfl.NewClient(mfl.Config{BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "test"}, nil)
	New(mflClient, Defaults{Year: 2026, LeagueID: "12345"}).Register(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"get_league": false, "get_rosters": false, "get_standings": false,
		"get_transactions": false, "get_players": false, "get_rules": false,
		"get_all_rules": false, "get_free_agents": false, "get_assets": false,
		"get_future_draft_picks": false, "get_draft_results": false, "get_live_draft_results": false,
		"get_player_scores": false, "get_projected_scores": false,
		"get_player_profiles": false, "get_salary_adjustments": false,
	}
	for _, tool := range result.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not annotated read-only", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q was not registered", name)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDraftModelToolsBuildDocumentedMFLRequests(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		arguments any
		year      string
		export    string
		leagueID  string
		query     url.Values
	}{
		{name: "rules", tool: "get_rules", arguments: map[string]any{}, year: "2026", export: "rules", leagueID: "12345"},
		{name: "all rules", tool: "get_all_rules", arguments: map[string]any{"year": 2025}, year: "2025", export: "allRules"},
		{name: "free agents", tool: "get_free_agents", arguments: map[string]any{"position": "wr"}, year: "2026", export: "freeAgents", leagueID: "12345", query: url.Values{"POSITION": {"WR"}}},
		{name: "assets", tool: "get_assets", arguments: map[string]any{}, year: "2026", export: "assets", leagueID: "12345"},
		{name: "future picks", tool: "get_future_draft_picks", arguments: map[string]any{}, year: "2026", export: "futureDraftPicks", leagueID: "12345"},
		{name: "draft results", tool: "get_draft_results", arguments: map[string]any{}, year: "2026", export: "draftResults", leagueID: "12345"},
		{
			name: "player scores", tool: "get_player_scores",
			arguments: map[string]any{"week": "ytd", "player_ids": []string{"0157", "12345", "0157"}, "position": "qb", "free_agents_only": true, "count": 25},
			year:      "2026", export: "playerScores", leagueID: "12345",
			query: url.Values{"W": {"YTD"}, "PLAYERS": {"0157,12345"}, "POSITION": {"QB"}, "STATUS": {"freeagent"}, "COUNT": {"25"}},
		},
		{
			name: "recalculated current player scores", tool: "get_player_scores",
			arguments: map[string]any{"recalculate": true},
			year:      "2026", export: "playerScores", leagueID: "12345", query: url.Values{"RULES": {"1"}},
		},
		{
			name: "projected scores", tool: "get_projected_scores",
			arguments: map[string]any{"week": 3, "player_ids": []string{"12345"}, "position": "te", "free_agents_only": true, "count": 10},
			year:      "2026", export: "projectedScores", leagueID: "12345",
			query: url.Values{"W": {"3"}, "PLAYERS": {"12345"}, "POSITION": {"TE"}, "STATUS": {"freeagent"}, "COUNT": {"10"}},
		},
		{
			name: "player profiles", tool: "get_player_profiles",
			arguments: map[string]any{"player_ids": []string{"0157", "12345"}},
			year:      "2026", export: "playerProfile", query: url.Values{"P": {"0157,12345"}},
		},
		{name: "salary adjustments", tool: "get_salary_adjustments", arguments: map[string]any{}, year: "2026", export: "salaryAdjustments", leagueID: "12345"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if got, want := request.URL.Path, "/"+test.year+"/export"; got != want {
					t.Errorf("path = %q, want %q", got, want)
				}
				wantQuery := make(url.Values, len(test.query)+3)
				for key, values := range test.query {
					wantQuery[key] = append([]string(nil), values...)
				}
				wantQuery.Set("TYPE", test.export)
				wantQuery.Set("JSON", "1")
				if test.leagueID != "" {
					wantQuery.Set("L", test.leagueID)
				}
				if got := request.URL.Query(); got.Encode() != wantQuery.Encode() {
					t.Errorf("query = %q, want %q", got.Encode(), wantQuery.Encode())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			})}

			mflClient, err := mfl.NewClient(mfl.Config{BaseURL: "https://example.test", HTTPClient: httpClient})
			if err != nil {
				t.Fatal(err)
			}
			server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "test"}, nil)
			New(mflClient, Defaults{Year: 2026, LeagueID: "12345"}).Register(server)
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			ctx := context.Background()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
			clientSession, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer clientSession.Close()

			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: test.tool, Arguments: test.arguments})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("tool returned an error: %#v", result.Content)
			}
		})
	}
}

func TestDraftModelToolValidation(t *testing.T) {
	mflClient, err := mfl.NewClient(mfl.Config{BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(mflClient, Defaults{Year: 2026, LeagueID: "12345"})
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "invalid score week",
			call: func() error {
				_, _, err := handler.getPlayerScores(ctx, nil, PlayerScoresInput{Week: "week 3"})
				return err
			},
			want: "week must be between 1 and 21, YTD, or AVG",
		},
		{
			name: "invalid projection week",
			call: func() error {
				_, _, err := handler.getProjectedScores(ctx, nil, ProjectedScoresInput{Week: 22})
				return err
			},
			want: "week must be between 1 and 21",
		},
		{
			name: "missing profile IDs",
			call: func() error {
				_, _, err := handler.getPlayerProfiles(ctx, nil, PlayerProfilesInput{})
				return err
			},
			want: "player_ids is required",
		},
		{
			name: "numeric IDs preserve leading zeroes",
			call: func() error {
				_, _, err := handler.getPlayerProfiles(ctx, nil, PlayerProfilesInput{PlayerIDs: []string{"157"}})
				return err
			},
			want: "four- or five-digit",
		},
		{
			name: "negative count",
			call: func() error {
				_, _, err := handler.getPlayerScores(ctx, nil, PlayerScoresInput{Count: -1})
				return err
			},
			want: "count must be positive",
		},
		{
			name: "recalculation only supports current week",
			call: func() error {
				_, _, err := handler.getPlayerScores(ctx, nil, PlayerScoresInput{Week: "YTD", Recalculate: true})
				return err
			},
			want: "recalculate requires week to be omitted",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}
}
