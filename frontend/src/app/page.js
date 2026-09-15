"use client";

import { useMemo } from "react";
import { useSession, signIn } from "next-auth/react";
import DashboardLayout from "@/components/DashboardLayout";
import ChatPane from "@/components/ChatPane";
import DagVisualizer from "@/components/DagVisualizer";
import { useOrchestratorSocket } from "@/hooks/useOrchestratorSocket";
import { Cpu, DollarSign, Zap, Play, XOctagon, Loader2 } from "lucide-react";

export default function Dashboard() {
  const { data: session, status } = useSession();

  const {
    dagData, telemetry, isConnected, sendPrompt,
    needsApproval, approvePlan, rejectPlan, systemMessage
  } = useOrchestratorSocket(session?.token);

  const isPipelineActive = Object.keys(dagData?.nodes || {}).length > 0;

  // FIX: Math.random() was being called directly in JSX, meaning it
  // re-rolled a new "JOB_ID" on every single re-render (every telemetry
  // tick), so the id visibly changed mid-run. useMemo locks it once per
  // mount/pipeline-start instead.
  const jobId = useMemo(
    () => Math.random().toString(36).substring(2, 10).toUpperCase(),
    [isPipelineActive]
  );

  if (status === "loading") {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-[#F8FAFC] px-4">
        <div className="flex flex-col items-center gap-6">
          <div className="relative flex items-center justify-center">
            <div className="absolute w-24 h-24 border-4 border-slate-200 border-t-blue-600 rounded-full animate-spin" />
            <div className="w-16 h-16 bg-white border border-slate-200 shadow-xl rounded-full flex items-center justify-center z-10">
              <Cpu className="w-8 h-8 text-blue-600 animate-pulse" />
            </div>
          </div>
          <div className="space-y-2 text-center">
            <h3 className="font-bold text-slate-900 text-lg">Initializing Workspace</h3>
            <div className="w-48 h-1.5 bg-slate-200 rounded-full overflow-hidden">
              <div className="w-1/2 h-full bg-blue-600 rounded-full animate-pulse" />
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen bg-[#F8FAFC] px-4 text-center">
        <div className="w-16 h-16 bg-white border border-slate-200 shadow-xl rounded-2xl flex items-center justify-center mb-6">
          <Cpu className="w-8 h-8 text-slate-900" />
        </div>
        <h1 className="text-2xl sm:text-3xl font-black text-slate-900 tracking-tight mb-2">Access OmniAgent</h1>
        <p className="text-slate-500 font-medium text-sm mb-8">Secure authentication required for cluster deployment.</p>
        <button onClick={() => signIn("github")} className="px-6 py-3 bg-slate-900 hover:bg-slate-800 text-white font-bold rounded-xl shadow-lg transition-all">
          Login with GitHub
        </button>
      </div>
    );
  }

  return (
    <DashboardLayout isConnected={isConnected}>
      {/*
      */}
      <div className={`w-full h-full flex ${isPipelineActive ? "flex-col lg:flex-row" : "items-center justify-center"} transition-all duration-500 overflow-y-auto lg:overflow-hidden`}>

        {isPipelineActive && (
          <div className="flex-1 flex flex-col h-full min-w-0 p-4 sm:p-8 overflow-y-auto">

            <div className="mb-6 flex flex-col sm:flex-row sm:justify-between sm:items-end gap-4">
              <div className="min-w-0">
                <div className="text-[10px] font-bold text-slate-400 tracking-widest uppercase mb-1 truncate">
                  JOB_ID: #{jobId}
                </div>
                <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900">Pipeline Execution</h2>
                <p className="text-slate-500 text-sm font-medium mt-1">Deploying autonomous workflows across routing networks.</p>
              </div>
              <div className="flex gap-2 shrink-0">
                 <button onClick={rejectPlan} className="px-4 py-2 bg-rose-50 border border-rose-200 text-rose-600 rounded-lg text-sm font-bold shadow-sm flex items-center gap-2 hover:bg-rose-100 transition-colors whitespace-nowrap">
                    <XOctagon className="w-4 h-4" /> Terminate
                 </button>
              </div>
            </div>

            {/* FIX: grid-cols-3 was fixed regardless of viewport; now stacks
                to 1 column on mobile, 2 on small tablets, 3 from md up. */}
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-5 mb-6 shrink-0">
              <div className="bg-white p-5 rounded-xl border border-slate-200 shadow-sm min-w-0">
                <div className="flex justify-between items-start mb-4">
                  <div className="w-10 h-10 bg-blue-50 rounded-lg flex items-center justify-center shrink-0"><Cpu className="w-5 h-5 text-blue-600" /></div>
                  <span className="text-[10px] font-bold bg-blue-50 text-blue-700 px-2 py-1 rounded-md flex items-center gap-1.5 shrink-0"><span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-pulse"/> Running</span>
                </div>
                <p className="text-xs font-bold text-slate-400 uppercase">System State</p>
                {/* FIX: truncate long tier names instead of overflowing the card */}
                <p className="text-2xl sm:text-3xl font-black text-slate-900 mt-1 truncate" title={telemetry.activeTier || "Evaluating"}>
                  {telemetry.activeTier || "Evaluating"}
                </p>
              </div>

              <div className="bg-white p-5 rounded-xl border border-slate-200 shadow-sm min-w-0">
                <div className="w-10 h-10 bg-slate-50 rounded-lg flex items-center justify-center mb-4"><DollarSign className="w-5 h-5 text-slate-600" /></div>
                <p className="text-xs font-bold text-slate-400 uppercase">Total Cost (EST)</p>
                <p className="text-2xl sm:text-3xl font-black text-slate-900 mt-1 truncate">${(telemetry.currentCost || 0).toFixed(4)}</p>
              </div>

              <div className="bg-white p-5 rounded-xl border border-slate-200 shadow-sm min-w-0">
                <div className="w-10 h-10 bg-orange-50 rounded-lg flex items-center justify-center mb-4"><Zap className="w-5 h-5 text-orange-500" /></div>
                <p className="text-xs font-bold text-slate-400 uppercase">Saved via Cascade</p>
                <p className="text-2xl sm:text-3xl font-black text-slate-900 mt-1 truncate">
                  {telemetry.tokensSaved || 0} <span className="text-sm text-slate-400">Tokens</span>
                </p>
              </div>
            </div>

            <div className="flex-1 bg-white rounded-xl border border-slate-200 shadow-sm flex flex-col relative min-h-[400px]">
              <div className="p-4 border-b border-slate-100 bg-slate-50">
                <span className="font-bold text-sm text-slate-900">Execution Topology</span>
              </div>

              <div className="flex-1 relative bg-slate-50/50 overflow-auto">
                <DagVisualizer data={dagData} />

                {needsApproval && (
                  <div className="absolute inset-x-2 sm:inset-x-4 bottom-4 bg-white/95 backdrop-blur-md p-4 sm:p-5 rounded-xl border border-blue-200 shadow-xl flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 z-50">
                    <div className="flex items-center gap-4 min-w-0">
                      <div className="w-10 h-10 bg-blue-50 text-blue-600 rounded-full flex items-center justify-center shrink-0">
                        <Loader2 className="w-5 h-5 animate-spin" />
                      </div>
                      <div className="min-w-0">
                        <h4 className="font-bold text-slate-900">Action Required: Review Topology</h4>
                        <p className="text-sm font-medium text-slate-500">Please authorize the generated sequence.</p>
                      </div>
                    </div>
                    <div className="flex gap-2 shrink-0 w-full sm:w-auto">
                      <button onClick={rejectPlan} className="flex-1 sm:flex-none px-4 py-2 bg-white border border-slate-200 text-slate-700 rounded-lg text-sm font-bold shadow-sm">Abort</button>
                      <button onClick={approvePlan} className="flex-1 sm:flex-none px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-bold shadow-md flex items-center justify-center gap-2">
                        <Play className="w-4 h-4" /> Authorize Execution
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        <ChatPane onSend={sendPrompt} isPipelineActive={isPipelineActive} systemMessage={systemMessage} />
      </div>
    </DashboardLayout>
  );
}