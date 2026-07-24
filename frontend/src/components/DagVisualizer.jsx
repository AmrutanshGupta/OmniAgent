"use client";

const STATUS_COLOR = {
  pending: "#4b5563",   // grey
  executing: "#3b82f6", // blue
  completed: "#22c55e", // green
  escalated: "#ef4444", // red
  failed: "#f97316",    // orange
};

// Simple layered layout: tasks with no unmet deps go left-to-right by "wave".
function layoutTasks(tasks) {
  const byId = Object.fromEntries(tasks.map((t) => [t.id, t]));
  const waveOf = {};
  function waveFor(id, seen = new Set()) {
    if (waveOf[id] !== undefined) return waveOf[id];
    if (seen.has(id)) return 0;
    seen.add(id);
    const t = byId[id];
    if (!t || !t.dependsOn || t.dependsOn.length === 0) {
      waveOf[id] = 0;
      return 0;
    }
    const w = 1 + Math.max(...t.dependsOn.map((d) => waveFor(d, seen)));
    waveOf[id] = w;
    return w;
  }
  tasks.forEach((t) => waveFor(t.id));
  const columns = {};
  tasks.forEach((t) => {
    const w = waveOf[t.id];
    columns[w] = columns[w] || [];
    columns[w].push(t);
  });
  const positions = {};
  const colWidth = 220;
  const rowHeight = 90;
  Object.entries(columns).forEach(([w, list]) => {
    list.forEach((t, i) => {
      positions[t.id] = {
        x: 60 + Number(w) * colWidth,
        y: 50 + i * rowHeight,
      };
    });
  });
  const maxCol = Math.max(0, ...Object.keys(columns).map(Number));
  const maxRow = Math.max(1, ...Object.values(columns).map((l) => l.length));
  return { positions, width: 60 + (maxCol + 1) * colWidth + 60, height: 50 + maxRow * rowHeight + 20 };
}

export default function DagVisualizer({ tasks }) {
  const list = Object.values(tasks || {});
  if (list.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-slate-500 text-sm">
        Submit a prompt to see the live DAG here.
      </div>
    );
  }
  const { positions, width, height } = layoutTasks(list);

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} className="min-h-[260px]">
      {/* edges */}
      {list.map((t) =>
        (t.dependsOn || []).map((dep) => {
          const from = positions[dep];
          const to = positions[t.id];
          if (!from || !to) return null;
          return (
            <line
              key={`${dep}-${t.id}`}
              x1={from.x + 80}
              y1={from.y + 20}
              x2={to.x}
              y2={to.y + 20}
              stroke="#334155"
              strokeWidth={2}
            />
          );
        })
      )}
      {/* nodes */}
      {list.map((t) => {
        const p = positions[t.id];
        if (!p) return null;
        const color = STATUS_COLOR[t.status] || "#4b5563";
        return (
          <g key={t.id} transform={`translate(${p.x},${p.y})`}>
            <rect width="160" height="56" rx="10" fill="#111827" stroke={color} strokeWidth="2" />
            <circle cx="14" cy="14" r="5" fill={color} />
            <text x="26" y="18" fontSize="10" fill="#e5e7eb" fontWeight="600">
              {t.domain}
            </text>
            <text x="10" y="34" fontSize="9" fill="#9ca3af">
              {(t.description || "").slice(0, 26)}
              {(t.description || "").length > 26 ? "…" : ""}
            </text>
            <text x="10" y="48" fontSize="9" fill={color}>
              {t.status} {t.tier ? `· ${t.tier}` : ""}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
