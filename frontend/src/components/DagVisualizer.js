"use client";
import { useMemo } from "react";
import ReactFlow, { Background, Controls, MarkerType, Handle, Position } from "reactflow";
import "reactflow/dist/style.css";
import { Cpu, DollarSign, Fingerprint } from "lucide-react";

// The Beautiful Custom Node with Hover Cards
const CustomTaskNode = ({ data }) => {
  return (
    <div className={`relative group px-4 py-3 shadow-md rounded-xl border-2 bg-white transition-all w-56 ${data.borderColor}`}>
      <Handle type="target" position={Position.Top} className="w-2 h-2 !bg-slate-400 border-none" />
      
      <div className="flex items-center justify-between mb-1">
        <span className={`text-[9px] font-black uppercase tracking-wider px-1.5 py-0.5 rounded-md ${data.badgeColor}`}>
          {data.status}
        </span>
        {data.actualTier && (
          <span className="text-[10px] font-bold text-slate-400 flex items-center gap-1">
            <Cpu className="w-3 h-3" /> {data.actualTier}
          </span>
        )}
      </div>

      <div className="text-xs font-bold text-slate-800 leading-snug line-clamp-3">
        {data.task}
      </div>

      <Handle type="source" position={Position.Bottom} className="w-2 h-2 !bg-slate-400 border-none" />

      {/* The Hover Tooltip (Appears when user hovers over node) */}
      <div className="absolute left-1/2 -translate-x-1/2 bottom-full mb-3 hidden group-hover:block w-48 bg-slate-900 text-white text-xs rounded-lg p-3 shadow-xl z-50 animate-in fade-in zoom-in duration-200">
        <div className="flex flex-col gap-2">
          <div className="flex justify-between items-center border-b border-slate-700 pb-1">
            <span className="text-slate-400 font-semibold">Planned:</span>
            <span className="font-bold text-blue-300">{data.plannedTier}</span>
          </div>
          <div className="flex justify-between items-center border-b border-slate-700 pb-1">
            <span className="text-slate-400 font-semibold">Tokens:</span>
            <span className="font-bold">{data.tokensUsed || 0}</span>
          </div>
          <div className="flex justify-between items-center">
            <span className="text-slate-400 font-semibold">Cost:</span>
            <span className="font-bold text-emerald-400">${(data.costUsd || 0).toFixed(6)}</span>
          </div>
        </div>
        {/* Triangle pointer */}
        <div className="absolute -bottom-1.5 left-1/2 -translate-x-1/2 w-3 h-3 bg-slate-900 rotate-45"></div>
      </div>
    </div>
  );
};

export default function DagVisualizer({ data }) {
  const nodeTypes = useMemo(() => ({ customTask: CustomTaskNode }), []);

  const nodes = Object.entries(data?.nodes || {}).map(([id, node], i) => {
    let borderColor = "border-slate-200";
    let badgeColor = "bg-slate-100 text-slate-500";
    
    if (node.status === "executing") {
      borderColor = "border-blue-500 shadow-blue-500/20";
      badgeColor = "bg-blue-100 text-blue-700";
    }
    if (node.status === "completed") {
      borderColor = "border-emerald-500";
      badgeColor = "bg-emerald-100 text-emerald-700";
    }
    if (node.status === "escalated") {
      borderColor = "border-orange-400";
      badgeColor = "bg-orange-100 text-orange-700";
    }
    if (node.status === "failed") {
      borderColor = "border-rose-500";
      badgeColor = "bg-rose-100 text-rose-700";
    }

    return {
      id: id,
      type: "customTask",
      position: { x: (i % 2 === 0 ? 100 : 350), y: i * 150 + 50 }, // Staggered layout for better visibility
      data: { 
        task: node.task,
        status: node.status,
        plannedTier: node.planned_tier,
        actualTier: node.actual_tier,
        tokensUsed: node.tokens_used,
        costUsd: node.cost_usd,
        borderColor,
        badgeColor
      },
    };
  });

  const edges = (data?.edges || []).map((edge, i) => ({
    id: `e${i}`,
    source: edge[0],
    target: edge[1],
    animated: true,
    style: { stroke: "#94a3b8", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#94a3b8" },
  }));

  return (
    <div className="w-full h-full bg-[#F8FAFC]">
      {nodes.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-full text-slate-400 text-sm gap-2">
          <Fingerprint className="w-8 h-8 opacity-20" />
          Awaiting task graph generation...
        </div>
      ) : (
        <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView className="bg-dots-pattern">
          <Background color="#cbd5e1" gap={24} size={2} />
          <Controls className="!bg-white !border-slate-200 !shadow-sm !rounded-lg" />
        </ReactFlow>
      )}
    </div>
  );
}