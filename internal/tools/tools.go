package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tylermclean/mfl-mcp/internal/mfl"
)

type Defaults struct {
	Year     int
	LeagueID string
}

type Handler struct {
	client   *mfl.Client
	defaults Defaults
}

func New(client *mfl.Client, defaults Defaults) *Handler {
	return &Handler{client: client, defaults: defaults}
}

func (h *Handler) Register(server *mcp.Server) {
	annotation := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_league", Title: "Get MFL league", Annotations: annotation,
		Description: "Get an MFL league's configuration, franchises, divisions, and lineup requirements.",
	}, h.getLeague)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_rosters", Title: "Get MFL rosters", Annotations: annotation,
		Description: "Get current or historical rosters for all franchises or one franchise in an MFL league.",
	}, h.getRosters)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_standings", Title: "Get MFL standings", Annotations: annotation,
		Description: "Get current league standings from MFL.",
	}, h.getStandings)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_transactions", Title: "Get MFL transactions", Annotations: annotation,
		Description: "Get league transactions with optional week, franchise, type, day, and count filters.",
	}, h.getTransactions)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_players", Title: "Get MFL players", Annotations: annotation,
		Description: "Get MFL's player database for a season. This is a large response; use details=false unless detailed attributes are needed.",
	}, h.getPlayers)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_rules", Title: "Get MFL scoring rules", Annotations: annotation,
		Description: "Get a league's scoring rules. Use get_all_rules to translate MFL scoring-rule abbreviations when needed.",
	}, h.getRules)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_all_rules", Title: "Get MFL scoring rule definitions", Annotations: annotation,
		Description: "Get MFL's season-wide scoring-rule definitions and abbreviations. This data is not league-specific and changes infrequently.",
	}, h.getAllRules)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_free_agents", Title: "Get MFL free agents", Annotations: annotation,
		Description: "Get the currently unrostered player IDs in a league, optionally filtered by position.",
	}, h.getFreeAgents)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_assets", Title: "Get MFL franchise assets", Annotations: annotation,
		Description: "Get every franchise's tradable players, current-year draft picks, and future draft picks.",
	}, h.getAssets)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_future_draft_picks", Title: "Get MFL future draft picks", Annotations: annotation,
		Description: "Get current ownership of future draft picks for every franchise in a league.",
	}, h.getFutureDraftPicks)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_draft_results", Title: "Get MFL draft results", Annotations: annotation,
		Description: "Get a league's draft order and completed picks. MFL notes that this export can lag a live draft by up to 15 minutes.",
	}, h.getDraftResults)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_live_draft_results", Title: "Get live MFL draft results", Annotations: annotation,
		Description: "Get MFL's near-real-time XML draft feed, including draft order, completed picks, on-the-clock position, and draft status. Use this during a live or slow draft instead of the delayed draft-results export.",
	}, h.getLiveDraftResults)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_player_scores", Title: "Get MFL player scores", Annotations: annotation,
		Description: "Get league-scored player points for a week, YTD, or weekly average; supports player, position, and free-agent filters.",
	}, h.getPlayerScores)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_projected_scores", Title: "Get MFL projected scores", Annotations: annotation,
		Description: "Get MFL's league-scored weekly projections, optionally filtered by player, position, or current free-agent status. These are weekly projections, not the draft model's required multi-year projections.",
	}, h.getProjectedScores)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_player_profiles", Title: "Get MFL player profiles", Annotations: annotation,
		Description: "Get profiles for up to 100 MFL player IDs, including date of birth and ADP when available.",
	}, h.getPlayerProfiles)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_salary_adjustments", Title: "Get MFL salary adjustments", Annotations: annotation,
		Description: "Get league-level salary cap credits and charges that are separate from player salaries.",
	}, h.getSalaryAdjustments)
}

type LeagueInput struct {
	LeagueID string `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year     int    `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
}

