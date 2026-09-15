"use client";

import { useState, useEffect } from "react";
import { useSession } from "next-auth/react";
import DashboardLayout from "@/components/DashboardLayout";
import Link from "next/link";
import { Eye, EyeOff, ShieldCheck, Key, ArrowLeft, Loader2 } from "lucide-react";

const PROVIDERS = [
  { key: "openai", label: "OpenAI API Key", badge: "Premium", hint: "Flagship GPT-4 models.", placeholder: "sk-proj-..." },
  { key: "anthropic", label: "Anthropic API Key", badge: "Premium", hint: "Claude Sonnet/Opus models.", placeholder: "sk-ant-..." },
  { key: "google", label: "Google Gemini Key", badge: "Standard", hint: "Fast Flash models.", placeholder: "AIzaSy..." },
  { key: "groq", label: "Groq API Key", badge: "Free Tier", hint: "Ultra-fast open weights via Groq LPU.", placeholder: "gsk_..." },
  { key: "huggingface", label: "Hugging Face Token", badge: "Free Tier", hint: "Serverless community inference.", placeholder: "hf_..." },
];

export default function SettingsPage() {
  const { data: session, status } = useSession();

  const [apiKeys, setApiKeys] = useState({ openai: "", anthropic: "", google: "", groq: "", huggingface: "" });
  const [showKey, setShowKey] = useState({ openai: false, anthropic: false, google: false, groq: false, huggingface: false });
  const [isLoading, setIsLoading] = useState(true);
  const [savingProvider, setSavingProvider] = useState(null);
  const [messages, setMessages] = useState({});

  useEffect(() => {
    if (status === "authenticated") {
      fetch("/api/user/keys")
        .then((res) => res.json())
        .then((data) => {
          if (data.keys) {
            setApiKeys({
              openai: data.keys.openai || "", anthropic: data.keys.anthropic || "",
              google: data.keys.google || "", groq: data.keys.groq || "", huggingface: data.keys.huggingface || "",
            });
          }
        })
        .finally(() => setIsLoading(false));
    }
  }, [status]);

  const handleSaveProvider = async (providerKey) => {
    setSavingProvider(providerKey);
    setMessages((prev) => ({ ...prev, [providerKey]: null }));
    try {
      const res = await fetch("/api/user/keys", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ keys: apiKeys }),
      });
      if (!res.ok) throw new Error("Save failed");
      setMessages((prev) => ({ ...prev, [providerKey]: { type: "success", text: "Encrypted & Saved" } }));
      setTimeout(() => setMessages((prev) => ({ ...prev, [providerKey]: null })), 3000);
    } catch {
      setMessages((prev) => ({ ...prev, [providerKey]: { type: "error", text: "Failed to save" } }));
    } finally {
      setSavingProvider(null);
    }
  };

  const toggleShow = (provider) => setShowKey((prev) => ({ ...prev, [provider]: !prev[provider] }));

  if (status === "loading" || isLoading) {
    return (
      <DashboardLayout>
        <div className="flex h-full w-full items-center justify-center">
          <Loader2 className="w-8 h-8 text-blue-600 animate-spin" />
        </div>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout isConnected={true}>
      <div className="w-full h-full overflow-y-auto px-4 sm:px-8 py-8">
        <div className="max-w-3xl mx-auto">
          
          <div className="mb-6">
            <Link href="/" className="inline-flex items-center gap-2 text-xs font-bold text-slate-500 hover:text-slate-900 transition-colors">
              <ArrowLeft className="w-4 h-4" /> Back to Dashboard
            </Link>
          </div>

          <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden mb-8">
            <div className="p-6 border-b border-slate-100 flex items-center justify-between bg-slate-50/50">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 bg-slate-900 text-white rounded-xl flex items-center justify-center"><Key className="w-5 h-5" /></div>
                <div>
                  <h3 className="font-extrabold text-lg text-slate-900 tracking-tight">API Key Vault</h3>
                  <p className="text-xs text-slate-500 font-medium">Manage credentials securely. Keys are encrypted at rest with AES-GCM.</p>
                </div>
              </div>
              <div className="hidden sm:flex px-3 py-1 bg-emerald-50 border border-emerald-100 rounded-full text-[10px] font-bold text-emerald-700 items-center gap-1.5 shadow-sm">
                <ShieldCheck className="w-3 h-3" /> SECURE
              </div>
            </div>

            <div className="p-6 space-y-4">
              {PROVIDERS.map(({ key, label, badge, hint, placeholder }) => (
                <div key={key} className="p-5 bg-white border border-slate-200 rounded-xl flex flex-col gap-3 shadow-sm hover:border-blue-200 transition-colors">
                  <div className="flex items-center justify-between">
                    <label className="text-sm font-bold text-slate-900">{label}</label>
                    <span className={`text-[10px] font-extrabold px-2 py-0.5 rounded-md uppercase ${badge.includes('Free') ? 'bg-emerald-100 text-emerald-700' : 'bg-blue-100 text-blue-700'}`}>
                      {badge}
                    </span>
                  </div>
                  <p className="text-xs text-slate-500 -mt-2">{hint}</p>

                  <div className="flex items-center gap-2 sm:gap-3">
                    <div className="relative flex-1">
                      <input
                        type={showKey[key] ? "text" : "password"}
                        value={apiKeys[key]}
                        onChange={(e) => setApiKeys({ ...apiKeys, [key]: e.target.value })}
                        placeholder={placeholder}
                        className="w-full bg-slate-50 pl-4 pr-10 py-2.5 border border-slate-200 rounded-lg text-sm font-semibold focus:bg-white focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10 transition-all outline-none"
                      />
                      <button type="button" onClick={() => toggleShow(key)} className="absolute right-3 top-3 text-slate-400 hover:text-slate-600">
                        {showKey[key] ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                      </button>
                    </div>

                    <button onClick={() => handleSaveProvider(key)} disabled={savingProvider === key} className="px-5 py-2.5 bg-slate-900 text-white hover:bg-slate-800 disabled:opacity-50 text-xs font-bold rounded-lg transition-all w-[90px] flex justify-center">
                      {savingProvider === key ? <Loader2 className="w-4 h-4 animate-spin" /> : "Save"}
                    </button>
                  </div>
                  {messages[key] && (
                    <p className={`text-xs font-bold ${messages[key].type === "success" ? "text-emerald-600" : "text-rose-600"} animate-in fade-in`}>{messages[key].text}</p>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}