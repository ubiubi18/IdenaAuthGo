package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBlockParsing(t *testing.T) {
	data := `{"result":{"height":9285951,"flags":["ShortSessionStarted"]}}`
	var br blockResponse
	if err := json.NewDecoder(strings.NewReader(data)).Decode(&br); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if br.Result.Height != 9285951 {
		t.Fatalf("expected height 9285951, got %d", br.Result.Height)
	}
	if !containsFlag(&br.Result, "ShortSessionStarted") {
		t.Fatalf("flag not detected")
	}
}