type RosterInput struct {
	LeagueID  string `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year      int    `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Week      int    `json:"week,omitempty" jsonschema:"Optional historical roster week"`
	Franchise string `json:"franchise,omitempty" jsonschema:"Optional four-digit franchise ID"`
}

type TransactionInput struct {
	LeagueID  string `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year      int    `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Week      int    `json:"week,omitempty" jsonschema:"Optional transaction week"`
	Franchise string `json:"franchise,omitempty" jsonschema:"Optional four-digit franchise ID"`
	Types     string `json:"types,omitempty" jsonschema:"Optional comma-separated transaction types, such as TRADE,WAIVER,FREE_AGENT"`
	Days      int    `json:"days,omitempty" jsonschema:"Only transactions from this many recent days"`
	Count     int    `json:"count,omitempty" jsonschema:"Maximum number of common transaction entries"`
}

type PlayersInput struct {
	Year    int  `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Details bool `json:"details,omitempty" jsonschema:"Include detailed player attributes"`
}

type SeasonInput struct {
	Year int `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
}

type FreeAgentsInput struct {
	LeagueID string `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year     int    `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Position string `json:"position,omitempty" jsonschema:"Optional MFL position abbreviation"`
}

type PlayerScoresInput struct {
	LeagueID       string   `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year           int      `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Week           string   `json:"week,omitempty" jsonschema:"Week 1-21, YTD, or AVG; defaults to MFL's current week"`
	PlayerIDs      []string `json:"player_ids,omitempty" jsonschema:"Optional list of up to 100 MFL player IDs"`
	Position       string   `json:"position,omitempty" jsonschema:"Optional MFL position abbreviation"`
	FreeAgentsOnly bool     `json:"free_agents_only,omitempty" jsonschema:"Return only players who are currently league free agents"`
	Recalculate    bool     `json:"recalculate,omitempty" jsonschema:"Ask MFL to recalculate current-week scores using this league's rules"`
	Count          int      `json:"count,omitempty" jsonschema:"Optional maximum number of players"`
}

type ProjectedScoresInput struct {
	LeagueID       string   `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year           int      `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Week           int      `json:"week,omitempty" jsonschema:"Optional projection week from 1 through 21"`
	PlayerIDs      []string `json:"player_ids,omitempty" jsonschema:"Optional list of up to 100 MFL player IDs"`
	Position       string   `json:"position,omitempty" jsonschema:"Optional MFL position abbreviation"`
	FreeAgentsOnly bool     `json:"free_agents_only,omitempty" jsonschema:"Return only players who are currently league free agents"`
	Count          int      `json:"count,omitempty" jsonschema:"Optional maximum number of players"`
}

