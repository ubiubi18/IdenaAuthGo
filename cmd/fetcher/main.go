package main

import (
	"flag"
	"log"

	"idenauthgo/agents"
)

func main() {
	cfgPath := flag.String("config", "agents/config.json", "path to fetcher config")
	flag.Parse()

	log.Println("Starting identity snapshot fetcher...")
	if err := agents.RunIdentityFetcherAutoEpoch(*cfgPath); err != nil {
		log.Fatalf("Fetcher failed: %v", err)
	}
	log.Println("Snapshot fetch complete")
}

// Usage:
//   go run cmd/fetcher/main.go -config agents/config.json
// The fetcher will query the connected Idena node for the current epoch and
// write data/whitelist_epoch_<epoch>.json with the snapshot of addresses listed
// in the config's address_list_file.
// Run this periodically (e.g. via cron or a systemd timer) to keep the snapshot
// up to date.
