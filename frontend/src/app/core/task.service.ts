import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Task {
  id: string;
  organizationId: string;
  projectId: string;
  channelId: string | null;
  parentTaskId: string | null;
  title: string;
  description: string;
  status: string;
  priority: string;
  deadline?: string;
  assignedToAgent?: string | null;
  assignedToUser?: string | null;
  correlationId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface TaskEvent {
  id: string;
  taskId: string;
  actorType: string;
  actorUserId?: string;
  actorAgentId?: string;
  eventType: string;
  payload: any;
  createdAt: string;
}

export interface AgentRun {
  id: string;
  agentId: string;
  taskId?: string;
  status: string;
  triggerType?: string;
  correlationId?: string;
  input: any;
  output?: any;
  createdAt: string;
  actions?: any[];
  tokenMetadata?: {
    promptTokens?: number;
    completionTokens?: number;
    totalTokens?: number;
    estimatedCost?: number;
  };
}

@Injectable({ providedIn: 'root' })
export class TaskService {
  private api = inject(ApiService);
  tasks = signal<Task[]>([]);
  selected = signal<Task | null>(null);
  events = signal<TaskEvent[]>([]);
  runs = signal<AgentRun[]>([]);
  loading = signal(false);

  list(projectId?: string, status?: string) {
    this.loading.set(true);
    const params: any = {};
    if (projectId) params.projectId = projectId;
    if (status) params.status = status;
    return this.api.get<Task[]>('/tasks', params).pipe(
      tap(res => { this.tasks.set(res.data || []); this.loading.set(false); }),
    );
  }

  get(id: string) {
    return this.api.get<Task>(`/tasks/${id}`).pipe(tap(res => this.selected.set(res.data)));
  }

  create(payload: any) {
    return this.api.post<Task>('/tasks', payload).pipe(
      tap(res => this.tasks.update(arr => [res.data, ...arr])),
    );
  }

  updateStatus(id: string, status: string) {
    return this.api.patch<Task>(`/tasks/${id}`, { status }).pipe(
      tap(res => {
        this.tasks.update(arr => arr.map(t => t.id === id ? res.data : t));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  assign(id: string, agentId?: string, userId?: string) {
    return this.api.post<Task>(`/tasks/${id}/assign`, { agentId, userId }).pipe(
      tap(res => {
        this.tasks.update(arr => arr.map(t => t.id === id ? res.data : t));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  delegate(parentId: string, payload: { title: string; description?: string; assignedToAgent?: string }) {
    return this.api.post<Task>(`/tasks/${parentId}/delegate`, payload).pipe(
      tap(res => this.tasks.update(arr => [res.data, ...arr])),
    );
  }

  cancel(id: string) {
    return this.api.post<Task>(`/tasks/${id}/cancel`, {}).pipe(
      tap(res => {
        this.tasks.update(arr => arr.map(t => t.id === id ? res.data : t));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  listEvents(id: string) {
    return this.api.get<TaskEvent[]>(`/tasks/${id}/events`).pipe(tap(res => this.events.set(res.data || [])));
  }

  listRuns(taskId: string) {
    return this.api.get<AgentRun[]>(`/tasks/${taskId}/runs`).pipe(tap(res => this.runs.set(res.data || [])));
  }

  // global runs
  listRunsGlobal(agentId?: string) {
    const params: any = {};
    if (agentId) params.agentId = agentId;
    return this.api.get<AgentRun[]>('/runs', params);
  }

  handleIncoming(event: { type: string; payload: any }) {
    const t = event.type;
    const p = event.payload;
    if (t === 'agent.task_created' && p.task) {
      const task = p.task as Task;
      if (!this.tasks().find(x => x.id === task.id)) {
        this.tasks.update(arr => [task, ...arr]);
      }
    } else if (t === 'agent.task_assigned' && p.taskId) {
      this.tasks.update(arr => arr.map(x => x.id === p.taskId ? { ...x, status: 'assigned', assignedToAgent: p.assignee } : x));
    } else if (t === 'agent.delegated' && p.subtask) {
      const sub = p.subtask as Task;
      if (!this.tasks().find(x => x.id === sub.id)) {
        this.tasks.update(arr => [sub, ...arr]);
      }
    } else if (t === 'agent.task_updated' && p.task) {
      const task = p.task as Task;
      this.tasks.update(arr => arr.map(x => x.id === task.id ? task : x));
      if (this.selected()?.id === task.id) this.selected.set(task);
    }
  }
}