type PlayerProfilesInput struct {
	Year      int      `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	PlayerIDs []string `json:"player_ids" jsonschema:"One to 100 MFL player IDs"`
}

type LiveDraftInput struct {
	LeagueID string `json:"league_id,omitempty" jsonschema:"MFL league ID; defaults to MFL_LEAGUE_ID"`
	Year     int    `json:"year,omitempty" jsonschema:"MFL season; defaults to MFL_YEAR"`
	Unit     string `json:"unit,omitempty" jsonschema:"Draft unit; defaults to LEAGUE, or use a Deluxe league unit such as CONFERENCE00"`
}

func (h *Handler) getLeague(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	return h.export(ctx, year, leagueID, "league", nil)
}

func (h *Handler) getRosters(ctx context.Context, _ *mcp.CallToolRequest, input RosterInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	params := make(url.Values)
	if input.Week != 0 {
		if input.Week < 1 || input.Week > 21 {
			return nil, nil, errors.New("week must be between 1 and 21")
		}
		params.Set("W", strconv.Itoa(input.Week))
	}
	if input.Franchise != "" {
		params.Set("FRANCHISE", input.Franchise)
	}
	return h.export(ctx, year, leagueID, "rosters", params)
}

func (h *Handler) getStandings(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	return h.export(ctx, year, leagueID, "leagueStandings", nil)
}

func (h *Handler) getTransactions(ctx context.Context, _ *mcp.CallToolRequest, input TransactionInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	params := make(url.Values)
	if input.Week != 0 {
		if input.Week < 1 || input.Week > 21 {
			return nil, nil, errors.New("week must be between 1 and 21")
		}
		params.Set("W", strconv.Itoa(input.Week))
	}
	if input.Franchise != "" {
		params.Set("FRANCHISE", input.Franchise)
	}
	if input.Types != "" {
		params.Set("TRANS_TYPE", strings.ToUpper(input.Types))
	}
	if input.Days != 0 {
		if input.Days < 1 {
			return nil, nil, errors.New("days must be positive")
		}
		params.Set("DAYS", strconv.Itoa(input.Days))
	}
	if input.Count != 0 {
		if input.Count < 1 {
			return nil, nil, errors.New("count must be positive")
		}
		params.Set("COUNT", strconv.Itoa(input.Count))
	}
	return h.export(ctx, year, leagueID, "transactions", params)
}

func (h *Handler) getPlayers(ctx context.Context, _ *mcp.CallToolRequest, input PlayersInput) (*mcp.CallToolResult, any, error) {
	year := input.Year
	if year == 0 {
		year = h.defaults.Year
	}
	params := make(url.Values)
	if input.Details {
		params.Set("DETAILS", "1")
	} else {
		params.Set("DETAILS", "0")
	}
	return h.export(ctx, year, "", "players", params)
}

func (h *Handler) getRules(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	return h.leagueExport(ctx, input, "rules", nil)
}

func (h *Handler) getAllRules(ctx context.Context, _ *mcp.CallToolRequest, input SeasonInput) (*mcp.CallToolResult, any, error) {
	year := input.Year
	if year == 0 {
		year = h.defaults.Year
	}
	return h.export(ctx, year, "", "allRules", nil)
}

func (h *Handler) getFreeAgents(ctx context.Context, _ *mcp.CallToolRequest, input FreeAgentsInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	params := make(url.Values)
	if position := strings.ToUpper(strings.TrimSpace(input.Position)); position != "" {
		params.Set("POSITION", position)
	}
	return h.export(ctx, year, leagueID, "freeAgents", params)
}

func (h *Handler) getAssets(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	return h.leagueExport(ctx, input, "assets", nil)
}

func (h *Handler) getFutureDraftPicks(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	return h.leagueExport(ctx, input, "futureDraftPicks", nil)
}

func (h *Handler) getDraftResults(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	return h.leagueExport(ctx, input, "draftResults", nil)
}

func (h *Handler) getLiveDraftResults(ctx context.Context, _ *mcp.CallToolRequest, input LiveDraftInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	payload, err := h.client.LiveDraftResults(ctx, year, leagueID, input.Unit)
	if err != nil {
		return nil, nil, err
	}
	xmlText := string(payload)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: xmlText}},
		StructuredContent: map[string]any{"format": "xml", "xml": xmlText},
	}, nil, nil
}

func (h *Handler) getPlayerScores(ctx context.Context, _ *mcp.CallToolRequest, input PlayerScoresInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	params := make(url.Values)
	if input.Week != "" {
		week, err := normalizeScoreWeek(input.Week)
		if err != nil {
			return nil, nil, err
		}
		params.Set("W", week)
	}
	if err := setPlayerFilters(params, input.PlayerIDs, input.Position, input.FreeAgentsOnly, input.Count); err != nil {
		return nil, nil, err
	}
	if input.Recalculate {
		if strings.TrimSpace(input.Week) != "" {
			return nil, nil, errors.New("recalculate requires week to be omitted so MFL uses the current week")
		}
		params.Set("RULES", "1")
	}
	return h.export(ctx, year, leagueID, "playerScores", params)
}

func (h *Handler) getProjectedScores(ctx context.Context, _ *mcp.CallToolRequest, input ProjectedScoresInput) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	params := make(url.Values)
	if input.Week != 0 {
		if input.Week < 1 || input.Week > 21 {
			return nil, nil, errors.New("week must be between 1 and 21")
		}
		params.Set("W", strconv.Itoa(input.Week))
	}
	if err := setPlayerFilters(params, input.PlayerIDs, input.Position, input.FreeAgentsOnly, input.Count); err != nil {
		return nil, nil, err
	}
	return h.export(ctx, year, leagueID, "projectedScores", params)
}

func (h *Handler) getPlayerProfiles(ctx context.Context, _ *mcp.CallToolRequest, input PlayerProfilesInput) (*mcp.CallToolResult, any, error) {
	year := input.Year
	if year == 0 {
		year = h.defaults.Year
	}
	playerIDs, err := normalizePlayerIDs(input.PlayerIDs, true)
	if err != nil {
		return nil, nil, err
	}
	return h.export(ctx, year, "", "playerProfile", url.Values{"P": {strings.Join(playerIDs, ",")}})
}

func (h *Handler) getSalaryAdjustments(ctx context.Context, _ *mcp.CallToolRequest, input LeagueInput) (*mcp.CallToolResult, any, error) {
	return h.leagueExport(ctx, input, "salaryAdjustments", nil)
}

func (h *Handler) leagueExport(ctx context.Context, input LeagueInput, exportType string, params url.Values) (*mcp.CallToolResult, any, error) {
	year, leagueID, err := h.resolve(input.Year, input.LeagueID)
	if err != nil {
		return nil, nil, err
	}
	return h.export(ctx, year, leagueID, exportType, params)
}

func setPlayerFilters(params url.Values, playerIDs []string, position string, freeAgentsOnly bool, count int) error {
	if len(playerIDs) > 0 {
		normalized, err := normalizePlayerIDs(playerIDs, false)
		if err != nil {
			return err
		}
		params.Set("PLAYERS", strings.Join(normalized, ","))
	}
	if position = strings.ToUpper(strings.TrimSpace(position)); position != "" {
		params.Set("POSITION", position)
	}
	if freeAgentsOnly {
		params.Set("STATUS", "freeagent")
	}
	if count != 0 {
		if count < 1 {
			return errors.New("count must be positive")
		}
		params.Set("COUNT", strconv.Itoa(count))
	}
	return nil
}

func normalizeScoreWeek(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "YTD" || value == "AVG" {
		return value, nil
	}
	week, err := strconv.Atoi(value)
	if err != nil || week < 1 || week > 21 {
		return "", errors.New("week must be between 1 and 21, YTD, or AVG")
	}
	return strconv.Itoa(week), nil
}

func normalizePlayerIDs(values []string, required bool) ([]string, error) {
	if required && len(values) == 0 {
		return nil, errors.New("player_ids is required")
	}
	if len(values) > 100 {
		return nil, errors.New("player_ids may contain at most 100 IDs")
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) < 4 || len(value) > 5 || !asciiDigits(value) {
			return nil, errors.New("player_ids must contain four- or five-digit MFL player ID strings")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if required && len(normalized) == 0 {
		return nil, errors.New("player_ids is required")
	}
	return normalized, nil
}

func asciiDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func (h *Handler) resolve(year int, leagueID string) (int, string, error) {
	if year == 0 {
		year = h.defaults.Year
	}
	if leagueID == "" {
		leagueID = h.defaults.LeagueID
	}
	if leagueID == "" {
		return 0, "", errors.New("league_id is required when MFL_LEAGUE_ID is not set")
	}
	return year, leagueID, nil
}

func (h *Handler) export(ctx context.Context, year int, leagueID, exportType string, params url.Values) (*mcp.CallToolResult, any, error) {
	payload, err := h.client.Export(ctx, year, leagueID, exportType, params)
	if err != nil {
		return nil, nil, err
	}
	var structured any
	if err := json.Unmarshal(payload, &structured); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(payload)}},
		StructuredContent: structured,
	}, nil, nil
}
