import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface Assistant {
  id: string;
  name: string;
  slug: string;
  purpose: string;
  status: string;
}

@Injectable({ providedIn: 'root' })
export class AssistantService {
  private api = inject(ApiService);
  assistant = signal<Assistant | null>(null);
  workspace = signal<any>(null);
  onboarding = signal<any>(null);

  get() {
    return this.api.get<Assistant>('/assistant').pipe(tap(r => this.assistant.set(r.data as any)));
  }
  ensure() {
    return this.api.post<Assistant>('/assistant/ensure', {}).pipe(tap(r => this.assistant.set(r.data as any)));
  }
  inspectWorkspace() {
    return this.api.get<any>('/assistant/workspace').pipe(tap(r => this.workspace.set(r.data)));
  }
  getOnboarding() {
    return this.api.get<any>('/assistant/onboarding').pipe(tap(r => this.onboarding.set(r.data)));
  }
  diagnose(taskId: string, runId?: string) {
    return this.api.post<any>('/assistant/diagnose', { taskId, runId });
  }
}
