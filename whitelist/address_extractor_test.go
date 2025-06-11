package whitelist

import "testing"

func TestExtractAddresses(t *testing.T) {
	txs := []Tx{
		{From: "0x1", Author: "0xA"},
		{From: "0x2", Author: "0xa"},
		{From: "0x1", Author: ""},
	}
	got := ExtractAddresses(txs)
	want := []string{"0x1", "0x2", "0xa"}
	if len(got) != len(want) {
		t.Fatalf("expected %d addrs, got %d", len(want), len(got))
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("want %s got %s", v, got[i])
		}
	}
}
