import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface LLMProviderStatus { name: string; real: boolean; }
export interface LLMSettings { defaultModel: string; availableModels: string[]; providers: LLMProviderStatus[]; }

@Injectable({ providedIn: 'root' })
export class SettingsService {
  private api = inject(ApiService);
  config = signal<LLMSettings | null>(null);
  loading = signal(false);

  llm() {
    this.loading.set(true);
    return this.api.get<LLMSettings>('/settings/llm').pipe(
      tap(res => { this.config.set(res.data); this.loading.set(false); }),
    );
  }
}
