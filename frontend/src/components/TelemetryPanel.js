"use client"
import { Activity, DollarSign, Zap } from "lucide-react";

export default function TelemetryPanel({ stats }) {
  return (
    <div className="grid grid-cols-3 gap-4 mb-4">
      <div className="bg-white p-4 rounded-lg border border-gray-200 flex items-center gap-4 shadow-sm">
        <div className="p-3 bg-blue-100 text-blue-600 rounded-full"><Zap size={20}/></div>
        <div>
          <p className="text-xs text-gray-500 uppercase font-semibold">System State</p>
          <p className="text-lg font-bold">{stats.activeTier || 'Idle'}</p>
        </div>
      </div>
      
      <div className="bg-white p-4 rounded-lg border border-gray-200 flex items-center gap-4 shadow-sm">
        <div className="p-3 bg-green-100 text-green-600 rounded-full"><Activity size={20}/></div>
        <div>
          <p className="text-xs text-gray-500 uppercase font-semibold">Total Cost</p>
          <p className="text-lg font-bold">${(stats.currentCost || 0).toFixed(4)}</p>
        </div>
      </div>

      <div className="bg-white p-4 rounded-lg border border-gray-200 flex items-center gap-4 shadow-sm">
        <div className="p-3 bg-orange-100 text-orange-600 rounded-full"><DollarSign size={20}/></div>
        <div>
          <p className="text-xs text-gray-500 uppercase font-semibold">Saved via Cascade</p>
          <p className="text-lg font-bold text-green-600">${(stats.tokensSaved || 0).toFixed(4)}</p>
        </div>
      </div>
    </div>
  );
}