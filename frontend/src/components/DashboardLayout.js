"use client";

import { useState } from "react";
import { useSession, signOut } from "next-auth/react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Cpu, Vault, LogOut, ChevronRight, Workflow, Menu, X, Settings } from "lucide-react";

export default function DashboardLayout({ children, isConnected }) {
  const { data: session } = useSession();
  const pathname = usePathname();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const sidebarContent = (
    <>
      <div>
        <div className="px-6 mb-8 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center shadow-md shadow-blue-600/20 shrink-0">
              <Workflow className="w-4 h-4 text-white stroke-[2.5]" />
            </div>
            <span className="font-extrabold text-xl tracking-tight truncate">OmniAgent</span>
          </div>
          <button onClick={() => setMobileNavOpen(false)} className="md:hidden text-slate-400 hover:text-slate-700 p-1 shrink-0">
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav className="px-3 space-y-1.5">
          <Link href="/" onClick={() => setMobileNavOpen(false)}
            className={`flex items-center justify-between px-3 py-2.5 rounded-lg font-semibold text-sm transition-all duration-200 ${
              pathname === "/" ? "bg-[#F1F5F9] text-slate-900" : "text-slate-500 hover:bg-slate-50 hover:text-slate-900"
            }`}>
            <div className="flex items-center gap-3 min-w-0">
              <Cpu className="w-4 h-4 stroke-[2.5] shrink-0" /> <span className="truncate">Orchestrator</span>
            </div>
            {pathname === "/" && <ChevronRight className="w-4 h-4 text-slate-400 shrink-0" />}
          </Link>

          <Link href="/settings" onClick={() => setMobileNavOpen(false)}
            className={`flex items-center justify-between px-3 py-2.5 rounded-lg font-semibold text-sm transition-all duration-200 ${
              pathname === "/settings" ? "bg-[#F1F5F9] text-slate-900" : "text-slate-500 hover:bg-slate-50 hover:text-slate-900"
            }`}>
            <div className="flex items-center gap-3 min-w-0">
              <Vault className="w-4 h-4 stroke-[2.5] shrink-0" /> <span className="truncate">Vault</span>
            </div>
            {pathname === "/settings" && <ChevronRight className="w-4 h-4 text-slate-400 shrink-0" />}
          </Link>
        </nav>
      </div>

      <div className="px-4 space-y-4">
        <div className="p-4 bg-slate-50 border border-slate-100 rounded-xl">
          <div className="text-[10px] font-bold text-slate-400 tracking-wider uppercase mb-2">System Status</div>
          <div className="flex items-center gap-2 text-xs font-bold text-slate-700">
            <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${isConnected ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)] animate-pulse" : "bg-slate-300"}`} />
            {isConnected ? "Active Engine" : "Standby"}
          </div>
        </div>

        <div className="flex items-center justify-between px-2 pt-2 border-t border-slate-100">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-8 h-8 bg-slate-900 text-white rounded-full flex items-center justify-center text-xs font-bold shadow-inner shrink-0">
              {session?.user?.email ? session.user.email[0].toUpperCase() : "U"}
            </div>
            {session?.user?.email && <span className="text-xs font-semibold text-slate-500 truncate">{session.user.email}</span>}
          </div>
          <button onClick={() => signOut()} className="text-slate-400 hover:text-rose-500 p-1.5 rounded-lg transition-colors bg-slate-50 hover:bg-rose-50 border border-slate-200 hover:border-rose-200 shrink-0">
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </div>
    </>
  );

  return (
    <div className="flex h-screen bg-[#F8FAFC] overflow-hidden antialiased text-slate-900">
      <aside className="hidden md:flex w-[260px] h-full bg-white border-r border-slate-200 flex-col justify-between pt-6 pb-4 shrink-0 shadow-sm z-20 relative">
        {sidebarContent}
      </aside>

      {mobileNavOpen && (
        <div className="md:hidden fixed inset-0 z-40 flex">
          <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm" onClick={() => setMobileNavOpen(false)} />
          <aside className="relative w-[260px] max-w-[80vw] h-full bg-white border-r border-slate-200 flex flex-col justify-between pt-6 pb-4 shadow-xl z-50">
            {sidebarContent}
          </aside>
        </div>
      )}

      <div className="flex-1 flex flex-col min-w-0 h-full relative">
        <header className="h-16 border-b border-slate-200 bg-white/80 backdrop-blur-md flex items-center justify-between px-4 sm:px-8 shrink-0 z-10 relative">
          <div className="flex items-center gap-3 sm:gap-6 min-w-0">
            <button onClick={() => setMobileNavOpen(true)} className="md:hidden text-slate-500 hover:text-slate-900 p-1.5 shrink-0">
              <Menu className="w-5 h-5" />
            </button>
            <span className="text-sm font-bold text-slate-900 border-b-[3px] border-slate-900 h-16 flex items-center shrink-0">
              {pathname === "/settings" ? "Settings Vault" : "Dashboard"}
            </span>
          </div>
          <div className="flex items-center gap-2 sm:gap-4 shrink-0">
            <div className="hidden sm:flex px-3 py-1 bg-slate-100 text-slate-600 rounded-full text-xs font-bold items-center gap-2">
              <span className="w-1.5 h-1.5 rounded-full bg-blue-600" /> OmniAgent Core
            </div>
            <Link href="/settings" className="text-slate-400 hover:text-slate-600 p-1.5 transition-colors">
              <Settings className="w-4 h-4 stroke-[2.5]" />
            </Link>
          </div>
        </header>

        {/* Added fade-in animation for smooth routing */}
        <main className="flex-1 overflow-hidden relative bg-[#F8FAFC] animate-in fade-in duration-300">{children}</main>
      </div>
    </div>
  );
}