package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	_ "github.com/mattn/go-sqlite3"
	"idenauthgo/checks"
	"idenauthgo/eligibility"
)

// Environment variables, with fallback for local/dev usage
var (
	BASE_URL      = getenv("BASE_URL", "https://proofofhuman.work")
	IDENA_RPC_KEY = getenv("IDENA_RPC_KEY", "")
)

var port = flag.Int("port", 3030, "Port to run the HTTP server on")

const (
	sessionDuration = 60 * 60 // Session duration in seconds
	dbFile          = "./sessions.db"
	idenaRpcUrl     = "http://localhost:9009"
	fallbackApiUrl  = "https://api.idena.io"
)

var (
	db             *sql.DB
	stakeThreshold = 10000.0
	resultTmpl     *template.Template

	wlMu             sync.RWMutex
	currentWhitelist []string
	currentEpoch     int

	logSubsMu sync.Mutex
	logSubs   = map[chan string]struct{}{}

	// identityFetcher can be replaced in tests to avoid network calls
	identityFetcher func(string) (string, float64) = getIdentity

	// fetchEpochIdentitiesFn allows tests to stub epoch identity retrieval
	fetchEpochIdentitiesFn func(int) ([]epochIdentity, error) = fetchEpochIdentities
)

type Session struct {
	Token         string
	Address       string
	Nonce         string
	Authenticated bool
	IdentityState string
	Stake         float64
	Created       int64
}

// logWriter broadcasts log lines to subscribers while also writing to stdout.
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	broadcastLog(line)
	return os.Stdout.Write(p)
}

func broadcastLog(line string) {
	logSubsMu.Lock()
	for ch := range logSubs {
		select {
		case ch <- line:
		default:
		}
	}
	logSubsMu.Unlock()
}

func getenv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

// sanitizeBaseURL ensures HTTPS is used for non-local hosts
func sanitizeBaseURL(u string) string {
	if strings.HasPrefix(u, "http://") {
		parsed, err := url.Parse(u)
		if err == nil {
			host := parsed.Hostname()
			if host != "localhost" && host != "127.0.0.1" {
				parsed.Scheme = "https"
				s := parsed.String()
				log.Printf("[CONFIG] Forcing HTTPS for BASE_URL: %s -> %s", u, s)
				return s
			}
		}
	}
	return u
}

func fetchEpochData() (int, float64, error) {
	url := idenaRpcUrl + "/api/Epoch/Last"
	if IDENA_RPC_KEY != "" {
		url += "?apikey=" + IDENA_RPC_KEY
	}
	resp, err := http.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Epoch     int     `json:"epoch"`
			Threshold float64 `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, err
	}
	return result.Result.Epoch, result.Result.Threshold, nil
}

func main() {
	indexNow := flag.Bool("index", false, "build whitelist for the current epoch and exit")
	epochFlag := flag.Int("epoch", 0, "override epoch number when used with -index")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, logWriter{}))
	if *indexNow {
		runIndexerCLI(*epochFlag)
		return
	}

	var err error
	db, err = sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()
	createSessionTable()
	createSnapshotTable()
	createEpochSnapshotTable()
	createSnapshotMetaTable()
	createConfigTable()
	createEpochTable()
	createMerkleRootTable()
	createPenaltyTable()
	epoch, thr, err := fetchEpochData()
	if err != nil {
		log.Printf("WARNING: Failed to fetch epoch data: %v (will continue...)", err)
	}
	stakeThreshold = thr
	currentEpoch = getConfigInt("current_epoch")
	if currentEpoch != epoch {
		currentEpoch = epoch
		if err := buildEpochWhitelist(epoch, thr); err != nil {
			log.Printf("initial whitelist build: %v", err)
		}
		saveSnapshotMeta(epoch, 0)
		setConfigInt("current_epoch", epoch)
	} else {
		if _, err := getWhitelist(); err != nil {
			if err := buildEpochWhitelist(epoch, thr); err != nil {
				log.Printf("whitelist load failed: %v", err)
			} else {
				saveSnapshotMeta(epoch, 0)
			}
		}
	}
	resultTmpl = mustLoadTemplate("templates/result.html")

	go watchEpochFinalization()

	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/signin", signinHandler)
	http.HandleFunc("/auth/v1/start-session", startSessionHandler)
	http.HandleFunc("/auth/v1/authenticate", authenticateHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/whitelist", whitelistCurrentHandler)
	http.HandleFunc("/whitelist/current", whitelistCurrentHandler)
	http.HandleFunc("/whitelist/epoch/", whitelistEpochHandler)
	http.HandleFunc("/whitelist/check", whitelistCheckHandler)
	http.HandleFunc("/eligibility", eligibilitySnapshotHandler)
	http.HandleFunc("/merkle_root", merkleRootHandler)
	http.HandleFunc("/merkle_proof", merkleProofHandler)
	http.HandleFunc("/logs/stream", logsStreamHandler)
	http.HandleFunc("/epochs", epochsHandler)
	http.HandleFunc("/api/Epoch/Last", epochLastHandler)
	http.HandleFunc("/api/Identity/", identityHandler)

	go cleanupExpiredSessions()
	log.Printf("Server running at http://localhost:%d", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatal(err)
	}
}

func mustLoadTemplate(path string) *template.Template {
	abs, _ := filepath.Abs(path)
	info, err := os.Stat(path)
	if err != nil {
		log.Fatalf("[TEMPLATE][FATAL] Missing template: %v (Path: %s, abs: %s)", err, path, abs)
	}
	log.Printf("[TEMPLATE][CHECK] Exists: %s (%d bytes)", abs, info.Size())
	tmpl, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}).ParseFiles(path)
	if err != nil {
		log.Fatalf("[TEMPLATE][FATAL] Could not parse template: %v", err)
	}
	return tmpl
}

func createSessionTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS sessions (
            token TEXT PRIMARY KEY,
            address TEXT,
            nonce TEXT,
            authenticated INTEGER DEFAULT 0,
            identity_state TEXT,
            stake REAL,
            created INTEGER
        )
    `)
	if err != nil {
		log.Fatal(err)
	}
}

func createSnapshotTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS identity_snapshots (
            address TEXT,
            state TEXT,
            stake REAL,
            ts INTEGER
        )
    `)
	if err != nil {
		log.Fatal(err)
	}
}

func createEpochSnapshotTable() {
	if err := ensureEpochSnapshotTable(db); err != nil {
		log.Fatal(err)
	}
}

func createSnapshotMetaTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS epoch_snapshot_meta (
            epoch INTEGER PRIMARY KEY,
            block INTEGER
        )`)
	if err != nil {
		log.Fatal(err)
	}
}

func createMerkleRootTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS epoch_merkle_roots (
            epoch INTEGER PRIMARY KEY,
            merkle_root TEXT,
            ts INTEGER
        )`)
	if err != nil {
		log.Fatal(err)
	}
}

func createConfigTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS config (
            key TEXT PRIMARY KEY,
            value TEXT
        )`)
	if err != nil {
		log.Fatal(err)
	}
}

func createEpochTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS epoch (
            epoch INTEGER,
            validationTime INTEGER,
            discriminationStakeThreshold REAL,
            ts INTEGER
        )`)
	if err != nil {
		log.Fatal(err)
	}
}

func createPenaltyTable() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS validation_penalties (
            epoch INTEGER,
            address TEXT,
            PRIMARY KEY (epoch, address)
        )`)
	if err != nil {
		log.Fatal(err)
	}
}

