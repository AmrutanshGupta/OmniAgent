"use client";
import { useState } from "react";
import Link from "next/link";
import ChatPane from "../components/ChatPane";
import DagVisualizer from "../components/DagVisualizer";
import TelemetryPanel from "../components/TelemetryPanel";
import { useOrchestratorSocket, submitPrompt } from "../hooks/useOrchestratorSocket";

export default function Workspace() {
  const { tasks, cost, connected, reset } = useOrchestratorSocket();
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [sla, setSla] = useState(0.5); // 0 = save money, 1 = max quality

  const handleSubmit = async (prompt) => {
    setLoading(true);
    reset();
    let keys = {};
    try {
      keys = JSON.parse(localStorage.getItem("omniagent_keys") || "{}");
    } catch {}
    try {
      const result = await submitPrompt({
        prompt,
        sla: { qualityWeight: sla, costWeight: 1 - sla },
        keys,
      });
      setHistory((h) => [...h, { prompt, final: result.final }]);
    } catch (e) {
      setHistory((h) => [...h, { prompt, final: `Error: ${e.message}` }]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="h-screen flex flex-col bg-[#0b0e14]">
      <header className="flex items-center justify-between px-6 py-3 border-b border-slate-800">
        <h1 className="text-lg font-semibold">
          OmniAgent <span className="text-slate-500 font-normal text-sm">Infrastructure-Aware LLM Orchestrator</span>
        </h1>
        <Link href="/settings" className="text-sm text-blue-400 hover:underline">
          Settings / API Vault
        </Link>
      </header>

      <div className="flex-1 grid grid-cols-2 gap-4 p-4 min-h-0">
        {/* Left pane: Chat */}
        <section className="bg-slate-900/60 rounded-xl p-4 flex flex-col min-h-0">
          <div className="mb-3 flex items-center gap-3">
            <span className="text-xs text-slate-400">Save Money</span>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={sla}
              onChange={(e) => setSla(Number(e.target.value))}
              className="flex-1"
            />
            <span className="text-xs text-slate-400">Max Quality</span>
          </div>
          <ChatPane onSubmit={handleSubmit} loading={loading} history={history} />
        </section>

        {/* Right pane: Telemetry + DAG */}
        <section className="bg-slate-900/60 rounded-xl p-4 flex flex-col min-h-0 gap-4">
          <TelemetryPanel tasks={tasks} cost={cost} connected={connected} />
          <div className="flex-1 bg-slate-950/60 rounded-lg overflow-auto">
            <DagVisualizer tasks={tasks} />
          </div>
        </section>
      </div>
    </main>
  );
}
