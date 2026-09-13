import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';

@Injectable({ providedIn: 'root' })
export class StatusService {
  private api = inject(ApiService);
  statuses = signal<any[]>([]);

  list() {
    return this.api.get<any[]>('/agents/status').subscribe(r => this.statuses.set(r.data || []));
  }

  get(agentId: string) {
    return this.api.get<any>(`/agents/${agentId}/status`);
  }
}
