package checks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// APIBase is the base URL used for REST API calls.
// It should not include a trailing slash.
var APIBase = "https://api.idena.io"

// validationSummary mirrors the ValidationSummary API response.
type ValidationSummary struct {
	State     string `json:"state"`
	Stake     string `json:"stake"`
	Approved  bool   `json:"approved"`
	Penalized bool   `json:"penalized"`
}

var (
	badMu    sync.Mutex
	badCache = make(map[int]map[string]struct{})
)

// Swagger: /Epoch/{epoch}/Authors/Bad
func fetchBadAuthors(base, apiKey string, epoch int) (map[string]struct{}, error) {
	bad := make(map[string]struct{})
	cont := ""
	for {
		url := fmt.Sprintf("%s/api/Epoch/%d/Authors/Bad?limit=100", strings.TrimRight(base, "/"), epoch)
		if apiKey != "" {
			url += "&apikey=" + apiKey
		}
		if cont != "" {
			url += "&continuationToken=" + cont
		}
		resp, err := http.Get(url)
		if err != nil {
			return bad, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return bad, fmt.Errorf("status %s", resp.Status)
		}
		var res struct {
			Result []struct {
				Address string `json:"address"`
			} `json:"result"`
			Continuation string `json:"continuationToken"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			resp.Body.Close()
			return bad, err
		}
		resp.Body.Close()
		for _, r := range res.Result {
			bad[strings.ToLower(r.Address)] = struct{}{}
		}
		if res.Continuation == "" {
			break
		}
		cont = res.Continuation
	}
	return bad, nil
}

func getBadAuthors(base, apiKey string, epoch int) (map[string]struct{}, error) {
	badMu.Lock()
	m, ok := badCache[epoch]
	badMu.Unlock()
	if ok {
		return m, nil
	}
	m, err := fetchBadAuthors(base, apiKey, epoch)
	if err != nil {
		return nil, err
	}
	badMu.Lock()
	badCache[epoch] = m
	badMu.Unlock()
	return m, nil
}

// BadAuthors returns the set of addresses reported as bad flip authors for the given epoch.
func BadAuthors(base, apiKey string, epoch int) (map[string]struct{}, error) {
	return getBadAuthors(base, apiKey, epoch)
}

// Swagger: /Epoch/{epoch}/Identity/{address}/ValidationSummary
func FetchValidationSummary(base, apiKey string, epoch int, addr string) (*ValidationSummary, error) {
	url := fmt.Sprintf("%s/api/Epoch/%d/Identity/%s/ValidationSummary", strings.TrimRight(base, "/"), epoch, addr)
	if apiKey != "" {
		if strings.Contains(url, "?") {
			url += "&apikey=" + apiKey
		} else {
			url += "?apikey=" + apiKey
		}
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	var out struct {
		Result ValidationSummary `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}

// LatestEpoch returns the latest epoch number from the API.
func LatestEpoch(base, apiKey string) (int, error) {
	url := strings.TrimRight(base, "/") + "/api/Epoch/Last"
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %s", resp.Status)
	}
	var out struct {
		Result struct {
			Epoch int `json:"epoch"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Result.Epoch, nil
}

// CheckPenaltyFlipForEpoch reports whether an address had a validation penalty or a bad flip report in the specified epoch.
func CheckPenaltyFlipForEpoch(base, apiKey string, epoch int, addr string) (penalized, flip bool, err error) {
	bad, err := getBadAuthors(base, apiKey, epoch)
	if err != nil {
		return false, false, err
	}
	sum, err := FetchValidationSummary(base, apiKey, epoch, strings.ToLower(addr))
	if err != nil {
		return false, false, err
	}
	penalized = sum.Penalized || !sum.Approved
	_, flip = bad[strings.ToLower(addr)]
	return penalized, flip, nil
}

// CheckPenaltyFlip uses the latest epoch-1 for checks.
func CheckPenaltyFlip(base, apiKey, addr string) (penalized, flip bool, err error) {
	latest, err := LatestEpoch(base, apiKey)
	if err != nil {
		return false, false, err
	}
	return CheckPenaltyFlipForEpoch(base, apiKey, latest-1, addr)
}
