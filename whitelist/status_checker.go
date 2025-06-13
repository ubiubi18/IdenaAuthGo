package whitelist

// CheckIdentityStatus retrieves identity state for addresses not flagged for
// reported flips.
func CheckIdentityStatus(addrs []string, flips map[string]bool, nodeURL, apiKey string) (map[string]string, error) {
	out := make(map[string]string)
	for _, a := range addrs {
		if flips[a] {
			continue
		}
		id, err := fetchIdentity(a, 0, nodeURL, apiKey)
		if err != nil {
			return nil, err
		}
		if id.State == "Human" || id.State == "Verified" || id.State == "Newbie" {
			out[a] = id.State
		}
	}
	return out, nil
}
