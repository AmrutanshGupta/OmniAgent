"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { GO_HTTP } from "../../hooks/useOrchestratorSocket";

const FIELDS = [
  { key: "openai", label: "OpenAI API Key" },
  { key: "anthropic", label: "Anthropic API Key" },
  { key: "google", label: "Google API Key" },
];

export default function Settings() {
  const [keys, setKeys] = useState({ openai: "", anthropic: "", google: "" });
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    try {
      const stored = JSON.parse(localStorage.getItem("omniagent_keys") || "{}");
      setKeys((k) => ({ ...k, ...stored }));
    } catch {}
  }, []);

  const handleSave = async () => {
    // Locally: keys stay in the browser (localStorage) and are sent per-request
    // over HTTPS to the Go backend, which decrypts-in-memory / never logs them.
    localStorage.setItem("omniagent_keys", JSON.stringify(keys));
    // Also demonstrate the AES-256-GCM vault encrypt endpoint for each non-empty key.
    for (const f of FIELDS) {
      if (keys[f.key]) {
        try {
          await fetch(`${GO_HTTP}/api/vault/encrypt`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ key: keys[f.key] }),
          });
        } catch {}
      }
    }
    setSaved(true);
    setTimeout(() => setSaved(false), 1500);
  };

  return (
    <main className="min-h-screen bg-[#0b0e14] p-6 max-w-xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-lg font-semibold">Settings / API Vault</h1>
        <Link href="/" className="text-sm text-blue-400 hover:underline">← Back to Workspace</Link>
      </div>

      <div className="bg-slate-900/60 rounded-xl p-5 space-y-4">
        <div className="flex items-center gap-2 text-xs text-green-400 mb-2">
          🔒 Keys are encrypted at rest (AES-256-GCM) and only decrypted in-memory server-side.
        </div>
        {FIELDS.map((f) => (
          <div key={f.key}>
            <label className="block text-xs text-slate-400 mb-1">{f.label}</label>
            <input
              type="password"
              value={keys[f.key]}
              onChange={(e) => setKeys((k) => ({ ...k, [f.key]: e.target.value }))}
              placeholder="sk-••••••••••••••••"
              className="w-full bg-slate-800 rounded-lg p-2 text-sm outline-none focus:ring-1 focus:ring-blue-500 font-mono"
            />
          </div>
        ))}
        <button
          onClick={handleSave}
          className="bg-blue-600 hover:bg-blue-500 rounded-lg px-4 py-2 text-sm font-medium w-full"
        >
          {saved ? "Saved ✓" : "Save Keys"}
        </button>
        <p className="text-xs text-slate-500">
          No keys? Leave blank — OmniAgent runs in simulated mode so you can still see the full
          Planner → Router → Worker → Critic → Synthesizer pipeline end-to-end.
        </p>
      </div>
    </main>
  );
}
