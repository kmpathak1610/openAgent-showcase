import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { Organization } from './auth.service';
import { tap } from 'rxjs';

export interface OrgMember {
  id: string;
  organizationId: string;
  userId: string;
  role: string;
  user?: { id: string; email: string; displayName: string };
}

@Injectable({ providedIn: 'root' })
export class OrgService {
  private api = inject(ApiService);
  currentOrg = signal<Organization | null>(null);
  members = signal<OrgMember[]>([]);

  list() {
    return this.api.get<Organization[]>('/organizations');
  }

  create(name: string) {
    return this.api.post<Organization>('/organizations', { name });
  }

  get(id: string) {
    return this.api.get<Organization>(`/organizations/${id}`);
  }

  update(id: string, name: string) {
    return this.api.put<Organization>(`/organizations/${id}`, { name });
  }

  listMembers(orgId: string) {
    return this.api.get<OrgMember[]>(`/organizations/${orgId}/members`).pipe(
      tap(res => this.members.set(res.data || [])),
    );
  }

  addMember(orgId: string, email: string, role = 'member') {
    return this.api.post(`/organizations/${orgId}/members`, { email, role });
  }

  removeMember(orgId: string, userId: string) {
    return this.api.delete(`/organizations/${orgId}/members/${userId}`);
  }
}
