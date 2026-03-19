"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { getToken } from "@/lib/api";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "";

interface SSEEvent {
  event: string;
  data: string;
  id?: string;
}

type SSEStatus = "connecting" | "connected" | "disconnected" | "error";

interface UseSSEOptions {
  /** URL path for the SSE endpoint */
  url: string;
  /** Event types to listen for */
  events?: string[];
  /** Whether to auto-reconnect on disconnect (default: true) */
  autoReconnect?: boolean;
  /** Reconnect delay in ms (default: 3000) */
  reconnectDelay?: number;
  /** Maximum reconnect attempts (default: 10) */
  maxReconnects?: number;
}

interface UseSSEResult<T> {
  /** Latest data received from the SSE stream */
  data: T | null;
  /** Connection status */
  status: SSEStatus;
  /** Last error message */
  error: string | null;
  /** Manually close the connection */
  close: () => void;
  /** Manually reconnect */
  reconnect: () => void;
}

/**
 * useSSE connects to a Server-Sent Events endpoint and returns real-time data.
 * Falls back to polling if SSE is not available.
 */
export function useSSE<T>(
  options: UseSSEOptions,
  transform?: (event: SSEEvent) => T | null,
  deps: unknown[] = []
): UseSSEResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [status, setStatus] = useState<SSEStatus>("connecting");
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectCountRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const {
    url,
    events = ["run_status", "agent_result", "approval_request", "kill_switch"],
    autoReconnect = true,
    reconnectDelay = 3000,
    maxReconnects = 10,
  } = options;

  const cleanup = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    cleanup();

    const token = getToken();
    if (!token) {
      setStatus("error");
      setError("Not authenticated");
      return;
    }

    // EventSource doesn't support custom headers, so pass token as query param.
    // The backend should accept ?token= for SSE endpoints.
    const sseUrl = `${API_BASE}${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}`;

    setStatus("connecting");
    setError(null);

    const eventSource = new EventSource(sseUrl);
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      setStatus("connected");
      setError(null);
      reconnectCountRef.current = 0;
    };

    // Listen for the "connected" event from the server
    eventSource.addEventListener("connected", () => {
      setStatus("connected");
    });

    // Register listeners for each event type
    for (const eventType of events) {
      eventSource.addEventListener(eventType, (e: MessageEvent) => {
        const sseEvent: SSEEvent = {
          event: eventType,
          data: e.data,
          id: e.lastEventId || undefined,
        };

        if (transform) {
          const transformed = transform(sseEvent);
          if (transformed !== null) {
            setData(transformed);
          }
        } else {
          try {
            setData(JSON.parse(e.data) as T);
          } catch {
            // If data isn't JSON, store as-is
            setData(e.data as unknown as T);
          }
        }
      });
    }

    eventSource.onerror = () => {
      setStatus("disconnected");

      if (autoReconnect && reconnectCountRef.current < maxReconnects) {
        reconnectCountRef.current++;
        const delay = reconnectDelay * Math.min(reconnectCountRef.current, 5);
        setError(`Connection lost. Reconnecting in ${Math.round(delay / 1000)}s...`);
        reconnectTimerRef.current = setTimeout(connect, delay);
      } else if (reconnectCountRef.current >= maxReconnects) {
        setStatus("error");
        setError("Connection lost. Max reconnection attempts reached.");
        eventSource.close();
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, autoReconnect, reconnectDelay, maxReconnects, ...deps]);

  useEffect(() => {
    connect();
    return cleanup;
  }, [connect, cleanup]);

  const close = useCallback(() => {
    cleanup();
    setStatus("disconnected");
  }, [cleanup]);

  const reconnect = useCallback(() => {
    reconnectCountRef.current = 0;
    connect();
  }, [connect]);

  return { data, status, error, close, reconnect };
}

/**
 * useRunSSE connects to the run-specific SSE endpoint for real-time run updates.
 */
export function useRunSSE(runId: string) {
  return useSSE<{ run_id: string; status: string; message?: string }>(
    {
      url: `/api/v1/runs/${runId}/events`,
      events: ["run_status", "agent_result"],
    },
    (event) => {
      try {
        const envelope = JSON.parse(event.data);
        return envelope.payload || envelope;
      } catch {
        return null;
      }
    },
    [runId]
  );
}

/**
 * useGlobalSSE connects to the global SSE endpoint for all platform events.
 */
export function useGlobalSSE() {
  return useSSE<{ event: string; payload: unknown }>(
    {
      url: "/api/v1/events/stream",
      events: ["run_status", "agent_result", "approval_request", "kill_switch"],
    },
    (event) => {
      try {
        const parsed = JSON.parse(event.data);
        return { event: event.event, payload: parsed.payload || parsed };
      } catch {
        return null;
      }
    }
  );
}
