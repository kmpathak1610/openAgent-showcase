import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';
import { tap } from 'rxjs';

export interface ArticleBrief {
  projectId: string;
  topic: string;
  length: 'short' | 'long' | 'flexible';
  tone: string;
  format: 'blog' | 'seo' | 'docs';
  sourceUrls?: string[];
  documentIds?: string[];
  knowledgeCollectionIds?: string[];
}

export interface Article {
  id: string;
  title: string;
  description: string;
  status: string;
  projectId: string;
}

@Injectable({ providedIn: 'root' })
export class ArticleService {
  private api = inject(ApiService);
  articles = signal<Article[]>([]);
  selected = signal<Article | null>(null);
  loading = signal(false);

  list(projectId?: string) {
    this.loading.set(true);
    const params: any = {};
    if (projectId) params.projectId = projectId;
    return this.api.get<Article[]>('/tasks', params).pipe(
      tap(res => {
        const rows = (res.data || []).filter((t: any) => (t.title || '').startsWith('Article:'));
        this.articles.set(rows as any);
        this.loading.set(false);
      }),
    );
  }

  create(brief: ArticleBrief) {
    // TODO: ingest via /knowledge/documents in follow-up; for now encode sources into description to avoid dropping textarea input.
    let description = `topic=${brief.topic}; length=${brief.length}; tone=${brief.tone}; format=${brief.format}`;
    const sources = (brief.sourceUrls || []).map(s => s.trim()).filter(Boolean);
    if (sources.length) {
      description += `; sources=${sources.join(',')}`;
    }
    const body = {
      projectId: brief.projectId,
      title: `Article: ${brief.topic}`,
      description,
      priority: 'medium' as const,
    };
    return this.api.post<Article>('/tasks', body).pipe(
      tap(res => this.articles.update(arr => [res.data, ...arr])),
    );
  }

  get(id: string) {
    return this.api.get<Article>(`/tasks/${id}`).pipe(tap(res => this.selected.set(res.data)));
  }
}
