package eligibility

// IsEligibleSnapshot checks if a state+stake combination passes the Proof-of-Humanity
// rules. Humans must meet the dynamic discrimination stake threshold, Verified and
// Newbie identities need at least 10k iDNA. This helper was duplicated in several
// packages (server, indexer, strictbuilder). It now lives here for reuse.
func IsEligibleSnapshot(state string, stake float64, threshold float64) bool {
	if state == "Human" && stake >= threshold {
		return true
	}
	if (state == "Verified" || state == "Newbie") && stake >= 10000 {
		return true
	}
	return false
}

// IsEligibleFull applies the snapshot rules plus penalty/flip checks.
func IsEligibleFull(state string, stake float64, penalized, flip bool, threshold float64) bool {
	if penalized || flip {
		return false
	}
	return IsEligibleSnapshot(state, stake, threshold)
}
