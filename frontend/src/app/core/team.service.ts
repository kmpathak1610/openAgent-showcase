import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Team {
  id: string;
  name: string;
  slug: string;
  objective: string;
  description: string;
  status: string;
  members?: any[];
}

export interface TeamPreview {
  name: string;
  objective: string;
  description: string;
  agents: { name: string; role: string; responsibilities: string; dependencies: string[]; tools: string[]; knowledge: string[]; autonomy: string }[];
  workflow: any[];
  communicationRules: string[];
  delegationRules: string[];
  permissions: any;
  approvalPolicy: any;
}

@Injectable({ providedIn: 'root' })
export class TeamService {
  private api = inject(ApiService);
  teams = signal<Team[]>([]);
  selected = signal<Team | null>(null);
  preview = signal<TeamPreview | null>(null);
  dashboard = signal<any>(null);

  builderPreview(outcome: string) {
    return this.api.post<TeamPreview>('/teams/builder/preview', { outcome }).pipe(tap(r => this.preview.set(r.data)));
  }

  create(preview: TeamPreview, projectId?: string) {
    return this.api.post<Team>('/teams', { preview, projectId }).pipe(tap(r => this.teams.update(arr => [...arr, r.data])));
  }

  list() {
    return this.api.get<Team[]>('/teams').pipe(tap(r => this.teams.set(r.data || [])));
  }

  get(id: string) {
    return this.api.get<Team>(`/teams/${id}`).pipe(tap(r => this.selected.set(r.data)));
  }

  members(id: string) {
    return this.api.get<any[]>(`/teams/${id}/members`);
  }

  dashboardData(id: string) {
    return this.api.get<any>(`/teams/${id}/dashboard`).pipe(tap(r => this.dashboard.set(r.data)));
  }

  startTask(teamId: string, projectId: string, title: string, description: string, channelId?: string) {
    return this.api.post<any>(`/teams/${teamId}/start`, { projectId, channelId, title, description });
  }

  activity(teamId: string) {
    return this.api.get<any[]>(`/teams/${teamId}/activity`);
  }
}
