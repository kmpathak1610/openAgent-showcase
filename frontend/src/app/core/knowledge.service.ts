import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface KnowledgeSource {
  id: string;
  name: string;
  sourceType: string;
  status: string;
  createdAt: string;
}
export interface KnowledgeCollection {
  id: string;
  name: string;
  slug: string;
  description: string;
}
export interface Document {
  id: string;
  organizationId: string;
  projectId: string | null;
  collectionId: string | null;
  title: string;
  sourceType: string;
  sourceUrl?: string;
  mimeType: string;
  status: string;
  scope: string;
  version: number;
  createdAt: string;
  chunkCount?: number;
}

@Injectable({ providedIn: 'root' })
export class KnowledgeService {
  private api = inject(ApiService);
  sources = signal<KnowledgeSource[]>([]);
  collections = signal<KnowledgeCollection[]>([]);
  documents = signal<Document[]>([]);
  loading = signal(false);

  listSources() {
    return this.api.get<KnowledgeSource[]>('/knowledge/sources').pipe(tap(r => this.sources.set(r.data || [])));
  }
  createSource(name: string, sourceType: string, config: any = {}) {
    return this.api.post<KnowledgeSource>('/knowledge/sources', { name, sourceType, config }).pipe(tap(() => this.listSources().subscribe()));
  }

  listCollections() {
    return this.api.get<KnowledgeCollection[]>('/knowledge/collections').pipe(tap(r => this.collections.set(r.data || [])));
  }
  createCollection(name: string, description = '') {
    return this.api.post<KnowledgeCollection>('/knowledge/collections', { name, description }).pipe(tap(() => this.listCollections().subscribe()));
  }

  listDocuments(projectId?: string, scope?: string) {
    this.loading.set(true);
    const params: any = {};
    if (projectId) params.projectId = projectId;
    if (scope) params.scope = scope;
    return this.api.get<Document[]>('/knowledge/documents', params).pipe(tap(r => { this.documents.set(r.data || []); this.loading.set(false); }));
  }

  getDocument(id: string) {
    return this.api.get<Document>(`/knowledge/documents/${id}`);
  }

  getChunks(id: string) {
    return this.api.get<any[]>(`/knowledge/documents/${id}/chunks`);
  }

  // Supports file upload (FormData) or JSON
  createDocument(payload: { title: string; sourceType: string; content?: string; sourceUrl?: string; scope?: string; projectId?: string; collectionId?: string; file?: File }) {
    if (payload.file) {
      const fd = new FormData();
      fd.append('title', payload.title);
      fd.append('sourceType', payload.sourceType);
      if (payload.sourceUrl) fd.append('sourceUrl', payload.sourceUrl);
      if (payload.scope) fd.append('scope', payload.scope);
      if (payload.projectId) fd.append('projectId', payload.projectId);
      if (payload.collectionId) fd.append('collectionId', payload.collectionId);
      if (payload.content) fd.append('content', payload.content);
      fd.append('file', payload.file);
      // use raw fetch via ApiService? ApiService post does JSON, so we need direct HttpClient
      // fallback: use ApiService with custom
      return this.api.post<Document>('/knowledge/documents', fd as any);
    }
    return this.api.post<Document>('/knowledge/documents', {
      title: payload.title,
      sourceType: payload.sourceType,
      content: payload.content,
      sourceUrl: payload.sourceUrl,
      scope: payload.scope,
      projectId: payload.projectId,
      collectionId: payload.collectionId,
    });
  }

  deleteDocument(id: string) {
    return this.api.delete(`/knowledge/documents/${id}`).pipe(tap(() => this.documents.update(arr => arr.filter(d => d.id !== id))));
  }

  search(query: string, opts: { projectId?: string; agentId?: string; scope?: string; hybrid?: boolean; limit?: number } = {}) {
    return this.api.post<any[]>('/knowledge/search', {
      query,
      projectId: opts.projectId,
      agentId: opts.agentId,
      scope: opts.scope,
      hybrid: !!opts.hybrid,
      limit: opts.limit || 5,
    });
  }

  // attach
  attachToAgent(agentId: string, documentId?: string, collectionId?: string) {
    return this.api.post(`/agents/${agentId}/knowledge`, { documentId, collectionId });
  }
  listAgentKnowledge(agentId: string) {
    return this.api.get<any[]>(`/agents/${agentId}/knowledge`);
  }
  detachAgentKnowledge(agentId: string, knowledgeId: string) {
    return this.api.delete(`/agents/${agentId}/knowledge/${knowledgeId}`);
  }

  attachToProject(projectId: string, documentId?: string, collectionId?: string) {
    return this.api.post(`/projects/${projectId}/knowledge`, { documentId, collectionId });
  }
  listProjectKnowledge(projectId: string) {
    return this.api.get<any[]>(`/projects/${projectId}/knowledge`);
  }
  detachProjectKnowledge(projectId: string, knowledgeId: string) {
    return this.api.delete(`/projects/${projectId}/knowledge/${knowledgeId}`);
  }
}
