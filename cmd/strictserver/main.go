package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"idenauthgo/strictlocal"
)

var (
	nodeURL = "http://localhost:9009"
	// IDENA_RPC_KEY must be set in the environment if the node requires it.
	// Never hardcode or log this value.
	apiKey      = os.Getenv("IDENA_RPC_KEY")
	dbPath      = ""
	currentFile = ""
)

func runSnapshot(w http.ResponseWriter, r *http.Request) {
	log.Println("/snapshot requested")
	if err := strictlocal.BuildWhitelist(nodeURL, apiKey, dbPath); err != nil {
		log.Printf("snapshot error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentFile = latestFile()
	w.Write([]byte("ok"))
}

func latestFile() string {
	files, _ := filepath.Glob("data/whitelist_epoch_*.jsonl")
	if len(files) == 0 {
		return ""
	}
	latest := files[len(files)-1]
	log.Printf("using whitelist file %s", latest)
	return latest
}

func serveCurrent(w http.ResponseWriter, r *http.Request) {
	if currentFile == "" {
		currentFile = latestFile()
	}
	if currentFile == "" {
		http.Error(w, "no whitelist", 404)
		return
	}
	log.Printf("serving current whitelist %s", currentFile)
	http.ServeFile(w, r, currentFile)
}

func serveEpoch(w http.ResponseWriter, r *http.Request) {
	ep := filepath.Base(r.URL.Path)
	file := filepath.Join("data", "whitelist_epoch_"+ep+".jsonl")
	log.Printf("serving epoch whitelist %s", file)
	http.ServeFile(w, r, file)
}

func checkAddr(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	if addr == "" {
		http.Error(w, "missing address", 400)
		return
	}
	if currentFile == "" {
		currentFile = latestFile()
	}
	f, err := os.Open(currentFile)
	if err != nil {
		log.Printf("open whitelist error: %v", err)
		http.Error(w, "no whitelist", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var rec strictlocal.IdentityInfo
		if err := dec.Decode(&rec); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("decode error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.EqualFold(rec.Address, addr) {
			json.NewEncoder(w).Encode(rec)
			return
		}
	}
	http.NotFound(w, r)
}

func download(w http.ResponseWriter, r *http.Request) {
	if currentFile == "" {
		currentFile = latestFile()
	}
	if currentFile == "" {
		http.Error(w, "no whitelist", 404)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=whitelist.jsonl")
	log.Printf("download %s", currentFile)
	http.ServeFile(w, r, currentFile)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot", runSnapshot)
	mux.HandleFunc("/whitelist/current", serveCurrent)
	mux.HandleFunc("/whitelist/epoch/", serveEpoch)
	mux.HandleFunc("/whitelist/check", checkAddr)
	mux.HandleFunc("/whitelist/download", download)
	mux.Handle("/", http.FileServer(http.Dir("static")))
	log.Println("strictserver listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
