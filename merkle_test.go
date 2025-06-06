package main

import "testing"

func TestComputeMerkleRootEmpty(t *testing.T) {
	if res := computeMerkleRoot([]string{}); res != "" {
		t.Fatalf("expected empty string, got %q", res)
	}
}

func TestComputeMerkleRootKnown(t *testing.T) {
	addrs := []string{
		"0x0000000000000000000000000000000000000001",
		"0x0000000000000000000000000000000000000002",
		"0x0000000000000000000000000000000000000003",
	}
	got := computeMerkleRoot(addrs)
	want := "839d9a6ca43af7a125e9ece32839c12217469d40453b82e8a46b91da964f1e03"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestMerkleProof(t *testing.T) {
	addrs := []string{
		"0x0000000000000000000000000000000000000001",
		"0x0000000000000000000000000000000000000002",
		"0x0000000000000000000000000000000000000003",
	}
	root := computeMerkleRoot(addrs)
	proof, ok := computeMerkleProof(addrs, addrs[1])
	if !ok {
		t.Fatalf("proof not found")
	}
	if !verifyMerkleProof(addrs[1], proof, root) {
		t.Fatalf("proof verification failed")
	}
}
