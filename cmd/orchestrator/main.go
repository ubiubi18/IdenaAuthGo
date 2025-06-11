package main

import (
	"encoding/json"
	"log"
	"os"

	"idenauthgo/whitelist"
)

func main() {
	txData, err := os.ReadFile("data/proof_txs.json")
	if err != nil {
		log.Fatalf("read txs: %v", err)
	}
	var txs []whitelist.Tx
	if err := json.Unmarshal(txData, &txs); err != nil {
		log.Fatalf("decode txs: %v", err)
	}

	addrs := whitelist.ExtractAddresses(txs)
	if err := whitelist.SaveAddresses(addrs, "data/extracted_addresses.json"); err != nil {
		log.Fatalf("save addresses: %v", err)
	}

	flips, err := whitelist.CheckFlipReports(addrs, "http://localhost:9009", "")
	if err != nil {
		log.Fatalf("flip check: %v", err)
	}

	status, err := whitelist.CheckIdentityStatus(addrs, flips, "http://localhost:9009", "")
	if err != nil {
		log.Fatalf("status check: %v", err)
	}

	wl := whitelist.BuildWhitelist(status)
	if err := whitelist.SaveWhitelist(wl, "data/whitelist.json"); err != nil {
		log.Fatalf("save whitelist: %v", err)
	}
}
