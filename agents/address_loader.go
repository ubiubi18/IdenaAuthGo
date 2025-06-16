package agents

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LoadAddressList reads addresses from a JSON or text file.
// For .json files it expects an array of strings. For .txt it
// accepts one address per line, ignoring blanks and comments.
// Invalid lines are skipped with a warning.
func LoadAddressList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var addrs []string
	ext := filepath.Ext(path)
	if ext == ".json" {
		var arr []string
		if err := json.Unmarshal(data, &arr); err == nil {
			for _, a := range arr {
				a = strings.TrimSpace(a)
				if strings.HasPrefix(a, "0x") && len(a) == 42 {
					addrs = append(addrs, a)
				} else if a != "" {
					log.Printf("[AddressLoader] skip invalid address: %s", a)
				}
			}
			return addrs, nil
		}
		// fallthrough to text parsing if JSON fails
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "0x") && len(line) == 42 {
			addrs = append(addrs, line)
		} else {
			log.Printf("[AddressLoader] skip invalid line: %s", line)
		}
	}
	return addrs, nil
}
