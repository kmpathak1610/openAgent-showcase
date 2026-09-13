import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface BrowserProfile {
  id: string;
  provider: string;
  name: string;
  status: string;
  organizationId: string;
  ownerUserId: string;
  createdAt: string;
  lastUsedAt?: string;
}

export interface BrowserSession {
  id: string;
  browserProfileId: string;
  status: string;
  startedAt: string;
  expiresAt: string;
}

@Injectable({ providedIn: 'root' })
export class BrowserService {
  private api = inject(ApiService);
  profiles = signal<BrowserProfile[]>([]);
  sessions = signal<BrowserSession[]>([]);

  listProfiles() {
    return this.api.get<BrowserProfile[]>('/browser/profiles').pipe(tap(r => this.profiles.set(r.data || [])));
  }
  createProfile(provider: string, name: string) {
    return this.api.post<BrowserProfile>('/browser/profiles', { provider, name }).pipe(tap(() => this.listProfiles().subscribe()));
  }
  deleteProfile(id: string) {
    return this.api.delete(`/browser/profiles/${id}`).pipe(tap(() => this.profiles.update(a => a.filter(p => p.id !== id))));
  }
  updateStatus(id: string, status: string) {
    return this.api.patch(`/browser/profiles/${id}`, { status });
  }
  createSession(profileId: string) {
    return this.api.post<BrowserSession>(`/browser/profiles/${profileId}/sessions`, {}).pipe(tap(() => this.listSessions().subscribe()));
  }
  listSessions() {
    return this.api.get<BrowserSession[]>('/browser/sessions').pipe(tap(r => this.sessions.set(r.data || [])));
  }
  closeSession(id: string) {
    return this.api.post(`/browser/sessions/${id}/close`, {}).pipe(tap(() => this.sessions.update(a => a.filter(s => s.id !== id))));
  }
  audit() {
    return this.api.get<any[]>('/browser/audit');
  }
  // capability status for assistant
  capabilityStatus() {
    return this.api.get<any>('/assistant/workspace'); // reuse
  }
}
