// Settings panel: API URL and epoch, robust error boundary, dark mode
/*
# Epoch-aware API download example
curl 'http://localhost:3030/whitelist/epoch/123' > whitelist_epoch123.json
# Fetch merkle proof for address 0x... at epoch 123
curl 'http://localhost:3030/merkle_proof?address=0x...&epoch=123'
*/

import React, { useState, useRef, useEffect } from "react";

// Error Boundary to catch render/runtime errors
class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }
  componentDidCatch(error, errorInfo) {
    // log error or send to analytics
    console.error(error, errorInfo);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div className="bg-red-100 border border-red-300 rounded p-8 mt-8 text-red-800">
          <h2 className="text-xl font-bold mb-2">An error occurred.</h2>
          <div className="mb-4">
            {this.state.error?.message || String(this.state.error)}
          </div>
          <button
            className="btn btn-accent"
            onClick={() => window.location.reload()}
          >
            Reload App
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

// Minimal toast popup
function Toast({ message, type, onClose }) {
  if (!message) return null;
  const color = type === "success" ? "bg-green-600" : "bg-red-700";
  return (
    <div
      className={`fixed top-4 right-4 z-50 rounded text-white px-4 py-2 shadow-lg ${color}`}
    >
      {message}
      <button className="ml-4 text-lg" onClick={onClose}>
        ×
      </button>
    </div>
  );
}

export default function MerkleWhitelistGenerator() {
  // --- Settings state ---
  const [apiBase, setApiBase] = useState(
    localStorage.getItem("idenaApiBase") || "http://localhost:3030",
  );
  const [epoch, setEpoch] = useState(null);
  const [epochs, setEpochs] = useState([]);
  const [selectedEpoch, setSelectedEpoch] = useState(null);
  const [latestEpoch, setLatestEpoch] = useState(null);
  const [apiStatus, setApiStatus] = useState(null); // 'ok' | 'error' | null
  const [darkMode, setDarkMode] = useState(
    localStorage.getItem("darkMode") === "true",
  );

  // --- UI state ---
  const [merkleRoot, setMerkleRoot] = useState("");
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [address, setAddress] = useState("");
  const [eligibilityResult, setEligibilityResult] = useState(null);
  const [error, setError] = useState(null);
  const [toast, setToast] = useState({ message: "", type: "success" });
  const [whitelistData, setWhitelistData] = useState(null);
  const logRef = useRef(null);
  const eventSourceRef = useRef(null);
  const [logConnected, setLogConnected] = useState(true);

  const getStoredToken = () => sessionStorage.getItem("idenaApiToken") || "";
  const [token, setToken] = useState(getStoredToken());
  const [authInput, setAuthInput] = useState("");
  const [authStatus, setAuthStatus] = useState("");

  // Utility to append a log line
  const appendLog = (line) => {
    setLogs((prev) => [...prev, line]);
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  };

  const fetchWithAuth = (url, options = {}) => {
    const opts = { ...options };
    opts.headers = opts.headers || {};
    if (token) opts.headers["Authorization"] = `Bearer ${token}`;
    return fetch(url, opts);
  };

  const handleLogin = (e) => {
    e.preventDefault();
    fetch(`${apiBase}/whoami`, {
      headers: { Authorization: `Bearer ${authInput}` },
    })
      .then((res) => (res.ok ? res.json() : Promise.reject()))
      .then((user) => {
        setToken(authInput);
        sessionStorage.setItem("idenaApiToken", authInput);
        setAuthStatus(
          user?.username ? `Logged in as ${user.username}` : "Logged in",
        );
      })
      .catch(() => setAuthStatus("Invalid key"));
  };

  const handleLogout = () => {
    setToken("");
    sessionStorage.removeItem("idenaApiToken");
    setAuthStatus("");
  };

  // Color map for status badges
  const statusColors = {
    Human: "bg-green-200 text-green-800",
    Verified: "bg-blue-200 text-blue-800",
    Newbie: "bg-yellow-100 text-yellow-800",
    Suspended: "bg-red-200 text-red-800",
    Zombie: "bg-gray-200 text-gray-700",
    Killed: "bg-gray-200 text-gray-700",
    Undefined: "bg-gray-100 text-gray-500",
  };

  // Format stake (thousands separator)
  function formatStake(stake) {
    if (!stake) return "";
    return (
      Number(stake).toLocaleString("en-US", { maximumFractionDigits: 3 }) +
      " iDNA"
    );
  }

  // --- Dark mode handling ---
  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add("dark");
      localStorage.setItem("darkMode", "true");
    } else {
      document.documentElement.classList.remove("dark");
      localStorage.setItem("darkMode", "false");
    }
  }, [darkMode]);

  // --- API status ping ---
  useEffect(() => {
    setApiStatus(null);
    fetchWithAuth(`${apiBase}/merkle_root`)
      .then((res) => (res.ok ? setApiStatus("ok") : setApiStatus("error")))
      .catch(() => setApiStatus("error"));
  }, [apiBase, token]);

  // --- Log streaming with reconnect ---
  useEffect(() => {
    let es;
    let timer;
    function connect() {
      if (es) es.close();
      es = new window.EventSource(`${apiBase}/logs/stream`);
      eventSourceRef.current = es;
      setLogConnected(true);
      es.onmessage = (e) => {
        if (e.data === "[DONE]") {
          fetchMerkleRoot();
          return;
        }
        appendLog(e.data);
      };
      es.onerror = () => {
        setLogConnected(false);
        appendLog("[Log stream lost, reconnecting…]");
        timer = setTimeout(() => {
          appendLog("[Attempting to reconnect log stream…]");
          connect();
        }, 5000);
      };
    }
    connect();
    return () => {
      if (es) es.close();
      clearTimeout(timer);
    };
  }, [apiBase]);

  // --- Fetch available epochs ---
  useEffect(() => {
    fetchWithAuth(`${apiBase}/epochs`)
      .then((res) => res.json())
      .then((list) => {
        setEpochs(list);
        setLatestEpoch(list[0] || null);
        if (!selectedEpoch) setSelectedEpoch(list[0]);
      })
      .catch(() => {
        fetchWithAuth(`${apiBase}/merkle_root`)
          .then((res) => res.json())
          .then((data) => {
            const latest = data.epoch;
            setLatestEpoch(latest);
            setEpochs(Array.from({ length: 10 }, (_, i) => latest - i));
            if (!selectedEpoch) setSelectedEpoch(latest);
          });
      });
  }, [apiBase, token]);

  // --- Save API base ---
  const handleApiBaseChange = (v) => {
    setApiBase(v);
    localStorage.setItem("idenaApiBase", v);
  };

  // --- Epoch handling ---
  const epochParam = selectedEpoch;

  // --- Whitelist generation with log streaming ---
  const handleGenerate = async (source) => {
    setLoading(true);
    setLogs([]);
    setMerkleRoot("");
    setEpoch(null);
    setError(null);
    try {
      await fetchWithAuth(`${apiBase}/generate_merkle?source=${source}`, {
        method: "POST",
      });
    } catch (err) {
      setError("Failed to start whitelist generation");
      setLoading(false);
    }
  };

  // --- Fetch Merkle root after generation ---
  const fetchMerkleRoot = async () => {
    try {
      const res = await fetchWithAuth(`${apiBase}/merkle_root`);
      const data = await res.json();
      setMerkleRoot(data.merkle_root || "");
      setEpoch(data.epoch || null);
      appendLog(`[Merkle Root]: ${data.merkle_root} (Epoch ${data.epoch})`);
    } catch (err) {
      appendLog("[Error: Failed to fetch Merkle root]");
    }
  };

  // --- Address eligibility and proof check ---
  const handleCheck = async () => {
    setEligibilityResult(null);
    setError(null);
    try {
      const res = await fetchWithAuth(
        `${apiBase}/whitelist/check?address=${address}` +
          (epochParam ? `&epoch=${epochParam}` : ""),
      );
      const data = await res.json();
      let reasons = [];
      if (Array.isArray(data.reasons)) reasons = data.reasons;
      else if (data.reason) reasons = [data.reason];
      if (data.eligible) {
        const proofRes = await fetchWithAuth(
          `${apiBase}/merkle_proof?address=${address}` +
            (epochParam ? `&epoch=${epochParam}` : ""),
        );
        const proofData = await proofRes.json();
        setEligibilityResult({
          eligible: true,
          status: data.status,
          stake: data.stake,
          reasons: [],
          proof: proofData.proof || [],
        });
      } else {
        setEligibilityResult({
          eligible: false,
          status: data.status,
          stake: data.stake,
          reasons,
        });
      }
    } catch (err) {
      setError("Eligibility check failed. Check address and try again.");
    }
  };

  // --- Fetch whitelist as file ---
  const fetchWhitelist = async () => {
    try {
      const url = epochParam
        ? `${apiBase}/whitelist/epoch/${epochParam}`
        : `${apiBase}/whitelist/current`;
      const res = await fetchWithAuth(url);
      const data = await res.json();
      setWhitelistData(data);
      setToast({ message: "Whitelist downloaded", type: "success" });
      downloadAsFile(`whitelist_epoch${epochParam || epoch || ""}.json`, data);
    } catch (e) {
      setToast({ message: "Failed to download whitelist", type: "error" });
    }
  };

  // --- Fetch proof as file ---
  const fetchProof = async () => {
    try {
      const url =
        `${apiBase}/merkle_proof?address=${address}` +
        (epochParam ? `&epoch=${epochParam}` : "");
      const res = await fetchWithAuth(url);
      const data = await res.json();
      downloadAsFile(
        `merkle_proof_${address}_epoch${epochParam || epoch || ""}.json`,
        data.proof || [],
      );
      setToast({ message: "Proof downloaded", type: "success" });
    } catch {
      setToast({ message: "Failed to download proof", type: "error" });
    }
  };

  // --- Settings Panel UI ---
  function SettingsPanel() {
    return (
      <div className="mb-6 p-3 bg-gray-100 dark:bg-gray-800 rounded flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <label className="font-semibold">API URL:</label>
          <input
            className="flex-1 border rounded p-2 dark:bg-gray-900"
            value={apiBase}
            onChange={(e) => handleApiBaseChange(e.target.value)}
          />
          <span
            className={
              apiStatus === "ok"
                ? "ml-2 text-green-600"
                : apiStatus === "error"
                  ? "ml-2 text-red-700"
                  : "ml-2 text-gray-500"
            }
          >
            {apiStatus === "ok" && "✓ Connected"}
            {apiStatus === "error" && "⚠ Error"}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <label className="font-semibold">Epoch:</label>
          <select
            className="border rounded p-1"
            value={selectedEpoch || ""}
            onChange={(e) => setSelectedEpoch(Number(e.target.value))}
          >
            {epochs.map((e) => (
              <option key={e} value={e}>
                {e}
              </option>
            ))}
          </select>
          <span className="ml-2 text-xs text-gray-500">
            Latest: {latestEpoch}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {token ? (
            <span className="text-green-700">
              API key active.{" "}
              <button onClick={handleLogout} className="ml-2 underline">
                Logout
              </button>
            </span>
          ) : (
            <form onSubmit={handleLogin} className="flex items-center gap-2">
              <input
                type="password"
                value={authInput}
                onChange={(e) => setAuthInput(e.target.value)}
                placeholder="API Key"
                className="border rounded p-1"
              />
              <button className="btn btn-xs" type="submit">
                Login
              </button>
              {authStatus && <span className="ml-2 text-xs">{authStatus}</span>}
            </form>
          )}
        </div>
        <div className="flex items-center gap-2">
          <label className="font-semibold">Dark mode:</label>
          <button
            className={`px-2 py-1 rounded ${darkMode ? "bg-gray-700 text-white" : "bg-gray-200"}`}
            onClick={() => setDarkMode((d) => !d)}
          >
            {darkMode ? "🌙 Dark" : "☀️ Light"}
          </button>
        </div>
      </div>
    );
  }

  // --- Download utility ---
  function downloadAsFile(filename, data) {
    const blob = new Blob(
      [typeof data === "string" ? data : JSON.stringify(data, null, 2)],
      { type: "application/json" },
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  // Copy proof helper
  function copyProof(proof) {
    navigator.clipboard.writeText(JSON.stringify(proof));
  }

  // --- Main UI ---
  return (
    <ErrorBoundary>
      <div
        className={`max-w-xl mx-auto p-6 ${darkMode ? "dark bg-gray-900 text-gray-100" : ""}`}
      >
        {/* Toast popup */}
        <Toast
          message={toast.message}
          type={toast.type}
          onClose={() => setToast({ message: "", type: "success" })}
        />

        {/* Settings Panel */}
        <SettingsPanel />

        {/* Title */}
        <h1 className="text-2xl font-bold text-center mb-4">
          Idena Eligibility Discriminator – Generate Whitelist Merkle Root
        </h1>

        {/* Mode Buttons */}
        <div className="flex justify-center gap-4 mb-2">
          <button
            className="btn btn-primary"
            disabled={loading}
            onClick={() => handleGenerate("node")}
          >
            From your own node
          </button>
          <button
            className="btn btn-secondary"
            disabled={loading}
            onClick={() => handleGenerate("public")}
          >
            From the public indexer
          </button>
        </div>

        {/* Description */}
        <div className="text-gray-600 dark:text-gray-300 text-center mb-4">
          Checks and filters Idena identities by PoP rules (status and stake) to
          generate a deterministic whitelist for the current epoch. The result
          is a Merkle root and inclusion proofs for eligibility verification.
          You can use your own node or fall back to a public indexer.
        </div>

        {/* Live Log Panel */}
        <div className="flex items-center gap-2 mb-1">
          <span className="font-semibold">Log:</span>
          <span
            className={`text-xs ${logConnected ? "text-green-600" : "text-red-700"}`}
          >
            {logConnected ? "Live" : "Offline (reconnecting...)"}
          </span>
        </div>
        <div
          ref={logRef}
          className="bg-black text-green-300 font-mono rounded p-2 h-32 overflow-y-auto mb-4"
        >
          {logs.length === 0 ? (
            <span className="opacity-50">Console output will appear here…</span>
          ) : (
            logs.map((line, idx) => <div key={idx}>{line}</div>)
          )}
        </div>

        {/* Merkle Root Display */}
        <div className="mb-4 flex items-center gap-2">
          <input
            className="flex-1 border rounded p-2 dark:bg-gray-800"
            readOnly
            value={merkleRoot}
            placeholder="Merkle root will appear here…"
          />
          <button
            className="btn btn-outline"
            disabled={!merkleRoot}
            onClick={() => navigator.clipboard.writeText(merkleRoot)}
          >
            Copy
          </button>
        </div>
        <div className="flex gap-2 mb-4">
          <button
            className="btn btn-outline"
            onClick={fetchWhitelist}
            disabled={loading}
          >
            Download Whitelist
          </button>
          <button
            className="btn btn-outline"
            onClick={fetchProof}
            disabled={!address}
          >
            Download Proof
          </button>
        </div>

        {/* Address Checker */}
        <div className="bg-gray-50 dark:bg-gray-800 rounded p-4 mt-8">
          <h2 className="font-semibold mb-2">Check Address</h2>
          <div className="flex gap-2 mb-2">
            <input
              className="flex-1 border rounded p-2 dark:bg-gray-900"
              placeholder="0x..."
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
            <button className="btn btn-accent" onClick={handleCheck}>
              Check
            </button>
          </div>
          {eligibilityResult && (
            <div className="mt-2 border rounded-lg p-4 shadow bg-white dark:bg-gray-900">
              <div className="flex items-center gap-4 mb-2">
                <span
                  className={
                    "text-xl font-bold " +
                    (eligibilityResult.eligible
                      ? "text-green-700"
                      : "text-red-700")
                  }
                >
                  {eligibilityResult.eligible ? "Eligible" : "Not eligible"}
                </span>
                <span
                  className={
                    "inline-block px-3 py-1 rounded-full text-xs font-semibold " +
                    (statusColors[eligibilityResult.status] ||
                      statusColors.Undefined)
                  }
                >
                  {eligibilityResult.status || "Unknown"}
                </span>
              </div>
              <div className="mb-2">
                <span className="font-medium">Stake:&nbsp;</span>
                <span className="font-mono font-semibold">
                  {formatStake(eligibilityResult.stake)}
                </span>
              </div>
              {!eligibilityResult.eligible && (
                <div>
                  <div className="font-semibold text-red-700">
                    Exclusion reason(s):
                  </div>
                  <ul className="list-disc pl-6 text-sm">
                    {eligibilityResult.reasons?.length ? (
                      eligibilityResult.reasons.map((r, i) => (
                        <li key={i}>{r}</li>
                      ))
                    ) : (
                      <li>No reason given</li>
                    )}
                  </ul>
                </div>
              )}
              {eligibilityResult.eligible && (
                <div>
                  <div className="font-semibold mt-2">Merkle Proof:</div>
                  <pre className="bg-gray-100 dark:bg-gray-700 rounded p-2 text-xs max-h-32 overflow-x-auto mb-1">
                    {JSON.stringify(eligibilityResult.proof, null, 2)}
                  </pre>
                  <button
                    className="btn btn-outline btn-xs"
                    onClick={() => copyProof(eligibilityResult.proof)}
                  >
                    Copy Merkle Proof
                  </button>
                </div>
              )}
            </div>
          )}
          {error && <div className="text-red-600 mt-2">{error}</div>}
        </div>
      </div>
    </ErrorBoundary>
  );
}
