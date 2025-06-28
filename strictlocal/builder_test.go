package strictlocal

import "testing"

func TestFilterIdentities(t *testing.T) {
	list := []IdentityInfo{
		{Address: "a", Stake: 12000, State: "Human", Penalty: "0"},
		{Address: "b", Stake: 9999, State: "Human", Penalty: "0"},
		{Address: "c", Stake: 10000, State: "Verified", Penalty: "0"},
		{Address: "d", Stake: 10000, State: "Zombie", Penalty: "0"},
		{Address: "e", Stake: 10000, State: "Verified", Penalty: "1"},
		{Address: "f", Stake: 10000, State: "Newbie", Penalty: "0", Flags: []string{"AtLeastOneFlipReported"}},
	}
	out := filterIdentities(list, 11000)
	if len(out) != 2 {
		t.Fatalf("expected 2 eligible, got %d", len(out))
	}
	if out[0].Address != "a" || out[1].Address != "c" {
		t.Fatalf("unexpected addresses: %+v", out)
	}
}