func getConfigInt(key string) int {
	row := db.QueryRow("SELECT value FROM config WHERE key=?", key)
	var v string
	if err := row.Scan(&v); err == nil {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

func setConfigInt(key string, val int) {
	_, _ = db.Exec("INSERT OR REPLACE INTO config(key,value) VALUES(?,?)", key, strconv.Itoa(val))
}

func recordIdentitySnapshot(address, state string, stake float64) {
	_, err := db.Exec(`INSERT INTO identity_snapshots(address,state,stake,ts) VALUES(?,?,?,?)`,
		address, state, stake, time.Now().Unix())
	if err != nil {
		log.Printf("[SNAPSHOT] DB error: %v", err)
	}
}

func cleanupOldSnapshots() {
	_, _ = db.Exec("DELETE FROM identity_snapshots WHERE ts < ?", time.Now().AddDate(0, 0, -30).Unix())
}

func recordPenalty(epoch int, address string) {
	_, err := db.Exec(`INSERT OR IGNORE INTO validation_penalties(epoch,address) VALUES(?,?)`,
		epoch, strings.ToLower(address))
	if err != nil {
		log.Printf("[PENALTY] DB error: %v", err)
	}
}

func hasPenalty(epoch int, address string) bool {
	row := db.QueryRow("SELECT 1 FROM validation_penalties WHERE epoch=? AND address=?", epoch, strings.ToLower(address))
	var x int
	return row.Scan(&x) == nil
}

func saveMerkleRoot(epoch int, root string) {
	_, err := db.Exec(`INSERT OR REPLACE INTO epoch_merkle_roots(epoch,merkle_root,ts) VALUES(?,?,?)`, epoch, root, time.Now().Unix())
	if err != nil {
		log.Printf("[MERKLE] save root: %v", err)
	}
}

func getMerkleRoot(epoch int) (string, bool) {
	row := db.QueryRow("SELECT merkle_root FROM epoch_merkle_roots WHERE epoch=?", epoch)
	var root string
	if err := row.Scan(&root); err == nil {
		return root, true
	}
	return "", false
}

func saveSnapshotMeta(epoch, block int) {
	_, err := db.Exec(`INSERT OR REPLACE INTO epoch_snapshot_meta(epoch, block) VALUES(?,?)`, epoch, block)
	if err != nil {
		log.Printf("[SNAPSHOT_META] save: %v", err)
	}
}

func getSnapshotBlock(epoch int) int {
	row := db.QueryRow("SELECT block FROM epoch_snapshot_meta WHERE epoch=?", epoch)
	var blk int
	if err := row.Scan(&blk); err == nil {
		return blk
	}
	return 0
}

func latestSnapshotEpoch() (int, int) {
	row := db.QueryRow("SELECT epoch, block FROM epoch_snapshot_meta ORDER BY epoch DESC LIMIT 1")
	var ep, blk int
	if err := row.Scan(&ep, &blk); err == nil {
		return ep, blk
	}
	return 0, 0
}

func getWhitelist() ([]string, error) {
	wlMu.RLock()
	list := append([]string(nil), currentWhitelist...)
	wlMu.RUnlock()
	if len(list) > 0 {
		return list, nil
	}
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", currentEpoch)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	wlMu.Lock()
	currentWhitelist = append([]string(nil), list...)
	wlMu.Unlock()
	sort.Strings(list)
	return list, nil
}

// loadWhitelistData reads addresses and root from a saved whitelist file.
func loadWhitelistData(epoch int) ([]string, string, error) {
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var out struct {
		MerkleRoot string   `json:"merkle_root"`
		Addresses  []string `json:"addresses"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		// try plain address array
		if err2 := json.Unmarshal(data, &out.Addresses); err2 != nil {
			return nil, "", err
		}
	}
	return out.Addresses, out.MerkleRoot, nil
}

func computeMerkleRoot(list []string) string {
	if len(list) == 0 {
		return ""
	}
	var hashes [][]byte
	for _, a := range list {
		h := sha256.Sum256([]byte(strings.ToLower(a)))
		hashes = append(hashes, h[:])
	}
	for len(hashes) > 1 {
		var next [][]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 == len(hashes) {
				next = append(next, hashes[i])
			} else {
				h := sha256.Sum256(append(hashes[i], hashes[i+1]...))
				next = append(next, h[:])
			}
		}
		hashes = next
	}
	return hex.EncodeToString(hashes[0])
}

type ProofStep struct {
	Hash string `json:"hash"`
	Left bool   `json:"left"`
}

func computeMerkleProof(list []string, target string) ([]ProofStep, bool) {
	if len(list) == 0 {
		return nil, false
	}
	var hashes [][]byte
	idx := -1
	for i, a := range list {
		h := sha256.Sum256([]byte(strings.ToLower(a)))
		hashes = append(hashes, h[:])
		if strings.EqualFold(a, target) {
			idx = i
		}
	}
	if idx == -1 {
		return nil, false
	}
	pos := idx
	var proof []ProofStep
	for len(hashes) > 1 {
		var next [][]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 == len(hashes) {
				if pos == i {
					pos = len(next)
				}
				next = append(next, hashes[i])
				continue
			}
			left := hashes[i]
			right := hashes[i+1]
			if pos == i {
				proof = append(proof, ProofStep{Hash: hex.EncodeToString(right), Left: false})
				pos = len(next)
			} else if pos == i+1 {
				proof = append(proof, ProofStep{Hash: hex.EncodeToString(left), Left: true})
				pos = len(next)
			}
			h := sha256.Sum256(append(left, right...))
			next = append(next, h[:])
		}
		hashes = next
	}
	return proof, true
}

func verifyMerkleProof(address string, proof []ProofStep, root string) bool {
	h := sha256.Sum256([]byte(strings.ToLower(address)))
	cur := h[:]
	for _, step := range proof {
		sib, err := hex.DecodeString(step.Hash)
		if err != nil {
			return false
		}
		if step.Left {
			h := sha256.Sum256(append(sib, cur...))
			cur = h[:]
		} else {
			h := sha256.Sum256(append(cur, sib...))
			cur = h[:]
		}
	}
	return hex.EncodeToString(cur) == root
}

type epochIdentity struct {
	Address string  `json:"address"`
	State   string  `json:"state"`
	Stake   float64 `json:"stake,string"`
}

// Identity represents a record in snapshot.json. Stake may be encoded as a
// string in the file, so we use the ",string" tag.
type Identity struct {
	Address string  `json:"address"`
	State   string  `json:"state"`
	Stake   float64 `json:"stake,string"`
	Age     int     `json:"age,omitempty"`
}

type EligibilityResponse struct {
	Eligible   bool    `json:"eligible"`
	State      string  `json:"state"`
	Stake      float64 `json:"stake"`
	Reason     string  `json:"reason"`
	Epoch      int     `json:"epoch"`
	Block      int     `json:"block"`
	Prediction string  `json:"prediction,omitempty"`
}

func fetchEpochIdentities(epoch int) ([]epochIdentity, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_epochIdentities",
		"params":  []interface{}{epoch, 0},
		"id":      1,
	}
	if IDENA_RPC_KEY != "" {
		req["key"] = IDENA_RPC_KEY
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(idenaRpcUrl, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
	list := make([]epochIdentity, 0, len(out.Result))
	for _, r := range out.Result {
		st, _ := strconv.ParseFloat(r.Stake, 64)
		list = append(list, epochIdentity{Address: r.Address, State: r.State, Stake: st})
	}
	return list, nil
}

// nextEpochHint provides a short message describing what the identity must do
// to be eligible in the next epoch based on the current live state and stake.
func nextEpochHint(state string, stake float64, threshold float64) string {
	if state == "" {
		return "Identity not found"
	}
	switch state {
	case "Human":
		if stake >= threshold {
			return fmt.Sprintf("Stay Human with stake ≥ %.0f IDNA", threshold)
		}
		return fmt.Sprintf("Add stake to %.0f IDNA and stay Human", threshold)
	case "Verified", "Newbie":
		if stake >= 10000 {
			return fmt.Sprintf("Stay %s with stake ≥ 10000 IDNA", state)
		}
		return fmt.Sprintf("Add stake to 10000 IDNA and remain %s", state)
	default:
		return fmt.Sprintf("Become Human and have at least %.0f IDNA stake", threshold)
	}
}

// predictNextEpoch compares current snapshot eligibility with the live state
// and stake to predict eligibility for the next epoch.
func predictNextEpoch(snapshotEligible bool, liveState string, liveStake float64, threshold float64) string {
	liveEligible := eligibility.IsEligibleSnapshot(liveState, liveStake, threshold)
	switch {
	case snapshotEligible && !liveEligible:
		return "eligible this epoch, but not next"
	case !snapshotEligible && liveEligible:
		return "not eligible this epoch, but eligible next"
	case liveEligible:
		return "eligible both epochs"
	default:
		return "not eligible"
	}
}

func buildEpochWhitelist(epoch int, threshold float64) error {
	ids, err := fetchEpochIdentitiesFn(epoch)
	if err != nil || len(ids) == 0 {
		log.Printf("[WHITELIST] local node missing data for epoch %d; using official API fallback", epoch)
		return buildEpochWhitelistAPI(epoch, threshold)
	}
	var snaps []EpochSnapshot
	var list []string
	lastEpoch := epoch - 1
	for _, id := range ids {
		penalized, flip, err := checks.CheckPenaltyFlipForEpoch(fallbackApiUrl, IDENA_RPC_KEY, lastEpoch, id.Address)
		if err != nil {
			log.Printf("[CHECK] %s: %v", id.Address, err)
		}
		snaps = append(snaps, EpochSnapshot{
			Address:      id.Address,
			State:        id.State,
			Stake:        id.Stake,
			Penalized:    penalized,
			FlipReported: flip,
		})
		if eligibility.IsEligibleFull(id.State, id.Stake, penalized, flip, threshold) {
			list = append(list, id.Address)
		}
	}
	if err := upsertEpochSnapshots(db, epoch, snaps); err != nil {
		return err
	}
	sort.Strings(list)
	root := computeMerkleRoot(list)
	saveMerkleRoot(epoch, root)
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	data, _ := json.MarshalIndent(map[string]interface{}{
		"merkle_root": root,
		"addresses":   list,
	}, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	wlMu.Lock()
	currentWhitelist = list
	wlMu.Unlock()
	log.Printf("[WHITELIST] built for epoch %d with %d addresses root=%s", epoch, len(list), root)
	return nil
}

func watchEpochChanges() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		epoch, thr, err := fetchEpochData()
		if err != nil {
			log.Printf("[EPOCH] fetch error: %v", err)
		} else {
			wlMu.RLock()
			cur := currentEpoch
			wlMu.RUnlock()
			if epoch != cur {
				log.Printf("[EPOCH] new epoch %d detected", epoch)
				stakeThreshold = thr
				if err := buildEpochWhitelist(epoch, thr); err != nil {
					log.Printf("[EPOCH] build whitelist: %v", err)
				} else {
					wlMu.Lock()
					currentEpoch = epoch
					wlMu.Unlock()
					setConfigInt("current_epoch", epoch)
				}
			}
		}
		<-ticker.C
	}
}

func exportWhitelist() {
	list, err := getWhitelist()
	if err != nil {
		log.Printf("[WHITELIST] query error: %v", err)
		return
	}
	data := map[string]interface{}{
		"merkle_root": computeMerkleRoot(list),
		"addresses":   list,
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", currentEpoch)
	if err := os.WriteFile(path, b, 0644); err != nil {
		log.Printf("[WHITELIST] failed to write whitelist.json: %v", err)
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start sign-in flow, redirect to Idena app (BASE_URL is used everywhere)
func signinHandler(w http.ResponseWriter, r *http.Request) {
	token := "signin-" + randHex(16)
	now := time.Now().Unix()
	_, err := db.Exec("INSERT INTO sessions(token, created) VALUES (?, ?)", token, now)
	if err != nil {
		log.Printf("[SIGNIN] DB error storing session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	idenaUrl := fmt.Sprintf(
		"https://app.idena.io/dna/signin?token=%s&callback_url=%s&nonce_endpoint=%s&authentication_endpoint=%s&favicon_url=%s",
		token,
		url.QueryEscape(fmt.Sprintf("%s/callback?token=%s", BASE_URL, token)),
		url.QueryEscape(BASE_URL+"/auth/v1/start-session"),
		url.QueryEscape(BASE_URL+"/auth/v1/authenticate"),
		url.QueryEscape(BASE_URL+"/favicon.ico"),
	)
	log.Printf("[SIGNIN] New session token=%s", token)
	log.Printf("[SIGNIN] Redirecting to: %s", idenaUrl)
	http.Redirect(w, r, idenaUrl, http.StatusFound)
}

// Handle nonce requests and log all body info
func startSessionHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] %s %s", r.Method, r.URL.Path)
	bodyBytes, _ := io.ReadAll(r.Body)
	log.Printf("[DEBUG] Body: %s", string(bodyBytes))
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	log.Printf("[NONCE_ENDPOINT] Called: %s %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Token   string `json:"token"`
			Address string `json:"address"`
		}
		if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&req); err != nil {
			log.Printf("[NONCE_ENDPOINT][POST] Invalid body: %v", err)
			writeError(w, "Invalid request")
			return
		}
		nonce := "signin-" + randHex(16)
		_, err := db.Exec("UPDATE sessions SET address=?, nonce=? WHERE token=?", req.Address, nonce, req.Token)
		if err != nil {
			log.Printf("[NONCE_ENDPOINT][POST] DB error: %v", err)
			writeError(w, "DB error")
			return
		}
		log.Printf("[NONCE_ENDPOINT][POST] Nonce issued for token %s, address %s, nonce %s", req.Token, req.Address, nonce)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"nonce": nonce,
			},
		})
	case http.MethodGet:
		log.Printf("[NONCE_ENDPOINT][GET] Query: %v", r.URL.Query())
		token := r.URL.Query().Get("token")
		addr := r.URL.Query().Get("address")
		if token == "" && addr == "" {
			log.Println("[NONCE_ENDPOINT][GET] Empty params – returning 200 OK for health-check")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		if token == "" || addr == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		nonce := "signin-" + randHex(16)
		_, err := db.Exec("UPDATE sessions SET address=?, nonce=? WHERE token=?", addr, nonce, token)
		if err != nil {
			log.Printf("[NONCE_ENDPOINT][GET] DB error: %v", err)
			writeError(w, "DB error")
			return
		}
		log.Printf("[NONCE_ENDPOINT][GET] Nonce issued for token %s, address %s, nonce %s", token, addr, nonce)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"nonce": nonce,
			},
		})
	default:
		log.Printf("[NONCE_ENDPOINT][%s] Method not allowed", r.Method)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// Authenticate nonce signature
func authenticateHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] %s %s", r.Method, r.URL.Path)
	bodyBytes, _ := io.ReadAll(r.Body)
	log.Printf("[DEBUG] Body: %s", string(bodyBytes))
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	log.Printf("[AUTH][RAW] %s %s", r.Method, r.URL.String())
	var req struct {
		Token     string `json:"token"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[AUTH] Invalid request body: %v", err)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"authenticated": false,
				"reason":        "Invalid request format.",
			},
		})
		return
	}

	row := db.QueryRow("SELECT nonce, address FROM sessions WHERE token=?", req.Token)
	var nonce, address string
	if err := row.Scan(&nonce, &address); err != nil {
		log.Printf("[AUTH] Token not found: %s", req.Token)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"authenticated": false,
				"reason":        "Session not found.",
			},
		})
		return
	}
	log.Printf("[AUTH] Authenticating address: %s for token: %s with nonce: %s", address, req.Token, nonce)

	sigOK := verifySignature(nonce, address, req.Signature)
	if !sigOK {
		log.Printf("[AUTH] Signature verification failed for address %s", address)
	}

	state, stake := getIdentity(address)
	eligible, reason := evaluateEligibility(sigOK, state, stake)
	log.Printf("[AUTH] Identity state: %s, stake: %.3f, eligible: %t", state, stake, eligible)

	_, err := db.Exec(`UPDATE sessions SET authenticated=?, identity_state=?, stake=? WHERE token=?`,
		boolToInt(eligible), state, stake, req.Token)
	if err != nil {
		log.Printf("[AUTH] DB error updating session: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	recordIdentitySnapshot(address, state, stake)
	exportWhitelist()

	writeJSON(w, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"authenticated": eligible,
			"reason":        reason,
		},
	})
}

