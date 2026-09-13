import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface AgentCapability {
  name: string;
  description: string;
  enabled: boolean;
}

export interface AgentPermission {
  id: string;
  resourceType: string;
  resourceId: string | null;
  permission: string;
}

export interface Agent {
  id: string;
  organizationId: string;
  name: string;
  slug: string;
  description: string;
  purpose: string;
  avatar?: string;
  avatarUrl?: string;
  status: string;
  autonomyLevel: string;
  currentVersion: number;
  ownerId?: string;
  createdAt: string;
  updatedAt: string;
  version?: any;
  capabilityList?: AgentCapability[];
  permissions?: AgentPermission[];
}

export interface BuilderInput {
  description: string;
  responsibilities: string[];
  informationSources: string[];
  actions: string[];
  autonomyPreference: string;
}

export interface BuilderPreview {
  name: string;
  description: string;
  purpose: string;
  role: string;
  objective: string;
  responsibilities: string[];
  instructions: string;
  capabilities: AgentCapability[];
  behavioralRules: string[];
  toolPolicy: any;
  memoryPolicy: any;
  approvalPolicy: any;
  modelConfiguration: any;
  autonomyLevel: string;
  avatar: string;
  knowledgeNeeds: string[];
  recommendedTools: string[];
  permissions: any[];
  warnings?: string[];
}

@Injectable({ providedIn: 'root' })
export class AgentService {
  private api = inject(ApiService);
  agents = signal<Agent[]>([]);
  selected = signal<Agent | null>(null);
  loading = signal(false);
  preview = signal<BuilderPreview | null>(null);

  list() {
    this.loading.set(true);
    return this.api.get<Agent[]>('/agents').pipe(
      tap(res => {
        this.agents.set(res.data || []);
        this.loading.set(false);
      }),
    );
  }

  get(id: string) {
    return this.api.get<Agent>(`/agents/${id}`).pipe(tap(res => this.selected.set(res.data)));
  }

  builderPreview(input: BuilderInput) {
    return this.api.post<BuilderPreview>('/agents/builder/preview', input).pipe(
      tap(res => this.preview.set(res.data)),
    );
  }

  create(preview: BuilderPreview, intent?: string) {
    return this.api.post<Agent>('/agents', { preview, intent }).pipe(
      tap(res => this.agents.update(arr => [...arr, res.data])),
    );
  }

  update(id: string, patch: any) {
    return this.api.put<Agent>(`/agents/${id}`, patch).pipe(
      tap(res => {
        this.agents.update(arr => arr.map(a => (a.id === id ? res.data : a)));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  versions(id: string) {
    return this.api.get<any[]>(`/agents/${id}/versions`);
  }

  versionDetail(id: string, version: number) {
    return this.api.get<any>(`/agents/${id}/versions/${version}`);
  }

  permissions(id: string) {
    return this.api.get<AgentPermission[]>(`/agents/${id}/permissions`);
  }
}
