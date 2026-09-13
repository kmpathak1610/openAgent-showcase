import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Project {
  id: string;
  organizationId: string;
  name: string;
  slug: string;
  description: string;
  objective: string;
  icon: string;
  status: string;
  createdAt: string;
}

@Injectable({ providedIn: 'root' })
export class ProjectService {
  private api = inject(ApiService);
  projects = signal<Project[]>([]);
  loading = signal(false);
  selected = signal<Project | null>(null);

  list() {
    this.loading.set(true);
    return this.api.get<Project[]>('/projects').pipe(
      tap(res => {
        this.projects.set(res.data || []);
        this.loading.set(false);
      }),
    );
  }

  create(payload: { name: string; description?: string; objective?: string; icon?: string }) {
    return this.api.post<Project>('/projects', payload).pipe(
      tap(res => this.projects.update(arr => [...arr, res.data])),
    );
  }

  get(id: string) {
    return this.api.get<Project>(`/projects/${id}`).pipe(tap(res => this.selected.set(res.data)));
  }

  update(id: string, payload: Partial<Project>) {
    return this.api.put<Project>(`/projects/${id}`, payload).pipe(
      tap(res => {
        this.projects.update(arr => arr.map(p => (p.id === id ? res.data : p)));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  archive(id: string) {
    return this.api.post(`/projects/${id}/archive`, {}).pipe(
      tap(() => this.projects.update(arr => arr.filter(p => p.id !== id))),
    );
  }

  members(projectId: string) {
    return this.api.get<any[]>(`/projects/${projectId}/members`);
  }

  addMember(projectId: string, email: string, role = 'member') {
    return this.api.post(`/projects/${projectId}/members`, { email, role });
  }
}
