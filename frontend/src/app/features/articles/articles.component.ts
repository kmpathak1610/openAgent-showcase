import { Component, inject, signal, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { ArticleService } from '../../core/article.service';
import { ProjectService } from '../../core/project.service';
import { KnowledgeService } from '../../core/knowledge.service';

@Component({
  selector: 'app-articles',
  standalone: true,
  imports: [FormsModule, RouterLink],
  template: `
    <div class="wrap">
      <div class="head">
        <h2>Articles</h2>
        <button class="btn small" (click)="showCreate.set(!showCreate())">+ New Article</button>
      </div>
      @if (showCreate()) {
        <div class="card">
          <input data-testid="article-topic-input" [(ngModel)]="topic" placeholder="Topic, e.g. Vector DBs for beginners" class="input" />
          <div class="row">
            <select [(ngModel)]="projectId" class="input">
              <option value="">Select project</option>
              @for (p of projects.projects(); track p.id) {
                <option [value]="p.id">{{ p.name }}</option>
              }
            </select>
            <select [(ngModel)]="length" class="input">
              <option value="short">Short (300-800)</option>
              <option value="long">Long-form SEO</option>
              <option value="flexible">Flexible</option>
            </select>
            <select [(ngModel)]="format" class="input">
              <option value="blog">Blog</option>
              <option value="seo">SEO</option>
              <option value="docs">Docs</option>
            </select>
            <input [(ngModel)]="tone" placeholder="Tone, e.g. friendly" class="input" />
          </div>
          <textarea [(ngModel)]="sourceUrls" placeholder="Source URLs, one per line" class="input"></textarea>
          @if (createError()) { <div class="error">{{ createError() }}</div> }
          <button class="btn" (click)="create()" [disabled]="creating()">Create brief</button>
        </div>
      }
      @for (a of service.articles(); track a.id) {
        <a class="card" [routerLink]="['/articles', a.id]">
          <strong>{{ a.title }}</strong>
          <span class="badge">{{ a.status }}</span>
        </a>
      }
    </div>
  `,
  styles: [`
    .wrap{padding:24px;max-width:1200px}
    .head{display:flex;align-items:center;justify-content:space-between}
    .card{display:block;background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:12px;margin-top:10px;text-decoration:none;color:inherit}
    .row{display:flex;gap:8px;margin-top:8px;flex-wrap:wrap}
    .input{border:1px solid #e8eaee;border-radius:8px;padding:8px 10px;font-size:13px;flex:1}
    .btn{background:#0f1117;color:#fff;border:0;padding:8px 12px;border-radius:8px;cursor:pointer}
    .btn.small{font-size:12px}
    .badge{background:#f1f3f5;border-radius:999px;padding:2px 8px;font-size:11px;margin-left:8px}
    .error{color:#b42318;font-size:12px;margin-top:6px}
  `],
})
export class ArticlesComponent implements OnInit {
  service = inject(ArticleService);
  projects = inject(ProjectService);
  knowledge = inject(KnowledgeService);
  showCreate = signal(false);
  creating = signal(false);
  createError = signal('');
  topic = '';
  projectId = '';
  length: 'short' | 'long' | 'flexible' = 'short';
  tone = 'friendly';
  format: 'blog' | 'seo' | 'docs' = 'blog';
  sourceUrls = '';

  ngOnInit() {
    this.projects.list().subscribe();
    this.service.list().subscribe();
    this.knowledge.listCollections().subscribe();
  }

  create() {
    if (!this.topic.trim() || !this.projectId) {
      this.createError.set('Topic and project required');
      return;
    }
    this.creating.set(true);
    this.createError.set('');
    this.service.create({
      projectId: this.projectId,
      topic: this.topic.trim(),
      length: this.length,
      tone: this.tone,
      format: this.format,
      sourceUrls: this.sourceUrls.split('\n').map(s => s.trim()).filter(Boolean),
    }).subscribe({
      next: () => {
        this.topic = '';
        this.sourceUrls = '';
        this.showCreate.set(false);
        this.creating.set(false);
      },
      error: (e) => {
        this.createError.set(e?.error?.error?.message || 'Create failed');
        this.creating.set(false);
      },
    });
  }
}
