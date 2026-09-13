import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Approval {
  id: string;
  title: string;
  description: string;
  riskLevel: string;
  action: string;
  target: string;
  status: string;
  requesterType: string;
  payload: any;
  createdAt: string;
}

@Injectable({ providedIn: 'root' })
export class ApprovalService {
  private api = inject(ApiService);
  approvals = signal<Approval[]>([]);
  pending = signal<Approval[]>([]);

  list(status?: string) {
    const params: any = {};
    if (status) params.status = status;
    return this.api.get<Approval[]>('/approvals', params).pipe(tap(r => {
      this.approvals.set(r.data || []);
      this.pending.set((r.data || []).filter(a => a.status === 'pending'));
    }));
  }

  approve(id: string) {
    return this.api.post<Approval>(`/approvals/${id}/approve`, {}).pipe(tap(() => this.list().subscribe()));
  }

  reject(id: string, reason?: string) {
    return this.api.post<Approval>(`/approvals/${id}/reject`, { reason }).pipe(tap(() => this.list().subscribe()));
  }

  cancel(id: string) {
    return this.api.post<Approval>(`/approvals/${id}/cancel`, {});
  }
}
