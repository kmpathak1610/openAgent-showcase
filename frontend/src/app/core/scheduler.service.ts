import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Trigger {
  id: string;
  name: string;
  triggerType: string;
  config: any;
  enabled: boolean;
  nextRunAt?: string;
  agentId?: string;
  teamId?: string;
  projectId?: string;
}

@Injectable({ providedIn: 'root' })
export class SchedulerService {
  private api = inject(ApiService);
  triggers = signal<Trigger[]>([]);
  schedules = signal<any[]>([]);

  list() {
    return this.api.get<Trigger[]>('/triggers').pipe(tap(r => this.triggers.set(r.data || [])));
  }

  create(payload: { name: string; triggerType: string; config: any; agentId?: string; teamId?: string; projectId?: string }) {
    return this.api.post<Trigger>('/triggers', payload).pipe(tap(() => this.list().subscribe()));
  }

  remove(id: string) {
    return this.api.delete(`/triggers/${id}`).pipe(tap(() => this.triggers.update(arr => arr.filter(t => t.id !== id))));
  }

  listSchedules() {
    return this.api.get<any[]>('/schedules').pipe(tap(r => this.schedules.set(r.data || [])));
  }
}
