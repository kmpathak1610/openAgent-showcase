import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Integration {
  id: string;
  name: string;
  provider: string;
  status: string;
  config: any;
  createdAt?: string;
}

@Injectable({ providedIn: 'root' })
export class IntegrationService {
  private api = inject(ApiService);
  integrations = signal<Integration[]>([]);

  list() {
    return this.api.get<Integration[]>('/integrations').pipe(tap(r => this.integrations.set(r.data || [])));
  }

  create(name: string, provider: string, config: any = {}, credentials: any = {}) {
    return this.api.post<Integration>('/integrations', { name, provider, config, credentials }).pipe(tap(() => this.list().subscribe()));
  }

  remove(id: string) {
    return this.api.delete(`/integrations/${id}`).pipe(tap(() => this.integrations.update(arr => arr.filter(i => i.id !== id))));
  }
}
