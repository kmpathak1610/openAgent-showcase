import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Reaction {
  id: string;
  messageId: string;
  userId: string;
  emoji: string;
}

export interface Message {
  id: string;
  organizationId: string;
  channelId: string;
  threadId: string | null;
  senderType: string;
  senderUserId: string | null;
  body: string;
  createdAt: string;
  updatedAt: string;
  editedAt?: string;
  deletedAt?: string;
  reactions?: Reaction[];
  threadCount?: number;
  sender?: { id: string; displayName: string; email: string };
}

@Injectable({ providedIn: 'root' })
export class MessageService {
  private api = inject(ApiService);
  messages = signal<Message[]>([]);
  loading = signal(false);
  hasMore = signal(true);
  private beforeCursor: string | null = null;

  list(channelId: string, opts?: { before?: string; threadId?: string; pageSize?: number }) {
    this.loading.set(true);
    const params: Record<string, string | number> = { channelId };
    if (opts?.before) params['before'] = opts.before;
    if (opts?.threadId) params['threadId'] = opts.threadId;
    if (opts?.pageSize) params['pageSize'] = opts.pageSize;
    return this.api.get<Message[]>('/messages', params as any).pipe(
      tap(res => {
        const fetched = res.data || [];
        if (opts?.before) {
          this.messages.update(arr => [...fetched.reverse(), ...arr]);
        } else {
          // initial load: sort ascending for display
          this.messages.set(fetched.reverse());
        }
        this.beforeCursor = fetched.length ? fetched[fetched.length - 1]?.createdAt : null;
        this.hasMore.set(fetched.length >= (opts?.pageSize || 20));
        this.loading.set(false);
      }),
    );
  }

  clear() {
    this.messages.set([]);
    this.beforeCursor = null;
    this.hasMore.set(true);
  }

  loadMore(channelId: string) {
    if (!this.beforeCursor || !this.hasMore()) return;
    this.list(channelId, { before: this.beforeCursor }).subscribe();
  }

  send(channelId: string, body: string, threadId?: string) {
    const payload: any = { channelId, body };
    if (threadId) payload.threadId = threadId;
    return this.api.post<Message>('/messages', payload).pipe(
      tap(res => {
        if (!threadId) this.messages.update(arr => [...arr, res.data]);
      }),
    );
  }

  edit(id: string, body: string) {
    return this.api.put<Message>(`/messages/${id}`, { body }).pipe(
      tap(res => this.messages.update(arr => arr.map(m => (m.id === id ? res.data : m)))),
    );
  }

  remove(id: string) {
    return this.api.delete(`/messages/${id}`).pipe(
      tap(() => this.messages.update(arr => arr.filter(m => m.id !== id))),
    );
  }

  replies(messageId: string) {
    return this.api.get<Message[]>(`/messages/${messageId}/replies`);
  }

  react(messageId: string, emoji: string) {
    return this.api.post<Reaction>(`/messages/${messageId}/reactions`, { emoji }).pipe(
      tap(res => {
        this.messages.update(arr =>
          arr.map(m => (m.id === messageId ? { ...m, reactions: [...(m.reactions || []), res.data] } : m)),
        );
      }),
    );
  }

  unreact(messageId: string, emoji: string) {
    return this.api.delete(`/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`).pipe(
      tap(() => {
        this.messages.update(arr =>
          arr.map(m => (m.id === messageId ? { ...m, reactions: (m.reactions || []).filter(r => r.emoji !== emoji) } : m)),
        );
      }),
    );
  }

  // called by WS events
  handleIncoming(event: { type: string; payload: any }) {
    const t = event.type;
    const p = event.payload;
    if (t === 'message.created' && p.message) {
      const msg = p.message as Message;
      // only append if currently viewing that channel and not a thread reply
      if (!msg.threadId) {
        const current = this.messages();
        if (!current.find(m => m.id === msg.id)) {
          this.messages.update(arr => [...arr, msg]);
        }
      }
    } else if (t === 'message.updated' && p.message) {
      const msg = p.message as Message;
      this.messages.update(arr => arr.map(m => (m.id === msg.id ? { ...m, ...msg } : m)));
    } else if (t === 'message.deleted' && p.messageId) {
      this.messages.update(arr => arr.filter(m => m.id !== p.messageId));
    } else if (t === 'reaction.added' && p.reaction) {
      const r = p.reaction as Reaction;
      this.messages.update(arr =>
        arr.map(m => (m.id === r.messageId ? { ...m, reactions: [...(m.reactions || []), r] } : m)),
      );
    } else if (t === 'reaction.removed' && p.messageId) {
      this.messages.update(arr =>
        arr.map(m =>
          m.id === p.messageId ? { ...m, reactions: (m.reactions || []).filter(x => x.emoji !== p.emoji) } : m,
        ),
      );
    }
  }
}
