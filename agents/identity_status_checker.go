package agents

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

// StatusCheckerConfig configures the identity status checker.
type StatusCheckerConfig struct {
	NodeURL         string `json:"node_url"`
	ApiKey          string `json:"api_key"`
	AddressListFile string `json:"address_list_file"`
	FlipReportFile  string `json:"flip_report_file"`
	OutputFile      string `json:"output_file"`
}

// LoadStatusCheckerConfig loads the checker config from a JSON file.
func LoadStatusCheckerConfig(path string) (*StatusCheckerConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg StatusCheckerConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CheckIdentityStatuses fetches identity status for addresses excluding those with flips.
func CheckIdentityStatuses(cfg *StatusCheckerConfig) ([]Identity, error) {
	addresses, err := LoadAddressList(cfg.AddressListFile)
	if err != nil {
		return nil, err
	}
	flips, err := LoadAddressList(cfg.FlipReportFile)
	if err != nil {
		return nil, err
	}
	flipSet := make(map[string]struct{})
	for _, a := range flips {
		flipSet[strings.ToLower(a)] = struct{}{}
	}

	var out []Identity
	for _, addr := range addresses {
		if _, skip := flipSet[strings.ToLower(addr)]; skip {
			continue
		}
		id, err := FetchIdentity(addr, cfg.NodeURL, cfg.ApiKey)
		if err != nil {
			log.Printf("[StatusChecker] fetch %s: %v", addr, err)
			continue
		}
		if id.State == "Human" || id.State == "Verified" || id.State == "Newbie" {
			out = append(out, *id)
		}
	}
	return out, nil
}

// RunIdentityStatusChecker executes the status check and writes the result.
func RunIdentityStatusChecker(configPath string) {
	cfg, err := LoadStatusCheckerConfig(configPath)
	if err != nil {
		log.Fatalf("[StatusChecker] load config: %v", err)
	}
	list, err := CheckIdentityStatuses(cfg)
	if err != nil {
		log.Fatalf("[StatusChecker] check statuses: %v", err)
	}
	if cfg.OutputFile == "" {
		return
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(cfg.OutputFile, b, 0644); err != nil {
		log.Printf("[StatusChecker] write output: %v", err)
	}
}
