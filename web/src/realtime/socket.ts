import type { Message } from "../api/client";

export type ConnectionState = "connecting" | "live" | "polling" | "retrying" | "offline";

interface EventEnvelope {
  event_id: string;
  type: string;
  room_id: string;
  sequence: number;
  payload: Message;
}

interface ConnectOptions {
  roomID: string;
  after: number;
  onMessage: (message: Message) => void;
  onState: (state: ConnectionState) => void;
}

export function connectRoomEvents({ roomID, after, onMessage, onState }: ConnectOptions): () => void {
  let stopped = false;
  let socket: WebSocket | undefined;
  let retryTimer: number | undefined;
  let connectTimer: number | undefined;
  let retryCount = 0;
  let lastSequence = after;

  const clearConnectTimer = () => {
    if (connectTimer !== undefined) window.clearTimeout(connectTimer);
    connectTimer = undefined;
  };

  const scheduleReconnect = () => {
    if (stopped || retryTimer !== undefined) return;
    retryCount += 1;
    // REST reconciliation remains active while the realtime channel recovers.
    onState("polling");
    const delay = Math.min(10_000, 500 * 2 ** Math.min(retryCount, 5));
    retryTimer = window.setTimeout(() => {
      retryTimer = undefined;
      connect();
    }, delay);
  };

  const connect = () => {
    if (stopped) return;
    onState(retryCount === 0 ? "connecting" : "polling");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = new URL(`${protocol}//${window.location.host}/api/v1/events`);
    url.searchParams.set("room_id", roomID);
    url.searchParams.set("after", String(lastSequence));
    const current = new WebSocket(url);
    socket = current;
    connectTimer = window.setTimeout(() => {
      if (!stopped && socket === current && current.readyState !== WebSocket.OPEN) current.close();
    }, 10_000);
    current.onopen = () => {
      if (stopped || socket !== current) return;
      clearConnectTimer();
      retryCount = 0;
      onState("live");
    };
    current.onmessage = (event) => {
      if (stopped || socket !== current) return;
      let envelope: EventEnvelope;
      try {
        envelope = JSON.parse(String(event.data)) as EventEnvelope;
      } catch {
        return;
      }
      if (envelope.type !== "message.created" || envelope.room_id !== roomID) return;
      lastSequence = Math.max(lastSequence, envelope.sequence);
      onMessage(envelope.payload);
    };
    current.onerror = () => {
      if (!stopped && socket === current) current.close();
    };
    current.onclose = () => {
      clearConnectTimer();
      if (socket !== current) return;
      socket = undefined;
      scheduleReconnect();
    };
  };

  const reconnectWhenOnline = () => {
    if (stopped || socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return;
    if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    retryTimer = undefined;
    connect();
  };

  window.addEventListener("online", reconnectWhenOnline);
  connect();
  return () => {
    stopped = true;
    onState("offline");
    window.removeEventListener("online", reconnectWhenOnline);
    if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    clearConnectTimer();
    socket?.close();
  };
}
