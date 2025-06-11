package whitelist

import (
	"encoding/json"
	"os"
	"sort"
)

// BuildWhitelist converts the status map into a sorted address slice.
func BuildWhitelist(status map[string]string) []string {
	list := make([]string, 0, len(status))
	for addr := range status {
		list = append(list, addr)
	}
	sort.Strings(list)
	return list
}

// SaveWhitelist writes the whitelist to disk as JSON.
func SaveWhitelist(addrs []string, path string) error {
	sort.Strings(addrs)
	b, err := json.MarshalIndent(addrs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
