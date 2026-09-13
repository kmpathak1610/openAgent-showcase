import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Tool {
  id: string;
  name: string;
  description: string;
  riskLevel: string;
  enabled: boolean;
}

@Injectable({ providedIn: 'root' })
export class ToolService {
  private api = inject(ApiService);
  tools = signal<Tool[]>([]);
  executions = signal<any[]>([]);

  list() {
    return this.api.get<Tool[]>('/tools').pipe(tap(r => this.tools.set(r.data || [])));
  }

  execute(name: string, input: any, agentId?: string, taskId?: string) {
    return this.api.post<any>(`/tools/${name}/execute`, { input, agentId, taskId });
  }

  listExecutions(agentId?: string, taskId?: string) {
    const params: any = {};
    if (agentId) params.agentId = agentId;
    if (taskId) params.taskId = taskId;
    return this.api.get<any[]>('/tool-executions', params).pipe(tap(r => this.executions.set(r.data || [])));
  }

  assignToAgent(agentId: string, toolName: string) {
    return this.api.post(`/agents/${agentId}/tools`, { toolName });
  }
}
