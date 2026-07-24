"use client";
import { useEffect, useRef, useState, useCallback } from "react";

const GO_HTTP = process.env.NEXT_PUBLIC_GO_API_URL || "http://localhost:8080";
const GO_WS = GO_HTTP.replace(/^http/, "ws") + "/ws";

export function useOrchestratorSocket() {
  const [tasks, setTasks] = useState({}); // id -> task
  const [plan, setPlan] = useState(null);
  const [cost, setCost] = useState({ spent: 0, saved: 0 });
  const [connected, setConnected] = useState(false);
  const wsRef = useRef(null);

  useEffect(() => {
    let retryTimer;
    function connect() {
      const ws = new WebSocket(GO_WS);
      wsRef.current = ws;
      ws.onopen = () => setConnected(true);
      ws.onclose = () => {
        setConnected(false);
        retryTimer = setTimeout(connect, 2000);
      };
      ws.onerror = () => ws.close();
      ws.onmessage = (evt) => {
        try {
          const msg = JSON.parse(evt.data);
          if (msg.type === "plan_ready") {
            setPlan(msg.data);
            const initial = {};
            (msg.data.tasks || []).forEach((t) => (initial[t.id] = t));
            setTasks(initial);
          } else if (msg.type === "task_update") {
            setTasks((prev) => ({ ...prev, [msg.data.id]: msg.data }));
          } else if (msg.type === "run_complete") {
            setCost({ spent: msg.data.costSpent, saved: msg.data.costSaved });
          }
        } catch (e) {
          // ignore malformed frames
        }
      };
    }
    connect();
    return () => {
      clearTimeout(retryTimer);
      wsRef.current?.close();
    };
  }, []);

  const reset = useCallback(() => {
    setTasks({});
    setPlan(null);
    setCost({ spent: 0, saved: 0 });
  }, []);

  return { tasks, plan, cost, connected, reset };
}

export async function submitPrompt({ prompt, sla, keys }) {
  const res = await fetch(`${GO_HTTP}/api/plan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt, sla, keys }),
  });
  if (!res.ok) throw new Error(`Orchestrator error: ${res.status}`);
  return res.json();
}

export { GO_HTTP };
