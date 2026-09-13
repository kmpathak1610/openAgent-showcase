import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Memory {
  id: string;
  memoryType: string;
  scope: string;
  content: string;
  importance: number;
  confidence: number;
  status: string; // candidate | validating | active | merged | superseded | stale | archived | conflict
  agentId?: string;
  projectId?: string;
  conversationId?: string;
  taskId?: string;
  source: string;
  sourceEventId?: string;
  contentHash?: string;
  version: number;
  expiresAt?: string;
  lastAccessedAt?: string;
  lastConfirmedAt?: string;
  createdAt: string;
  updatedAt: string;
  metadata?: any;
}

export interface MemoryVersion {
  id: string;
  memoryId: string;
  version: number;
  content: string;
  importance: number;
  confidence: number;
  status: string;
  source: string;
  reason: string;
  createdAt: string;
}

@Injectable({ providedIn: 'root' })
export class MemoryService {
  private api = inject(ApiService);
  memories = signal<Memory[]>([]);

  list(filters: { agentId?: string; projectId?: string; scope?: string; memoryType?: string; status?: string } = {}) {
    const params: any = {};
    if (filters.agentId) params.agentId = filters.agentId;
    if (filters.projectId) params.projectId = filters.projectId;
    if (filters.scope) params.scope = filters.scope;
    if (filters.memoryType) params.memoryType = filters.memoryType;
    if (filters.status) params.status = filters.status;
    return this.api.get<Memory[]>('/memories', params).pipe(tap(r => this.memories.set(r.data || [])));
  }

  // List including archived/stale for admin/debug (uses status filter)
  listAll(filters: { agentId?: string; projectId?: string; scope?: string; memoryType?: string } = {}) {
    const params: any = { all: 'true', ...filters };
    if (filters.agentId) params.agentId = filters.agentId;
    if (filters.projectId) params.projectId = filters.projectId;
    return this.api.get<Memory[]>('/memories', params).pipe(tap(r => this.memories.set(r.data || [])));
  }

  get(id: string) {
    return this.api.get<{ memory: Memory; versions: MemoryVersion[] }>(`/memories/${id}`);
  }

  versions(id: string) {
    return this.api.get<MemoryVersion[]>(`/memories/${id}/versions`);
  }

  create(payload: { agentId?: string; projectId?: string; conversationId?: string; taskId?: string; memoryType: string; scope?: string; source?: string; content: string; importance?: number; confidence?: number }) {
    return this.api.post<any>('/memories', payload).pipe(tap(() => this.list().subscribe()));
  }

  search(query: string, agentId?: string, projectId?: string, limit = 5) {
    return this.api.post<Memory[]>('/memories/search', { query, agentId, projectId, limit });
  }

  archive(id: string) {
    return this.api.post(`/memories/${id}/archive`, {}).pipe(tap(() => this.memories.update(arr => arr.map(m => m.id === id ? { ...m, status: 'archived' } : m))));
  }

  restore(id: string) {
    return this.api.post(`/memories/${id}/restore`, {}).pipe(tap(() => this.list().subscribe()));
  }

  remove(id: string) {
    return this.api.delete(`/memories/${id}`).pipe(tap(() => this.memories.update(arr => arr.filter(m => m.id !== id))));
  }
}
