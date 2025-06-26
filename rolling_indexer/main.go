// Idena rolling indexer
//
// Setup:
//  1. Optionally create a `config.json` with fields `rpc_url`, `rpc_key`,
//     `interval_minutes`, and `db_path`.
//  2. Environment variables `RPC_URL`, `RPC_KEY`, and `FETCH_INTERVAL_MINUTES`
//     override config values when set.
//  3. Build and run with `go build` then `./rolling-indexer`.
//
// The indexer polls an Idena node for all identities every few minutes and stores
// snapshots in `identities.db`. Records older than 30 days are deleted. The HTTP
// server exposes:
//
//	GET /identities/latest   - latest snapshot for all addresses
//	GET /identities/eligible - addresses eligible for PoH (Human/Verified/Newbie
//	                           with stake >= 10,000)
//	GET /identity/{address}  - full history for an address
//	GET /state/{state}       - addresses currently in a given state
//
// If the local node fails, targeted fallback requests are made to the public
// API with rate limits to avoid abuse.

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds runtime settings loaded from env or config.json
type Config struct {
	RPCURL             string `json:"rpc_url"`
	RPCKey             string `json:"rpc_key"`
	IntervalMin        int    `json:"interval_minutes"`
	DBPath             string `json:"db_path"`
	BootstrapEpochs    int    `json:"bootstrap_epochs"`
	UsePublicBootstrap bool   `json:"use_public_bootstrap"`
}

// Snapshot represents one identity record at a particular time
type Snapshot struct {
	Address string    `json:"address"`
	State   string    `json:"state"`
	Stake   float64   `json:"stake"`
	TS      time.Time `json:"timestamp"`
}

var (
	cfg Config
	db  *sql.DB

	// fallback rate limiting
	fbMu      sync.Mutex
	fbTotal   int
	fbWindow  time.Time
	fbPerAddr = make(map[string]*fbInfo)

	tracked = make(map[string]struct{})
)

type fbInfo struct {
	Count  int
	Window time.Time
}

// loadConfig reads config.json if present and applies env overrides
func loadConfig() Config {
	c := Config{
		RPCURL:             "http://localhost:9009",
		RPCKey:             "",
		IntervalMin:        10,
		DBPath:             "identities.db",
		BootstrapEpochs:    3,
		UsePublicBootstrap: true,
	}
	if data, err := os.ReadFile("config.json"); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if v := os.Getenv("RPC_URL"); v != "" {
		c.RPCURL = v
	}
	if v := os.Getenv("RPC_KEY"); v != "" {
		c.RPCKey = v
	}
	if v := os.Getenv("FETCH_INTERVAL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.IntervalMin = n
		}
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("BOOTSTRAP_EPOCHS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.BootstrapEpochs = n
		}
	}
	if v := os.Getenv("USE_PUBLIC_BOOTSTRAP"); v != "" {
		v = strings.ToLower(v)
		if v == "0" || v == "false" || v == "no" {
			c.UsePublicBootstrap = false
		}
	}
	return c
}

func createSchema() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS snapshots (
            address TEXT,
            state   TEXT,
            stake   REAL,
            ts      INTEGER
        );
        CREATE INDEX IF NOT EXISTS idx_addr_ts ON snapshots(address, ts);
    `)
	if err != nil {
		log.Fatalf("create schema: %v", err)
	}
}

func loadTracked() {
	data, err := os.ReadFile("addresses.txt")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tracked[line] = struct{}{}
		}
	}
}

func dbIsEmpty() bool {
	row := db.QueryRow("SELECT COUNT(*) FROM snapshots")
	var n int
	if err := row.Scan(&n); err != nil {
		return true
	}
	return n == 0
}

func fetchPublicEpoch() (int, error) {
	resp, err := http.Get("https://rpc.idena.io/api/Epoch/Last")
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

func fetchPublicEpochIdentities(epoch int) ([]Snapshot, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_epochIdentities",
		"params":  []interface{}{epoch, 0},
		"id":      1,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post("https://rpc.idena.io", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	var out struct {
		Result []struct {
			Address string `json:"address"`
			State   string `json:"state"`
			Stake   string `json:"stake"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	list := make([]Snapshot, 0, len(out.Result))
	for _, r := range out.Result {
		stake, _ := strconv.ParseFloat(r.Stake, 64)
		list = append(list, Snapshot{Address: r.Address, State: r.State, Stake: stake})
	}
	return list, nil
}

