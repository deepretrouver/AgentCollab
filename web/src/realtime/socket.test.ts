import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { connectRoomEvents, type ConnectionState } from "./socket";

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readonly url: string;
  readyState = FakeWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(url: string | URL) {
    this.url = String(url);
    FakeWebSocket.instances.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  close() {
    if (this.readyState === FakeWebSocket.CLOSED) return;
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeWebSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeWebSocket);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test("断线时保留轮询同步并重新建立实时连接", () => {
  const states: ConnectionState[] = [];
  const stop = connectRoomEvents({ roomID: "room-1", after: 3, onMessage: vi.fn(), onState: (state) => states.push(state) });
  expect(states).toEqual(["connecting"]);
  expect(FakeWebSocket.instances).toHaveLength(1);
  expect(FakeWebSocket.instances[0].url).toContain("room_id=room-1");
  expect(FakeWebSocket.instances[0].url).toContain("after=3");

  FakeWebSocket.instances[0].open();
  expect(states.at(-1)).toBe("live");
  FakeWebSocket.instances[0].close();
  expect(states.at(-1)).toBe("polling");

  vi.advanceTimersByTime(1_000);
  expect(FakeWebSocket.instances).toHaveLength(2);
  FakeWebSocket.instances[1].open();
  expect(states.at(-1)).toBe("live");

  stop();
  expect(states.at(-1)).toBe("offline");
});

test("旧连接的延迟错误不会关闭新的连接", () => {
  const states: ConnectionState[] = [];
  const stop = connectRoomEvents({ roomID: "room-1", after: 0, onMessage: vi.fn(), onState: (state) => states.push(state) });
  const first = FakeWebSocket.instances[0];
  first.open();
  first.close();
  vi.advanceTimersByTime(1_000);
  const second = FakeWebSocket.instances[1];
  second.open();

  first.onerror?.();
  expect(second.readyState).toBe(FakeWebSocket.OPEN);
  expect(states.at(-1)).toBe("live");
  stop();
});