// Show result, log User-Agent, all params
func callbackHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] %s %s", r.Method, r.URL.Path)
	bodyBytes, _ := io.ReadAll(r.Body)
	log.Printf("[DEBUG] Body: %s", string(bodyBytes))
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	token := r.URL.Query().Get("token")
	log.Printf("[CALLBACK] Request params: %v", r.URL.Query())
	log.Printf("[CALLBACK] User-Agent: %s", r.Header.Get("User-Agent"))
	log.Printf("[CALLBACK] Token: %s", token)
	row := db.QueryRow("SELECT address, authenticated, identity_state, stake FROM sessions WHERE token=?", token)
	var address, state string
	var authenticated int
	var stake float64
	err := row.Scan(&address, &authenticated, &state, &stake)

	data := struct {
		Headline string
		Address  string
		State    string
		Stake    float64
		Reason   string
		BaseUrl  string
	}{BaseUrl: BASE_URL}

	if err != nil {
		data.Headline = "Session not found"
		data.Reason = "Your login session could not be found or has expired.<br>Please try logging in again."
		log.Printf("[CALLBACK][DENIED] session not found for token %s", token)
	} else {
		data.Address = address
		data.State = state
		data.Stake = stake
		eligible := authenticated == 1
		var reason string
		if eligible {
			_, reason = evaluateEligibility(true, state, stake)
		} else {
			ok, r := evaluateEligibility(true, state, stake)
			if ok {
				reason = "Invalid signature."
			} else {
				reason = r
			}
		}
		data.Reason = reason
		if eligible {
			data.Headline = "Access granted!"
			log.Printf("[CALLBACK][GRANTED] %s %s %.3f", address, state, stake)
		} else {
			data.Headline = "Access denied!"
			log.Printf("[CALLBACK][DENIED] %s %s %.3f: %s", address, state, stake, reason)
		}
	}

	log.Printf("[CALLBACK] Rendering result page: %s", data.Headline)
	if resultTmpl == nil {
		resultTmpl = mustLoadTemplate("templates/result.html")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := resultTmpl.Execute(w, data); err != nil {
		log.Printf("[CALLBACK][ERROR] Template rendering failed: %v", err)
		fmt.Fprintf(w, "<html><body><h1>%s</h1><p>%s</p><a href=\"%s\">Continue</a></body></html>", data.Headline, data.Reason, BASE_URL)
	}
}

