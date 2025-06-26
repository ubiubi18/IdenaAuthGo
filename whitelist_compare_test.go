package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestWhitelistMatchesReference compares the whitelist built by Go code
// against the historic Python whitelist. The Python reference list lives in
// agents/address_list.txt (mixed case addresses) while the Go code produces
// data/whitelist_epoch_<N>.json. If any address differs between the two
// implementations the test fails and logs the detailed differences.
func TestWhitelistMatchesReference(t *testing.T) {
	// load Go whitelist output
	var goRef struct {
		Addresses []string `json:"addresses"`
	}
	data, err := os.ReadFile("data/whitelist_epoch_164.json")
	if err != nil {
		t.Fatalf("read go whitelist: %v", err)
	}
	if err := json.Unmarshal(data, &goRef); err != nil {
		t.Fatalf("parse go whitelist: %v", err)
	}

	// load historic Python whitelist
	pyData, err := os.ReadFile("agents/address_list.txt")
	if err != nil {
		t.Fatalf("read python whitelist: %v", err)
	}
	var pyRef []string
	for _, line := range strings.Split(string(pyData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pyRef = append(pyRef, line)
	}

	// normalise to lowercase for comparison
	goMap := make(map[string]struct{}, len(goRef.Addresses))
	for _, a := range goRef.Addresses {
		goMap[strings.ToLower(a)] = struct{}{}
	}
	pyMap := make(map[string]struct{}, len(pyRef))
	for _, a := range pyRef {
		pyMap[strings.ToLower(a)] = struct{}{}
	}

	var missing, extra []string
	for a := range pyMap {
		if _, ok := goMap[a]; !ok {
			missing = append(missing, a)
		}
	}
	for a := range goMap {
		if _, ok := pyMap[a]; !ok {
			extra = append(extra, a)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Logf("addresses missing in Go output (%d): %v", len(missing), missing)
	}
	if len(extra) > 0 {
		t.Logf("addresses extra in Go output (%d): %v", len(extra), extra)
	}
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("whitelist mismatch: go=%d python=%d", len(goRef.Addresses), len(pyRef))
	}

}