func bootstrapHistory(epochs int) error {
	log.Println("Bootstrapping from public API... (can be disabled). If unavailable, will only track new data after this session.")
	current, err := fetchPublicEpoch()
	if err != nil {
		return err
	}
	now := time.Now()
	for i := 0; i < epochs; i++ {
		ep := current - i
		snaps, err := fetchPublicEpochIdentities(ep)
		if err != nil {
			return err
		}
		storeSnapshots(snaps, now.AddDate(0, 0, -7*i))
		for _, s := range snaps {
			tracked[s.Address] = struct{}{}
		}
	}
	return nil
}

func fetchAllIdentities() ([]Snapshot, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "dna_identities",
		"params":  []interface{}{},
	}
	if cfg.RPCKey != "" {
		req["key"] = cfg.RPCKey
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(cfg.RPCURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc status %s", resp.Status)
	}
	var out struct {
		Result []struct {
			Address string `json:"address"`
			State   string `json:"state"`
			Stake   string `json:"stake"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	list := make([]Snapshot, 0, len(out.Result))
	for _, r := range out.Result {
		stake, _ := strconv.ParseFloat(r.Stake, 64)
		list = append(list, Snapshot{Address: r.Address, State: r.State, Stake: stake})
	}
	return list, nil
}

func fetchIdentityFallback(addr string) (*Snapshot, error) {
	fbMu.Lock()
	now := time.Now()
	if fbWindow.IsZero() || now.Sub(fbWindow) >= 8*time.Hour {
		fbWindow = now
		fbTotal = 0
	}
	if fbTotal >= 1000 {
		fbMu.Unlock()
		return nil, errors.New("api cooldown mode, try again in 8 hours")
	}
	info := fbPerAddr[addr]
	if info == nil {
		info = &fbInfo{Window: now}
		fbPerAddr[addr] = info
	}
	if now.Sub(info.Window) >= 24*time.Hour {
		info.Window = now
		info.Count = 0
	}
	if info.Count >= 20 {
		fbMu.Unlock()
		return nil, fmt.Errorf("address %s in cooldown", addr)
	}
	info.Count++
	fbTotal++
	fbMu.Unlock()

	url := fmt.Sprintf("https://api.idena.io/api/Identity/%s", addr)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fallback status %s", resp.Status)
	}
	var res struct {
		Result struct {
			Address string `json:"address"`
			State   string `json:"state"`
			Stake   string `json:"stake"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	stake, _ := strconv.ParseFloat(res.Result.Stake, 64)
	return &Snapshot{Address: res.Result.Address, State: res.Result.State, Stake: stake}, nil
}

func storeSnapshots(snaps []Snapshot, ts time.Time) {
	tx, err := db.Begin()
	if err != nil {
		log.Printf("db begin: %v", err)
		return
	}
	stmt, err := tx.Prepare("INSERT INTO snapshots(address,state,stake,ts) VALUES(?,?,?,?)")
	if err != nil {
		log.Printf("prepare: %v", err)
		return
	}
	for _, s := range snaps {
		if _, err := stmt.Exec(s.Address, s.State, s.Stake, ts.Unix()); err != nil {
			log.Printf("insert %s: %v", s.Address, err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		log.Printf("commit: %v", err)
	}
	log.Printf("stored %d snapshots", len(snaps))
}

func cleanupOld(now time.Time) {
	cutoff := now.AddDate(0, 0, -30).Unix()
	res, err := db.Exec("DELETE FROM snapshots WHERE ts < ?", cutoff)
	if err != nil {
		log.Printf("cleanup: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("pruned %d old entries", n)
	}
}

func runIndexer() {
	interval := time.Duration(cfg.IntervalMin) * time.Minute
	if interval == 0 {
		interval = 10 * time.Minute
	}
	loadTracked()
	for {
		now := time.Now()
		snaps, err := fetchAllIdentities()
		if err != nil {
			log.Printf("local RPC failed: %v", err)
			var list []Snapshot
			for addr := range tracked {
				snap, err := fetchIdentityFallback(addr)
				if err != nil {
					log.Printf("fallback for %s: %v", addr, err)
					continue
				}
				list = append(list, *snap)
			}
			if len(list) > 0 {
				storeSnapshots(list, now)
			}
			cleanupOld(now)
			time.Sleep(interval)
			continue
		}
		for _, s := range snaps {
			tracked[s.Address] = struct{}{}
		}
		storeSnapshots(snaps, now)
		cleanupOld(now)
		time.Sleep(interval)
	}
}

func rowsToSnapshots(rows *sql.Rows) ([]Snapshot, error) {
	var list []Snapshot
	for rows.Next() {
		var s Snapshot
		var ts int64
		if err := rows.Scan(&s.Address, &s.State, &s.Stake, &ts); err != nil {
			return nil, err
		}
		s.TS = time.Unix(ts, 0)
		list = append(list, s)
	}
	return list, nil
}

func handleLatest(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
        SELECT s.address, s.state, s.stake, s.ts
        FROM snapshots s
        JOIN (
            SELECT address, MAX(ts) m FROM snapshots GROUP BY address
        ) last ON s.address=last.address AND s.ts=last.m
    `)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list, err := rowsToSnapshots(rows)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func queryEligibleSnapshots() ([]Snapshot, error) {
	rows, err := db.Query(`
        SELECT s.address, s.state, s.stake, s.ts
        FROM snapshots s
        JOIN (
            SELECT address, MAX(ts) m FROM snapshots GROUP BY address
        ) last ON s.address=last.address AND s.ts=last.m
        WHERE s.state IN ('Human','Verified','Newbie') AND s.stake >= 10000
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToSnapshots(rows)
}

func handleEligible(w http.ResponseWriter, r *http.Request) {
	list, err := queryEligibleSnapshots()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func handleAPIWhitelistCurrent(w http.ResponseWriter, r *http.Request) {
	snaps, err := queryEligibleSnapshots()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	addrs := make([]string, len(snaps))
	for i, s := range snaps {
		addrs[i] = s.Address
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"addresses": addrs})
}

func handleIdentity(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimPrefix(r.URL.Path, "/identity/")
	if addr == "" {
		http.Error(w, "missing address", 400)
		return
	}
	rows, err := db.Query("SELECT address, state, stake, ts FROM snapshots WHERE address=? ORDER BY ts DESC", addr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list, err := rowsToSnapshots(rows)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func handleState(w http.ResponseWriter, r *http.Request) {
	st := strings.TrimPrefix(r.URL.Path, "/state/")
	if st == "" {
		http.Error(w, "missing state", 400)
		return
	}
	rows, err := db.Query(`
        SELECT s.address, s.state, s.stake, s.ts
        FROM snapshots s
        JOIN (
            SELECT address, MAX(ts) m FROM snapshots GROUP BY address
        ) last ON s.address=last.address AND s.ts=last.m
        WHERE s.state=?
    `, st)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list, err := rowsToSnapshots(rows)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// getEpochAndThreshold fetches the current epoch and discrimination stake threshold
// from the local node's HTTP API. If the data cannot be retrieved, zero values
// are returned with an error.
func getEpochAndThreshold() (int, float64, error) {
	url := strings.TrimRight(cfg.RPCURL, "/") + "/api/Epoch/Last"
	if cfg.RPCKey != "" {
		url += "?apikey=" + cfg.RPCKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Result struct {
			Epoch     int    `json:"epoch"`
			Threshold string `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, 0, err
	}
	thr, _ := strconv.ParseFloat(out.Result.Threshold, 64)
	return out.Result.Epoch, thr, nil
}

// handleEpochLast serves the /api/Epoch/Last endpoint.
func handleEpochLast(w http.ResponseWriter, r *http.Request) {
	epoch, thr, err := getEpochAndThreshold()
	if err != nil {
		epoch = 0
		thr = 0
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"result": map[string]interface{}{
			"epoch":                        epoch,
			"discriminationStakeThreshold": thr,
		},
	})
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg = loadConfig()
	var err error
	db, err = sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	createSchema()
	if cfg.UsePublicBootstrap && cfg.BootstrapEpochs > 0 && dbIsEmpty() {
		if err := bootstrapHistory(cfg.BootstrapEpochs); err != nil {
			log.Printf("bootstrap failed: %v -- continuing without", err)
		}
	}
	go runIndexer()

	http.HandleFunc("/identities/latest", handleLatest)
	http.HandleFunc("/identities/eligible", handleEligible)
	http.HandleFunc("/api/whitelist/current", handleAPIWhitelistCurrent)
	http.HandleFunc("/api/Epoch/Last", handleEpochLast)
	http.HandleFunc("/identity/", handleIdentity)
	http.HandleFunc("/state/", handleState)

	log.Println("HTTP server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
