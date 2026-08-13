package main

import (
	"context"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyler180/mfl-mcp/internal/config"
	"github.com/tyler180/mfl-mcp/internal/mfl"
	servertools "github.com/tyler180/mfl-mcp/internal/tools"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	client, err := mfl.NewClient(mfl.Config{
		BaseURL:     cfg.BaseURL,
		HTTPClient:  &http.Client{Timeout: cfg.Timeout},
		APIKey:      cfg.APIKey,
		UserCookie:  cfg.UserCookie,
		UserAgent:   cfg.UserAgent,
		MinInterval: cfg.MinInterval,
	})
	if err != nil {
		log.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "mfl-mcp", Version: version}, nil)
	servertools.New(client, servertools.Defaults{
		Year:     cfg.Year,
		LeagueID: cfg.LeagueID,
	}).Register(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