// Return whitelist JSON
func whitelistHandler(w http.ResponseWriter, r *http.Request) {
	list, err := getWhitelist()
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	writeJSON(w, map[string]interface{}{"addresses": list})
}

// whitelistCurrentHandler returns the whitelist file for the node's current epoch.
// It queries the local Idena node via JSON-RPC (dna_epoch) to determine the
// epoch, then serves the corresponding snapshot file. All steps are logged so
// that issues can be diagnosed easily.
func whitelistCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// Log request with timestamp and path
	log.Printf("[WHITELIST][CURRENT] %s %s", time.Now().Format(time.RFC3339), r.URL.Path)

	// Build JSON-RPC request to fetch the current epoch from the node
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_epoch",
		"id":      1,
	}
	if IDENA_RPC_KEY != "" {
		reqBody["key"] = IDENA_RPC_KEY // include API key if configured
	}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post("http://127.0.0.1:9009", "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("[WHITELIST][CURRENT][ERROR] epoch RPC request failed: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Parse the epoch from the RPC response
	var rpcResp struct {
		Result struct {
			Epoch int `json:"epoch"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		log.Printf("[WHITELIST][CURRENT][ERROR] decoding epoch response: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	epoch := rpcResp.Result.Epoch
	log.Printf("[WHITELIST][CURRENT] fetched epoch=%d", epoch)

	// Construct the file path for this epoch's whitelist
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	log.Printf("[WHITELIST][CURRENT] path=%s", path)

	// Read and return the file. Errors are logged and reported as 500.
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[WHITELIST][CURRENT][ERROR] reading %s: %v", path, err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func whitelistEpochHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	epochStr := parts[len(parts)-1]
	epoch, err := strconv.Atoi(epochStr)
	if err != nil {
		http.Error(w, "bad epoch", 400)
		return
	}
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		http.Error(w, "server error", 500)
		return
	}
	writeJSON(w, map[string]interface{}{"addresses": list, "epoch": epoch})
}

// whitelistCheckHandler fetches identity details for the given address and
// returns an eligibility decision along with a reason. Errors are logged but the
// response always contains a structured JSON result.
func whitelistCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var (
		eligible bool
		valid    bool
		state    string
		stake    float64
		reason   string
		rule     string
		hint     string
		logErr   string
	)

	addr := strings.ToLower(r.URL.Query().Get("address"))

	defer func() {
		if rec := recover(); rec != nil {
			logErr = fmt.Sprintf("panic: %v", rec)
			log.Printf("[WHITELIST][CHECK][PANIC] %v\n%s", rec, debug.Stack())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			return
		}
		log.Printf("[WHITELIST][CHECK] address=%s eligible=%t state=%s stake=%.3f reason=%s", addr, eligible, state, stake, reason)
	}()

	if addr == "" {
		logErr = "missing address"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": logErr})
		return
	}

	wlMu.RLock()
	epoch := currentEpoch
	wlMu.RUnlock()
	if epStr := r.URL.Query().Get("epoch"); epStr != "" {
		if ep, err := strconv.Atoi(epStr); err == nil {
			epoch = ep
		}
	}
	state, stake, penalized, flip, ok := getEpochSnapshot(epoch, addr)
	if ok {
		valid = true
		if penalized {
			reason = "Validation penalty"
			rule = "penalty"
		} else if flip {
			reason = "Flip reported"
			rule = "flip"
		} else if eligibility.IsEligibleSnapshot(state, stake, stakeThreshold) {
			eligible = true
			rule = "snapshot"
		} else {
			reason = fmt.Sprintf("Not eligible in snapshot: %s %.0f", state, stake)
		}
	} else {
		reason = "Address not in snapshot"
	}

	liveState, liveStake := identityFetcher(addr)
	hint = nextEpochHint(liveState, liveStake, stakeThreshold)

	writeJSON(w, map[string]interface{}{
		"eligible": eligible,
		"valid":    valid,
		"state":    state,
		"stake":    stake,
		"reason":   reason,
		"rule":     rule,
		"hint":     hint,
	})
}

// eligibilitySnapshotHandler returns eligibility info from the last finalized epoch snapshot.
func eligibilitySnapshotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	addr := strings.ToLower(r.URL.Query().Get("address"))
	if addr == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}

	epochStr := r.URL.Query().Get("epoch")
	var epoch, block int
	var err error
	if epochStr == "" {
		epoch, block = latestSnapshotEpoch()
	} else {
		epoch, err = strconv.Atoi(epochStr)
		if err != nil {
			http.Error(w, "invalid epoch", http.StatusBadRequest)
			return
		}
		block = getSnapshotBlock(epoch)
	}

	if epoch == 0 {
		json.NewEncoder(w).Encode(EligibilityResponse{Reason: "eligibility unknown \u2013 snapshot not taken yet."})
		return
	}

	state, stake, penalized, flip, ok := getEpochSnapshot(epoch, addr)
	if !ok {
		json.NewEncoder(w).Encode(EligibilityResponse{Reason: "eligibility unknown \u2013 snapshot not taken yet.", Epoch: epoch, Block: block})
		return
	}

	resp := EligibilityResponse{State: state, Stake: stake, Epoch: epoch, Block: block}
	if penalized {
		resp.Reason = "Validation penalty"
	} else if flip {
		resp.Reason = "Flip reported"
	} else if eligibility.IsEligibleSnapshot(state, stake, stakeThreshold) {
		resp.Eligible = true
	} else {
		resp.Reason = fmt.Sprintf("Not eligible in snapshot: %s %.0f", state, stake)
	}

	liveState, liveStake := identityFetcher(addr)
	resp.Prediction = predictNextEpoch(resp.Eligible, liveState, liveStake, stakeThreshold)

	json.NewEncoder(w).Encode(resp)
}

func merkleRootHandler(w http.ResponseWriter, r *http.Request) {
	wlMu.RLock()
	epoch := currentEpoch
	wlMu.RUnlock()
	if epStr := r.URL.Query().Get("epoch"); epStr != "" {
		if ep, err := strconv.Atoi(epStr); err == nil {
			epoch = ep
		}
	}

	root, ok := getMerkleRoot(epoch)
	if !ok {
		list, fileRoot, err := loadWhitelistData(epoch)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if fileRoot != "" {
			root = fileRoot
		} else {
			root = computeMerkleRoot(list)
		}
		saveMerkleRoot(epoch, root)
	}
	writeJSON(w, map[string]interface{}{"merkle_root": root, "epoch": epoch})
}

func merkleProofHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[MERKLE_PROOF][PANIC] %v\n%s", rec, debug.Stack())
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}()

	addr := strings.ToLower(r.URL.Query().Get("address"))
	if addr == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}

	wlMu.RLock()
	epoch := currentEpoch
	wlMu.RUnlock()
	if epStr := r.URL.Query().Get("epoch"); epStr != "" {
		if ep, err := strconv.Atoi(epStr); err == nil {
			epoch = ep
		}
	}

	var list []string
	var root string
	var err error
	if epoch == currentEpoch {
		list, err = getWhitelist()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		root, _ = getMerkleRoot(epoch)
	} else {
		list, root, err = loadWhitelistData(epoch)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	if root == "" {
		root = computeMerkleRoot(list)
	}

	proof, ok := computeMerkleProof(list, addr)
	if !ok {
		http.Error(w, "address not found", http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]interface{}{
		"merkle_root": root,
		"proof":       proof,
		"epoch":       epoch,
	})
}

func logsStreamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	ch := make(chan string, 100)
	logSubsMu.Lock()
	logSubs[ch] = struct{}{}
	logSubsMu.Unlock()
	defer func() {
		logSubsMu.Lock()
		delete(logSubs, ch)
		logSubsMu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case line := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func epochsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT epoch FROM epoch_merkle_roots ORDER BY epoch DESC LIMIT 20")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var eps []int
	for rows.Next() {
		var e int
		if err := rows.Scan(&e); err == nil {
			eps = append(eps, e)
		}
	}
	if len(eps) == 0 {
		wlMu.RLock()
		cur := currentEpoch
		wlMu.RUnlock()
		for i := 0; i < 10 && cur-i > 0; i++ {
			eps = append(eps, cur-i)
		}
	}
	writeJSON(w, eps)
}

// Verify Ethereum signature from Idena App
func verifySignature(nonce, address, signatureHex string) bool {
	sig, err := hex.DecodeString(strings.TrimPrefix(signatureHex, "0x"))
	if err != nil || len(sig) != 65 {
		log.Printf("[VERIFY] Signature format error")
		return false
	}
	msg := crypto.Keccak256([]byte(nonce))
	hash := crypto.Keccak256(msg)
	pubKey, err := crypto.SigToPub(hash, sig)
	if err != nil {
		log.Printf("[VERIFY] Signature recovery failed: %v", err)
		return false
	}
	recoveredAddr := crypto.PubkeyToAddress(*pubKey).Hex()
	match := strings.EqualFold(recoveredAddr, address)
	log.Printf("[VERIFY] Expected: %s, Recovered: %s, Match: %t", address, recoveredAddr, match)
	return match
}

// Get identity from node or public API as fallback
func getIdentity(address string) (string, float64) {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_identity",
		"params":  []string{address},
		"id":      1,
	}
	if IDENA_RPC_KEY != "" {
		rpcReq["key"] = IDENA_RPC_KEY
	}
	body, _ := json.Marshal(rpcReq)
	req, _ := http.NewRequest("POST", idenaRpcUrl, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == 200 {
		var rpcResp struct {
			Result struct {
				State string  `json:"state"`
				Stake float64 `json:"stake,string"`
			} `json:"result"`
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
		if rpcResp.Error.Message == "" || rpcResp.Error.Code == 0 {
			if rpcResp.Result.State != "" {
				log.Printf("[IDENTITY][RPC] Success: state=%s, stake=%.3f", rpcResp.Result.State, rpcResp.Result.Stake)
				return rpcResp.Result.State, rpcResp.Result.Stake
			}
		}
		if rpcResp.Error.Message != "" {
			log.Printf("[IDENTITY][RPC] Node returned error: %+v", rpcResp.Error)
		}
	} else {
		log.Printf("[IDENTITY][RPC] RPC call failed: %v", err)
	}
	log.Printf("[IDENTITY][FALLBACK] Using public indexer for %s", address)
	var state string
	resp2, err := http.Get(fallbackApiUrl + "/api/Identity/" + address)
	if err == nil && resp2.StatusCode == 200 {
		var apiResp struct {
			Result struct {
				State string `json:"state"`
			} `json:"result"`
		}
		_ = json.NewDecoder(resp2.Body).Decode(&apiResp)
		state = apiResp.Result.State
	}
	var stake float64
	resp3, err := http.Get(fallbackApiUrl + "/api/Address/" + address)
	if err == nil && resp3.StatusCode == 200 {
		var addrResp struct {
			Result struct {
				Stake string `json:"stake"`
			} `json:"result"`
		}
		_ = json.NewDecoder(resp3.Body).Decode(&addrResp)
		stake, _ = strconv.ParseFloat(addrResp.Result.Stake, 64)
	}
	log.Printf("[IDENTITY][FALLBACK] Indexer: state=%s, stake=%.3f", state, stake)
	return state, stake
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// evaluateEligibility checks signature validity, identity state and stake
// against the configured threshold. It returns whether the user is eligible
// and a human readable reason for the result.
func evaluateEligibility(sigOK bool, state string, stake float64) (bool, string) {
	reasons := []string{}
	if !sigOK {
		reasons = append(reasons, "Invalid signature.")
	}
	if state == "" {
		reasons = append(reasons, "Identity not found or status undefined.")
	} else if state != "Human" && state != "Verified" && state != "Newbie" {
		reasons = append(reasons, fmt.Sprintf("Identity state %s is not eligible.", state))
	}
	if stake < stakeThreshold {
		reasons = append(reasons, fmt.Sprintf("Stake too low: %.3f (%.3f required).", stake, stakeThreshold))
	}
	if len(reasons) == 0 {
		return true, "Eligible for login."
	}
	return false, strings.Join(reasons, " ")
}

// Clean up expired sessions regularly
func cleanupExpiredSessions() {
	for {
		_, _ = db.Exec("DELETE FROM sessions WHERE created < ?", time.Now().Add(-1*time.Hour).Unix())
		cleanupOldSnapshots()
		exportWhitelist()
		log.Println("[CLEANUP] housekeeping done")
		time.Sleep(15 * time.Minute)
	}
}

// Helper: write JSON with application/json
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Helper: write Idena protocol error response
func writeError(w http.ResponseWriter, msg string) {
	writeJSON(w, map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}

// callLocalRPC performs a JSON-RPC POST request to the configured Idena node.
// The response body is decoded into out.
func callLocalRPC(method string, params interface{}, out interface{}) error {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	if IDENA_RPC_KEY != "" {
		reqBody["key"] = IDENA_RPC_KEY
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, idenaRpcUrl, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// getCachedEpoch retrieves the most recent epoch info from the database.
func getCachedEpoch() (int, int64, float64, int64, bool) {
	row := db.QueryRow("SELECT epoch, validationTime, discriminationStakeThreshold, ts FROM epoch ORDER BY ts DESC LIMIT 1")
	var epoch int
	var vt int64
	var thr float64
	var ts int64
	if err := row.Scan(&epoch, &vt, &thr, &ts); err == nil {
		return epoch, vt, thr, ts, true
	}
	return 0, 0, 0, 0, false
}

// saveEpoch stores epoch info in the database.
func saveEpoch(epoch int, vt int64, thr float64) {
	_, _ = db.Exec("INSERT INTO epoch(epoch,validationTime,discriminationStakeThreshold,ts) VALUES(?,?,?,?)", epoch, vt, thr, time.Now().Unix())
}

// fetchEpochFromNode queries the local node for epoch information.
func fetchEpochFromNode() (int, int64, float64, error) {
	var resp struct {
		Result struct {
			Epoch          int     `json:"epoch"`
			ValidationTime string  `json:"validationTime"`
			Threshold      float64 `json:"discriminationStakeThreshold"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := callLocalRPC("bcn_lastBlock", []interface{}{}, &resp); err != nil {
		return 0, 0, 0, err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return 0, 0, 0, fmt.Errorf(resp.Error.Message)
	}
	vt, _ := time.Parse(time.RFC3339, resp.Result.ValidationTime)
	return resp.Result.Epoch, vt.Unix(), resp.Result.Threshold, nil
}

// fetchEpochFromAPI gets epoch info from the public API.
func fetchEpochFromAPI() (int, int64, float64, error) {
	resp, err := http.Get(fallbackApiUrl + "/api/Epoch/Last")
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	var apiResp struct {
		Result struct {
			Epoch          int     `json:"epoch"`
			ValidationTime string  `json:"validationTime"`
			Threshold      float64 `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return 0, 0, 0, err
	}
	vt, _ := time.Parse(time.RFC3339, apiResp.Result.ValidationTime)
	return apiResp.Result.Epoch, vt.Unix(), apiResp.Result.Threshold, nil
}

// updateEpochCache tries to refresh epoch info from the node, falling back to the public API.
func updateEpochCache() (int, int64, float64, error) {
	epoch, vt, thr, err := fetchEpochFromNode()
	if err != nil || epoch == 0 {
		epoch, vt, thr, err = fetchEpochFromAPI()
	}
	if err == nil && epoch != 0 {
		saveEpoch(epoch, vt, thr)
	}
	return epoch, vt, thr, err
}

// getCachedIdentity returns the latest cached identity record for an address.
func getCachedIdentity(addr string) (string, float64, int64, bool) {
	row := db.QueryRow("SELECT state, stake, ts FROM identity_snapshots WHERE address=? ORDER BY ts DESC LIMIT 1", addr)
	var state string
	var stake float64
	var ts int64
	if err := row.Scan(&state, &stake, &ts); err == nil {
		return state, stake, ts, true
	}
	return "", 0, 0, false
}

// getEpochSnapshot retrieves the stored snapshot for an address in a given epoch.
// It returns state, stake, penalty flags and a boolean indicating if a record exists.
func getEpochSnapshot(epoch int, addr string) (string, float64, bool, bool, bool) {
	row := db.QueryRow(
		"SELECT state, stake, penalized, flipReported FROM epoch_identity_snapshot WHERE epoch=? AND address=?",
		epoch, strings.ToLower(addr),
	)
	var state string
	var stake float64
	var pen, flip int
	if err := row.Scan(&state, &stake, &pen, &flip); err == nil {
		return state, stake, pen != 0, flip != 0, true
	}
	return "", 0, false, false, false
}

// fetchIdentityFromNode queries the local node for an identity.
func fetchIdentityFromNode(addr string) (string, float64, error) {
	var resp struct {
		Result struct {
			State string  `json:"state"`
			Stake float64 `json:"stake,string"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := callLocalRPC("dna_identity", []interface{}{addr}, &resp); err != nil {
		return "", 0, err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return "", 0, fmt.Errorf(resp.Error.Message)
	}
	if resp.Result.State == "" {
		return "", 0, fmt.Errorf("empty state")
	}
	return resp.Result.State, resp.Result.Stake, nil
}

// fetchIdentityFromAPI queries the public API for identity state and stake.
func fetchIdentityFromAPI(addr string) (string, float64, error) {
	var state string
	resp, err := http.Get(fallbackApiUrl + "/api/Identity/" + addr)
	if err == nil && resp.StatusCode == http.StatusOK {
		var apiResp struct {
			Result struct {
				State string `json:"state"`
			} `json:"result"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiResp)
		state = apiResp.Result.State
	}
	resp2, err2 := http.Get(fallbackApiUrl + "/api/Address/" + addr)
	var stake float64
	if err2 == nil && resp2.StatusCode == http.StatusOK {
		var addrResp struct {
			Result struct {
				Stake string `json:"stake"`
			} `json:"result"`
		}
		_ = json.NewDecoder(resp2.Body).Decode(&addrResp)
		stake, _ = strconv.ParseFloat(addrResp.Result.Stake, 64)
	}
	if state == "" && stake == 0 {
		return "", 0, fmt.Errorf("api error")
	}
	return state, stake, nil
}

// updateIdentityCache refreshes the cached identity information.
func updateIdentityCache(addr string) (string, float64, error) {
	state, stake, err := fetchIdentityFromNode(addr)
	if err != nil || state == "" {
		state, stake, err = fetchIdentityFromAPI(addr)
	}
	if err == nil && state != "" {
		recordIdentitySnapshot(addr, state, stake)
	}
	return state, stake, err
}

func fetchValidationPenalty(epoch int, addr string) (bool, error) {
	url := fmt.Sprintf("%s/api/Epoch/%d/Identity/%s/ValidationSummary", fallbackApiUrl, epoch, addr)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %s", resp.Status)
	}
	var out struct {
		Result struct {
			Penalized bool   `json:"penalized"`
			Reason    string `json:"penaltyReason"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	if out.Result.Penalized {
		return true, nil
	}
	return false, nil
}

func getPenaltyStatus(epoch int, addr string) bool {
	pen, _, err := checks.CheckPenaltyFlipForEpoch(fallbackApiUrl, IDENA_RPC_KEY, epoch, addr)
	if err != nil {
		log.Printf("[PENALTY] fetch %s epoch %d: %v", addr, epoch, err)
		return false
	}
	if pen {
		recordPenalty(epoch, addr)
	}
	return pen
}

// hasFlipReport checks lastValidationFlags for AtLeastOneFlipReported for the given epoch.
// 2025-06-13 ticket #42
func hasFlipReport(epoch int, addr string) bool {
	_, flip, err := checks.CheckPenaltyFlipForEpoch(fallbackApiUrl, IDENA_RPC_KEY, epoch, addr)
	if err != nil {
		log.Printf("[FLIP] %s epoch %d: %v", addr, epoch, err)
	}
	return flip
}

// epochLastHandler serves the /api/Epoch/Last endpoint.
func epochLastHandler(w http.ResponseWriter, r *http.Request) {
	epoch, vt, thr, ts, ok := getCachedEpoch()
	if ok && time.Since(time.Unix(ts, 0)) < 5*time.Minute {
		writeJSON(w, map[string]interface{}{
			"result": map[string]interface{}{
				"epoch":                        epoch,
				"validationTime":               time.Unix(vt, 0).UTC().Format(time.RFC3339),
				"discriminationStakeThreshold": fmt.Sprintf("%.8f", thr),
			},
		})
		return
	}
	if ok {
		go updateEpochCache()
		writeJSON(w, map[string]interface{}{
			"result": map[string]interface{}{
				"epoch":                        epoch,
				"validationTime":               time.Unix(vt, 0).UTC().Format(time.RFC3339),
				"discriminationStakeThreshold": fmt.Sprintf("%.8f", thr),
			},
		})
		return
	}
	epoch, vt, thr, err := updateEpochCache()
	if err != nil {
		writeJSON(w, map[string]string{"error": "failed to fetch epoch"})
		return
	}
	writeJSON(w, map[string]interface{}{
		"result": map[string]interface{}{
			"epoch":                        epoch,
			"validationTime":               time.Unix(vt, 0).UTC().Format(time.RFC3339),
			"discriminationStakeThreshold": fmt.Sprintf("%.8f", thr),
		},
	})
}

// identityHandler serves the /api/Identity/{address} endpoint.
func identityHandler(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimPrefix(r.URL.Path, "/api/Identity/")
	addr = strings.ToLower(addr)
	if addr == "" {
		http.Error(w, "bad address", http.StatusBadRequest)
		return
	}
	state, stake, ts, ok := getCachedIdentity(addr)
	if ok && time.Since(time.Unix(ts, 0)) < 30*24*time.Hour {
		if time.Since(time.Unix(ts, 0)) > 24*time.Hour {
			go updateIdentityCache(addr)
		}
		writeJSON(w, map[string]interface{}{
			"result": map[string]interface{}{
				"address": addr,
				"state":   state,
				"stake":   fmt.Sprintf("%.8f", stake),
			},
		})
		return
	}
	state, stake, err := updateIdentityCache(addr)
	if err != nil && ok {
		writeJSON(w, map[string]interface{}{
			"result": map[string]interface{}{
				"address": addr,
				"state":   state,
				"stake":   fmt.Sprintf("%.8f", stake),
			},
		})
		return
	}
	if err != nil {
		writeJSON(w, map[string]string{"error": "failed to fetch identity"})
		return
	}
	writeJSON(w, map[string]interface{}{
		"result": map[string]interface{}{
			"address": addr,
			"state":   state,
			"stake":   fmt.Sprintf("%.8f", stake),
		},
	})
}

// runIndexerCLI builds the whitelist for the given epoch and prints the Merkle root.
// If epoch is 0, the latest epoch from the node is used.
func runIndexerCLI(epoch int) {
	var err error
	db, err = sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	createSessionTable()
	createSnapshotTable()
	createEpochSnapshotTable()
	createSnapshotMetaTable()
	createConfigTable()
	createEpochTable()
	createMerkleRootTable()
	createPenaltyTable()

	ep, thr, err := fetchEpochData()
	if err != nil {
		log.Fatalf("fetch epoch: %v", err)
	}
	if epoch > 0 {
		ep = epoch
	}
	stakeThreshold = thr
	if err := buildEpochWhitelist(ep, thr); err != nil {
		log.Fatalf("build whitelist: %v", err)
	}
	saveSnapshotMeta(ep, 0)
	root, _ := getMerkleRoot(ep)
	fmt.Printf("epoch %d merkle root %s\n", ep, root)
}
