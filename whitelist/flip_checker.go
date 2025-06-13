package whitelist

// Flip report checker queries each identity and marks those with penalties.

// CheckFlipReports queries each identity and marks those with reported flips.
// If lastValidationFlags contains "AtLeastOneFlipReported", the address is flagged.
// 2025-06-13 ticket #42
func CheckFlipReports(addrs []string, epoch int, nodeURL, apiKey string) (map[string]bool, error) {
	res := make(map[string]bool)
	for _, a := range addrs {
		id, err := fetchIdentity(a, epoch, nodeURL, apiKey)
		if err != nil {
			return nil, err
		}
		flagged := false
		for _, f := range id.LastValidationFlags {
			if f == "AtLeastOneFlipReported" {
				flagged = true
				break
			}
		}
		res[a] = flagged
	}
	return res, nil
}
