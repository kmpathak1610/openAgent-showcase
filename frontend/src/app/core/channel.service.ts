import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Channel {
  id: string;
  organizationId: string;
  projectId: string | null;
  name: string;
  displayName: string;
  description: string;
  topic: string;
  channelType: string;
  createdAt: string;
}

@Injectable({ providedIn: 'root' })
export class ChannelService {
  private api = inject(ApiService);
  channels = signal<Channel[]>([]);
  selectedId = signal<string | null>(null);
  selected = signal<Channel | null>(null);
  loading = signal(false);

  list(projectId?: string) {
    this.loading.set(true);
    const params: Record<string, string> = {};
    if (projectId) params['projectId'] = projectId;
    return this.api.get<Channel[]>('/channels', params).pipe(
      tap(res => {
        this.channels.set(res.data || []);
        this.loading.set(false);
      }),
    );
  }

  create(payload: { name: string; displayName?: string; description?: string; projectId?: string; channelType?: string }) {
    return this.api.post<Channel>('/channels', payload).pipe(
      tap(res => this.channels.update(arr => [...arr, res.data])),
    );
  }

  get(id: string) {
    return this.api.get<Channel>(`/channels/${id}`).pipe(tap(res => this.selected.set(res.data)));
  }

  rename(id: string, name: string) {
    return this.api.post<Channel>(`/channels/${id}/rename`, { name }).pipe(
      tap(res => {
        this.channels.update(arr => arr.map(c => (c.id === id ? res.data : c)));
        if (this.selected()?.id === id) this.selected.set(res.data);
      }),
    );
  }

  update(id: string, payload: Partial<Channel>) {
    return this.api.put<Channel>(`/channels/${id}`, payload);
  }

  archive(id: string) {
    return this.api.post(`/channels/${id}/archive`, {}).pipe(
      tap(() => this.channels.update(arr => arr.filter(c => c.id !== id))),
    );
  }

  select(id: string) {
    this.selectedId.set(id);
    const found = this.channels().find(c => c.id === id) || null;
    if (found) this.selected.set(found);
    else this.get(id).subscribe();
  }
}
