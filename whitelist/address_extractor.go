package whitelist

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Tx represents a minimal transaction needed for extraction.
type Tx struct {
	From   string `json:"from"`
	Author string `json:"author"`
}

// ExtractAddresses parses a list of transactions and returns a
// deduplicated, sorted set of sender/author addresses.
func ExtractAddresses(txs []Tx) []string {
	set := make(map[string]struct{})
	for _, tx := range txs {
		if tx.From != "" {
			set[strings.ToLower(tx.From)] = struct{}{}
		}
		if tx.Author != "" {
			set[strings.ToLower(tx.Author)] = struct{}{}
		}
	}
	addrs := make([]string, 0, len(set))
	for a := range set {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)
	return addrs
}

// SaveAddresses writes the address list to disk as JSON.
func SaveAddresses(addrs []string, path string) error {
	sort.Strings(addrs)
	b, err := json.MarshalIndent(addrs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
