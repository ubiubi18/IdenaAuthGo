package main

import (
	"flag"
	"log"

	"idenauthgo/agents"
)

func main() {
	cfgPath := flag.String("config", "agents/config.json", "path to fetcher config")
	addressFile := flag.String("address-file", "", "optional static address list")
	flag.Parse()

	log.Println("Starting identity snapshot fetcher...")
	if err := agents.RunIdentityFetcherAutoEpoch(*cfgPath, *addressFile); err != nil {
		log.Fatalf("Fetcher failed: %v", err)
	}
	log.Println("Snapshot fetch complete")
}

// Usage:
//   go run cmd/fetcher/main.go -config agents/config.json
// The fetcher will query the rolling indexer for the list of eligible addresses
// and then contact the Idena node for identity details. The resulting snapshot
// is written to data/whitelist_epoch_<epoch>.json. Use -address-file to override
// the address source if needed.
// Run this periodically (e.g. via cron or a systemd timer) to keep the snapshot
// up to date.
