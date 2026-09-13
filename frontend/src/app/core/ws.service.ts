import { Injectable, inject, signal } from '@angular/core';
import { Subject, Observable } from 'rxjs';
import { AuthService } from './auth.service';
import { environment } from '../../environments/environment';

export interface WsEvent {
  id?: string;
  type: string;
  payload: Record<string, unknown>;
  timestamp?: string;
  organizationId?: string;
}

@Injectable({ providedIn: 'root' })
export class WsService {
  private auth = inject(AuthService);
  private ws: WebSocket | null = null;
  private incoming$ = new Subject<WsEvent>();
  private _connected = signal(false);

  connected = this._connected.asReadonly();
  events: Observable<WsEvent> = this.incoming$.asObservable();

  private reconnectAttempts = 0;
  private reconnectTimer: any = null;
  private seenIds = new Set<string>();
  private heartbeatTimer: any = null;

  connect() {
    const token = this.auth.getToken();
    if (!token || this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) return;

    const url = `${environment.wsBase}?token=${encodeURIComponent(token)}`;
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this._connected.set(true);
      this.reconnectAttempts = 0;
      // Heartbeat: send ping every 30s
      this.heartbeatTimer = setInterval(() => this.send('ping', {}), 30000);
    };
    this.ws.onclose = () => {
      this._connected.set(false);
      clearInterval(this.heartbeatTimer);
      this.scheduleReconnect();
    };
    this.ws.onerror = () => {
      this.ws?.close();
    };
    this.ws.onmessage = ev => {
      try {
        const data = JSON.parse(ev.data) as WsEvent;
        // Deduplicate via event ID
        if (data.id && this.seenIds.has(data.id)) return;
        if (data.id) {
          this.seenIds.add(data.id);
          // cleanup old ids every 1000
          if (this.seenIds.size > 1000) {
            const first = this.seenIds.values().next().value;
            if (first) this.seenIds.delete(first);
          }
        }
        // Verify organization: only accept events for current org
        const currentOrg = this.auth.org()?.id;
        if (data.organizationId && currentOrg && data.organizationId !== currentOrg) {
          return;
        }
        // Also check payload organizationId
        const payloadOrg = (data.payload as any)?.organizationId;
        if (payloadOrg && currentOrg && payloadOrg !== currentOrg) {
          return;
        }
        // Handle pong/heartbeat
        if (data.type === 'pong') return;
        this.incoming$.next(data);
      } catch {}
    };
  }

  private scheduleReconnect() {
    if (!this.auth.isAuthenticated()) return;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
    this.reconnectAttempts++;
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = setTimeout(() => this.connect(), delay);
  }

  joinChannel(channelId: string) {
    this.send('join_channel', { channelId });
  }

  leaveChannel(channelId: string) {
    this.send('leave_channel', { channelId });
  }

  disconnect() {
    clearInterval(this.heartbeatTimer);
    this.ws?.close();
    this.ws = null;
    this._connected.set(false);
  }

  send(type: string, payload: Record<string, unknown> = {}) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, payload }));
    }
  }
}
