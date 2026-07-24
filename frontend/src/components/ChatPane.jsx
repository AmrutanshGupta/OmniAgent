"use client";
import { useState } from "react";

export default function ChatPane({ onSubmit, loading, history }) {
  const [prompt, setPrompt] = useState("");

  const handleSend = () => {
    if (!prompt.trim() || loading) return;
    onSubmit(prompt.trim());
    setPrompt("");
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto space-y-4 pr-2">
        {history.length === 0 && (
          <div className="text-slate-500 text-sm">
            Try: "Build a REST API endpoint that analyzes CSV upload data and writes a short marketing summary of the results."
          </div>
        )}
        {history.map((h, i) => (
          <div key={i} className="space-y-2">
            <div className="bg-blue-600/20 border border-blue-600/40 rounded-lg p-3 text-sm">
              {h.prompt}
            </div>
            {h.final && (
              <pre className="bg-slate-800/60 rounded-lg p-3 text-xs whitespace-pre-wrap text-slate-200">
                {h.final}
              </pre>
            )}
          </div>
        ))}
        {loading && <div className="text-slate-400 text-sm animate-pulse">Planning &amp; executing…</div>}
      </div>
      <div className="mt-3 flex gap-2">
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              handleSend();
            }
          }}
          placeholder="Describe what you want built…"
          className="flex-1 bg-slate-800 rounded-lg p-3 text-sm resize-none h-16 outline-none focus:ring-1 focus:ring-blue-500"
        />
        <button
          onClick={handleSend}
          disabled={loading}
          className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded-lg px-4 text-sm font-medium"
        >
          Send
        </button>
      </div>
    </div>
  );
}
