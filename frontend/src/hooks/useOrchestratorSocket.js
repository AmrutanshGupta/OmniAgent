"use client";

import { useState, useEffect, useCallback } from 'react';

export function useOrchestratorSocket(token, baseUrl = process.env.NEXT_PUBLIC_WS_URL || 'ws://127.0.0.1:8080/ws') {
  const [socket, setSocket] = useState(null);
  const [dagData, setDagData] = useState({ nodes: {}, edges: [] });
  const [telemetry, setTelemetry] = useState({ activeTier: 'None', tokensSaved: 0, currentCost: 0 });
  const [isConnected, setIsConnected] = useState(false);
  const [systemMessage, setSystemMessage] = useState(null);
  
  // HITL State
  const [needsApproval, setNeedsApproval] = useState(false);

  useEffect(() => {
    if (!token) return;

    let isStale = false;
    const wsUrl = `${baseUrl}?token=${token}`;
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => { if (!isStale) setIsConnected(true); };
    ws.onclose = () => { if (!isStale) setIsConnected(false); };

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      
      if (data.type === 'DAG_UPDATE') setDagData(data.payload);
      if (data.type === 'TELEMETRY_UPDATE') setTelemetry(data.payload);
      
      if (data.type === 'DAG_PENDING_APPROVAL') {
        setDagData(data.payload);
        setNeedsApproval(true);
      }
      
      if (data.type === 'SYSTEM_MESSAGE') {
        setSystemMessage({ type: 'info', text: data.payload });
      }

      if (data.type === 'ERROR') {
        console.error("Orchestrator Error:", data.payload);
        setSystemMessage({ type: 'error', text: data.payload });
        setNeedsApproval(false);
      }
      if (data.type === 'FINAL_RESULT') {
        setSystemMessage({ type: 'success', text: data.payload });
      }
    };

    setSocket(ws);
    return () => {
      isStale = true;
      ws.close();
    };
  }, [baseUrl, token]);

  const sendPrompt = useCallback((promptText) => {
    if (socket && socket.readyState === WebSocket.OPEN) {
      setSystemMessage(null); 
      setDagData({ nodes: {}, edges: [] });
      setNeedsApproval(false);
      
      socket.send(JSON.stringify({
        type: 'NEW_TASK',
        payload: promptText,
        weights: { w_q: 0.5, w_c: 0.5, w_l: 0.2 }
      }));
    }
  }, [socket]);

  const approvePlan = useCallback(() => {
    if (socket) {
      socket.send(JSON.stringify({ type: 'APPROVE_DAG' }));
      setNeedsApproval(false);
    }
  }, [socket]);

  const rejectPlan = useCallback(() => {
    if (socket) {
      socket.send(JSON.stringify({ type: 'REJECT_DAG' }));
      setNeedsApproval(false);
      setDagData({ nodes: {}, edges: [] });
    }
  }, [socket]);

  return { socket, dagData, telemetry, isConnected, sendPrompt, systemMessage, needsApproval, approvePlan, rejectPlan };
}