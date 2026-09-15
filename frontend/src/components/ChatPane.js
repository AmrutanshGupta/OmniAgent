"use client";

import { useState, useRef, useEffect } from "react";
import { Send, Terminal, Search, Code, FileText, Paperclip, Bot, Sparkles, Loader2, AlertCircle } from "lucide-react";

export default function ChatPane({ onSend, isPipelineActive, systemMessage }) {
  const [input, setInput] = useState("");
  const [isWaiting, setIsWaiting] = useState(false);
  const scrollRef = useRef(null);

  const [logs, setLogs] = useState([
    { id: 1, type: "SYS", msg: "OmniAgent Engine Core operational. Awaiting task graph." }
  ]);

  // Handle incoming system messages (Errors or Confirmations)
  useEffect(() => {
    if (systemMessage) {
      setIsWaiting(false); // Stop loading spinner if we get an error or response
      setLogs((prev) => [
        ...prev,
        { id: Date.now(), type: systemMessage.type === "error" ? "ERR" : "NODE", msg: systemMessage.text },
      ]);
    }
  }, [systemMessage]);

  // Automatically turn off the loading state when the pipeline takes over the screen
  useEffect(() => {
    if (isPipelineActive) setIsWaiting(false);
  }, [isPipelineActive]);

  useEffect(() => {
    scrollRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  const sendMessage = (text) => {
    const trimmed = text.trim();
    if (!trimmed) return;
    
    setIsWaiting(true); // Trigger the UI loading state
    onSend(trimmed);    // Fire the websocket command
    
    setLogs((prev) => [...prev, { id: Date.now(), type: "USER", msg: trimmed }]);
    setInput("");
  };

  const handleSubmit = (e) => {
    e.preventDefault();
    sendMessage(input);
  };

  const handleSuggestionClick = (text) => {
    sendMessage(text);
  };

  // --- EMPTY STATE (Landing Page) ---
  if (!isPipelineActive) {
    return (
      <div className="w-full max-w-2xl mx-auto flex flex-col items-center justify-center px-4 h-full relative z-10 animate-in fade-in duration-700">
        <div className="w-16 h-16 bg-white border border-slate-200 shadow-xl shadow-slate-200/50 rounded-2xl flex items-center justify-center mb-6 relative">
          <Sparkles className={`w-8 h-8 text-blue-600 stroke-[2] ${isWaiting ? 'animate-ping opacity-50' : ''}`} />
        </div>

        <h2 className="text-3xl sm:text-4xl font-extrabold text-slate-900 tracking-tight text-center mb-3">
          OmniAgent
        </h2>
        <p className="text-xs font-bold text-slate-400 uppercase tracking-widest text-center mb-10 flex items-center gap-2 flex-wrap justify-center">
          Reasoning with multi-provider cascade <span className="w-2 h-4 bg-blue-600 animate-pulse" />
        </p>

        {/* Surface Backend Errors on the Landing Page */}
        {systemMessage && systemMessage.type === "error" && (
          <div className="w-full mb-6 p-4 bg-rose-50 text-rose-700 text-sm font-semibold rounded-xl border border-rose-200 flex items-center gap-3 animate-in slide-in-from-bottom-2">
            <AlertCircle className="w-5 h-5 shrink-0" />
            {systemMessage.text}
          </div>
        )}

        <form
          onSubmit={handleSubmit}
          className="w-full bg-white border border-slate-200 shadow-lg shadow-slate-100 rounded-2xl p-2 flex items-center gap-2 transition-all focus-within:ring-4 focus-within:ring-blue-500/10 focus-within:border-blue-500 mb-8"
        >
          <button
            type="button"
            className="p-2 text-slate-400 hover:text-slate-600 transition-colors rounded-xl hover:bg-slate-50 shrink-0"
          >
            <Paperclip className="w-5 h-5" />
          </button>
          
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            disabled={isWaiting}
            placeholder={isWaiting ? "Generating execution graph..." : "Message OmniAgent..."}
            className="flex-1 min-w-0 bg-transparent border-none outline-none text-base text-slate-800 placeholder:text-slate-400 py-2 disabled:opacity-50"
          />
          
          <button
            type="submit"
            disabled={isWaiting}
            className="bg-blue-600 hover:bg-blue-700 text-white w-10 h-10 rounded-xl flex items-center justify-center shadow-md transition-all active:scale-95 shrink-0 disabled:opacity-70 disabled:hover:bg-blue-600"
          >
            {isWaiting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4 ml-0.5" />}
          </button>
        </form>

        <div className="flex flex-wrap items-center justify-center gap-3">
          <button onClick={() => handleSuggestionClick("Analyze logs")} disabled={isWaiting} className="px-4 py-2 bg-white border border-slate-200 text-slate-600 font-semibold text-xs rounded-full hover:bg-slate-50 transition-all shadow-sm flex items-center gap-2 disabled:opacity-50">
            <Search className="w-3.5 h-3.5" /> Analyze logs
          </button>
          <button onClick={() => handleSuggestionClick("Generate script")} disabled={isWaiting} className="px-4 py-2 bg-white border border-slate-200 text-slate-600 font-semibold text-xs rounded-full hover:bg-slate-50 transition-all shadow-sm flex items-center gap-2 disabled:opacity-50">
            <Code className="w-3.5 h-3.5" /> Generate script
          </button>
          <button onClick={() => handleSuggestionClick("Research topic")} disabled={isWaiting} className="px-4 py-2 bg-white border border-slate-200 text-slate-600 font-semibold text-xs rounded-full hover:bg-slate-50 transition-all shadow-sm flex items-center gap-2 disabled:opacity-50">
            <FileText className="w-3.5 h-3.5" /> Research topic
          </button>
        </div>
      </div>
    );
  }

  // --- ACTIVE PIPELINE STATE (Side Panel) ---
  return (
    <div className="flex flex-col h-full bg-white border-l border-slate-200 shadow-sm w-full md:w-[400px] md:shrink-0 min-h-0">
      <div className="p-4 border-b border-slate-100 flex items-center gap-2 bg-slate-50/50 shrink-0">
        <Terminal className="w-4 h-4 text-slate-500" />
        <span className="font-bold text-sm text-slate-900 tracking-tight">Command Stream</span>
      </div>

      <div className="flex-1 p-4 overflow-y-auto space-y-4 bg-slate-50/30 custom-scrollbar min-h-0">
        {logs.map((log) => (
          <div key={log.id} className={`flex gap-3 ${log.type === "USER" ? "flex-row-reverse" : "flex-row"} animate-in slide-in-from-bottom-2`}>
            {log.type !== "USER" && (
              <div className="w-7 h-7 rounded-lg bg-blue-100 flex items-center justify-center shrink-0 border border-blue-200">
                <Bot className="w-4 h-4 text-blue-600" />
              </div>
            )}

            <div className={`p-3 rounded-2xl max-w-[85%] text-sm shadow-sm break-words ${
                log.type === "USER" ? "bg-slate-900 text-white rounded-tr-sm"
              : log.type === "ERR"  ? "bg-rose-50 border border-rose-200 text-rose-700 rounded-tl-sm"
              : "bg-white border border-slate-200 text-slate-700 rounded-tl-sm"
            }`}>
              <p className="leading-relaxed whitespace-pre-wrap break-all">{log.msg}</p>
            </div>
          </div>
        ))}
        {isWaiting && (
          <div className="flex gap-3 flex-row animate-in slide-in-from-bottom-2">
            <div className="w-7 h-7 rounded-lg bg-slate-100 flex items-center justify-center shrink-0 border border-slate-200">
               <Loader2 className="w-4 h-4 text-slate-400 animate-spin" />
            </div>
          </div>
        )}
        <div ref={scrollRef} />
      </div>

      <div className="p-4 bg-white border-t border-slate-100 shrink-0">
        <form onSubmit={handleSubmit} className="bg-slate-50 border border-slate-200 rounded-xl p-1.5 flex items-center gap-2 focus-within:ring-2 focus-within:border-blue-500 transition-all">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            disabled={isWaiting}
            placeholder="Send a follow-up..."
            className="flex-1 min-w-0 bg-transparent border-none outline-none text-sm text-slate-800 px-3 py-2 disabled:opacity-50"
          />
          <button type="submit" disabled={isWaiting} className="bg-blue-600 text-white p-2.5 rounded-lg hover:bg-blue-700 transition-all shadow-sm shrink-0 disabled:opacity-70">
            <Send className="w-4 h-4" />
          </button>
        </form>
      </div>
    </div>
  );
}