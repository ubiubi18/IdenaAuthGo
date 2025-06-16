package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAddressListTxt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "list.txt")
	data := "# comment\n0x0000000000000000000000000000000000000001\nbad\n0x0000000000000000000000000000000000000002\n"
	os.WriteFile(f, []byte(data), 0644)
	addrs, err := LoadAddressList(f)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestLoadAddressListJSON(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "list.json")
	data := `["0x0000000000000000000000000000000000000001", "", "bad", "0x0000000000000000000000000000000000000002"]`
	os.WriteFile(f, []byte(data), 0644)
	addrs, err := LoadAddressList(f)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}
