package main

import (
    "bytes"
    "crypto/rand"
    "crypto/sha256"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "html/template"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "time"
    "idenauthgo/agents" // If using modules; may need path adjustment
    _ "github.com/mattn/go-sqlite3"
    "github.com/ethereum/go-ethereum/crypto"
)

// Environment variables, with fallback for local/dev usage
var (
	BASE_URL      = getenv("BASE_URL", "http://proofofhuman.work")
	IDENA_RPC_KEY = getenv("IDENA_RPC_KEY", "")
)

const (
	sessionDuration = 60 * 60 // Session duration in seconds
	listenAddr      = ":3030"
	dbFile          = "./sessions.db"
	idenaRpcUrl     = "http://localhost:9009"
	fallbackApiUrl  = "https://api.idena.io"
)

var (
	db             *sql.DB
	stakeThreshold = 10000.0
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

func getenv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func fetchStakeThreshold() {
	url := idenaRpcUrl + "/api/Epoch/Last"
	if IDENA_RPC_KEY != "" {
		url += "?apikey=" + IDENA_RPC_KEY
	}
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[THRESHOLD] fetch error: %v", err)
		return
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Threshold string `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if v, err := strconv.ParseFloat(result.Result.Threshold, 64); err == nil {
			stakeThreshold = v
			log.Printf("[THRESHOLD] Updated stake threshold: %.3f", stakeThreshold)
		}
	}
}

func main() {
	go agents.RunIdentityFetcher("agents/fetcher_config.json")
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var err error
	db, err = sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()
	createSessionTable()
	createSnapshotTable()
	fetchStakeThreshold()
	exportWhitelist()

	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/signin", signinHandler)
	http.HandleFunc("/auth/v1/start-session", startSessionHandler)
	http.HandleFunc("/auth/v1/authenticate", authenticateHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/whitelist", whitelistHandler)
	http.HandleFunc("/whitelist/check", whitelistCheckHandler)
	http.HandleFunc("/merkle_root", merkleRootHandler)

	go cleanupExpiredSessions()
	log.Printf("Server running at http://localhost%s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
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

func getWhitelist() ([]string, error) {
	rows, err := db.Query(`SELECT address FROM identity_snapshots WHERE ts >= ? AND (state='Human' OR state='Verified' OR state='Newbie') AND stake>=? GROUP BY address`,
		time.Now().AddDate(0, 0, -30).Unix(), stakeThreshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err == nil {
			list = append(list, addr)
		}
	}
	sort.Strings(list)
	return list, nil
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
	os.WriteFile("data/whitelist.json", b, 0644)
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
	log.Printf("[NONCE_ENDPOINT] Called: %s %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[NONCE_ENDPOINT][POST] Failed to read body: %v", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		log.Printf("[NONCE_ENDPOINT][POST] Request body: %s", string(body))
		r.Body = io.NopCloser(bytes.NewBuffer(body)) // Allow reuse

		var req struct {
			Token   string `json:"token"`
			Address string `json:"address"`
		}
		if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
			log.Printf("[NONCE_ENDPOINT][POST] Invalid body: %v", err)
			writeError(w, "Invalid request")
			return
		}
		nonce := "signin-" + randHex(16)
		_, err = db.Exec("UPDATE sessions SET address=?, nonce=? WHERE token=?", req.Address, nonce, req.Token)
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
		log.Printf("[NONCE_ENDPOINT][GET] Called - not standard flow")
		http.Error(w, "Not implemented", http.StatusNotImplemented)
	default:
		log.Printf("[NONCE_ENDPOINT][%s] Method not allowed", r.Method)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// Authenticate nonce signature
func authenticateHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[AUTH][RAW] %s %s", r.Method, r.URL.String())
	bodyBytes, _ := io.ReadAll(r.Body)
	log.Printf("[AUTH][BODY] %s", string(bodyBytes))
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	var req struct {
		Token     string `json:"token"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[AUTH] Invalid request body: %v", err)
		writeError(w, "Bad request")
		return
	}

	row := db.QueryRow("SELECT nonce, address FROM sessions WHERE token=?", req.Token)
	var nonce, address string
	if err := row.Scan(&nonce, &address); err != nil {
		log.Printf("[AUTH] Token not found: %s", req.Token)
		writeError(w, "Session not found")
		return
	}
	log.Printf("[AUTH] Authenticating address: %s for token: %s with nonce: %s", address, req.Token, nonce)

	authenticated := verifySignature(nonce, address, req.Signature)
	if !authenticated {
		log.Printf("[AUTH] Signature verification failed for address %s", address)
	}

	state, stake := getIdentity(address)
	isEligible := authenticated && (state == "Newbie" || state == "Verified" || state == "Human") && stake >= stakeThreshold
	log.Printf("[AUTH] Identity state: %s, stake: %.3f, eligible: %t", state, stake, isEligible)

	_, _ = db.Exec(`UPDATE sessions SET authenticated=?, identity_state=?, stake=? WHERE token=?`,
		boolToInt(isEligible), state, stake, req.Token)
	recordIdentitySnapshot(address, state, stake)
	exportWhitelist()

	writeJSON(w, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"authenticated": isEligible,
		},
	})
}

// Show result, log User-Agent, all params
func callbackHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	log.Printf("[CALLBACK] Request params: %v", r.URL.Query())
	log.Printf("[CALLBACK] User-Agent: %s", r.Header.Get("User-Agent"))
	row := db.QueryRow("SELECT address, authenticated, identity_state, stake FROM sessions WHERE token=?", token)
	var address, state string
	var authenticated int
	var stake float64
	err := row.Scan(&address, &authenticated, &state, &stake)
	if err != nil {
		log.Printf("[CALLBACK] Token not found: %s", token)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	log.Printf("[CALLBACK] Rendering result for address: %s, state: %s, stake: %.3f", address, state, stake)

	data := struct {
		Headline string
		Message  string
		BaseUrl  string
	}{
		BaseUrl: BASE_URL,
	}

	if authenticated == 1 {
		data.Headline = "Access granted!"
		data.Message = fmt.Sprintf(`Address: <b>%s</b><br>Status: <b>%s</b><br>Stake: <b>%.3f</b>`, address, state, stake)
	} else {
		data.Headline = "Access denied!"
		data.Message = fmt.Sprintf(`Address: <b>%s</b><br>Status: <b>%s</b><br>Stake: <b>%.3f</b>`, address, state, stake)
	}

	log.Printf("[CALLBACK] Rendering HTML: Headline=%s, Message=%s", data.Headline, data.Message)
	tmpl := mustLoadTemplate("templates/result.html")
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("[CALLBACK][ERROR] Template rendering failed: %v", err)
		http.Error(w, "Template error: "+err.Error(), 500)
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

// Check if address is eligible
func whitelistCheckHandler(w http.ResponseWriter, r *http.Request) {
	addr := strings.ToLower(r.URL.Query().Get("address"))
	list, err := getWhitelist()
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	found := false
	for _, a := range list {
		if strings.ToLower(a) == addr {
			found = true
			break
		}
	}
	writeJSON(w, map[string]bool{"eligible": found})
}

func merkleRootHandler(w http.ResponseWriter, r *http.Request) {
	list, err := getWhitelist()
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	writeJSON(w, map[string]string{"merkle_root": computeMerkleRoot(list)})
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
