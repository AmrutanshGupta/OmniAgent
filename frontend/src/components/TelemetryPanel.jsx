"use client";

export default function TelemetryPanel({ tasks, cost, connected }) {
  const list = Object.values(tasks || {});
  const active = list.find((t) => t.status === "executing");
  const completedCount = list.filter((t) => t.status === "completed").length;

  return (
    <div className="grid grid-cols-2 gap-3 text-sm">
      <div className="bg-slate-800/60 rounded-lg p-3">
        <div className="text-slate-400 text-xs mb-1">Connection</div>
        <div className={connected ? "text-green-400" : "text-red-400"}>
          {connected ? "● live" : "○ reconnecting…"}
        </div>
      </div>
      <div className="bg-slate-800/60 rounded-lg p-3">
        <div className="text-slate-400 text-xs mb-1">Progress</div>
        <div>{completedCount} / {list.length || 0} tasks</div>
      </div>
      <div className="bg-slate-800/60 rounded-lg p-3">
        <div className="text-slate-400 text-xs mb-1">Routing HUD — active tier</div>
        <div className="font-mono text-blue-400">{active ? active.tier : "idle"}</div>
      </div>
      <div className="bg-slate-800/60 rounded-lg p-3">
        <div className="text-slate-400 text-xs mb-1">Cost Tracker (FrugalGPT)</div>
        <div>
          spent: <span className="text-amber-400">${cost.spent?.toFixed(4) || "0.0000"}</span>
        </div>
        <div>
          saved: <span className="text-green-400">${cost.saved?.toFixed(4) || "0.0000"}</span>
        </div>
      </div>
    </div>
  );
}
