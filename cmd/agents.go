package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"idenauthgo/agents"
)

// agentsConfig mirrors config/agents.json structure.
type agentsConfig struct {
	Fetcher       agents.FetcherConfig       `json:"identity-fetcher"`
	SessionFinder agents.SessionFinderConfig `json:"session-block-finder"`
}

func main() {
	cfgPath := flag.String("config", "config/agents.json", "config file path")
	runSession := flag.Bool("session", false, "run session block finder")
	flag.Parse()

	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	defer f.Close()
	var cfg agentsConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("decode config: %v", err)
	}

	if *runSession {
		agents.RunSessionBlockFinderWithConfig(&cfg.SessionFinder)
		return
	}
	agents.RunIdentityFetcherWithConfig(&cfg.Fetcher)
}
