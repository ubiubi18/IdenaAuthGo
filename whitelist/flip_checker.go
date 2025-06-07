package whitelist

// Flip report checker queries each identity and marks those with penalties.

func CheckFlipReports(addrs []string, nodeURL, apiKey string) (map[string]bool, error) {
	res := make(map[string]bool)
	for _, a := range addrs {
		id, err := fetchIdentity(a, nodeURL, apiKey)
		if err != nil {
			return nil, err
		}
		reported := id.Penalty != "" && id.Penalty != "0"
		res[a] = reported
	}
	return res, nil
}
