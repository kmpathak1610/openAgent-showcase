import { Component, inject, signal, OnInit, OnDestroy } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { Subscription } from 'rxjs';
import { ArticleService } from '../../core/article.service';
import { TaskService } from '../../core/task.service';
import { KnowledgeService } from '../../core/knowledge.service';
import { ApprovalService } from '../../core/approval.service';
import { WsService } from '../../core/ws.service';

@Component({
  selector: 'app-article-detail',
  standalone: true,
  template: `
    <div class="wrap">
      <h2>{{ article.selected()?.title || 'Article' }}</h2>
      <p class="muted">{{ article.selected()?.description }}</p>
      <div class="grid">
        <div class="card">
          <h3>Research</h3>
          <button class="btn small" (click)="search()">Search knowledge</button>
          @for (r of results(); track $index) {
            <div class="row">{{ r?.document?.title || r?.chunk?.documentId }} — {{ r?.score }}</div>
          }
        </div>
        <div class="card">
          <h3>Runs</h3>
          @for (run of tasks.runs(); track run.id) {
            <div class="row">{{ run.status }} — {{ run.tokenMetadata?.totalTokens || 0 }} tokens</div>
          }
        </div>
        <div class="card">
          <h3>Events</h3>
          @for (e of tasks.events(); track e.id) {
            <div class="row">{{ e.eventType }}</div>
          }
        </div>
      </div>
    </div>
  `,
  styles: [`
    .wrap{padding:24px;max-width:1200px}
    .grid{display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px;margin-top:12px}
    .card{border:1px solid #e8eaee;border-radius:10px;padding:12px}
    .row{font-size:12px;padding:4px 0;border-top:1px solid #f1f3f5}
    .muted{color:#6b7280}
    .btn{background:#0f1117;color:#fff;border:0;padding:6px 10px;border-radius:6px;cursor:pointer}
    .btn.small{font-size:12px}
  `],
})
export class ArticleDetailComponent implements OnInit, OnDestroy {
  private route = inject(ActivatedRoute);
  article = inject(ArticleService);
  tasks = inject(TaskService);
  knowledge = inject(KnowledgeService);
  approvals = inject(ApprovalService);
  ws = inject(WsService);
  results = signal<any[]>([]);
  private sub?: Subscription;

  ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id') || '';
    if (!id) return;
    this.article.get(id).subscribe();
    this.tasks.listEvents(id).subscribe();
    this.tasks.listRuns(id).subscribe();
    this.sub = this.ws.events.subscribe(ev => {
      if ((ev.payload as any)?.taskId === id || (ev.payload as any)?.task?.id === id) {
        this.tasks.listEvents(id).subscribe();
        this.tasks.listRuns(id).subscribe();
      }
    });
  }

  search() {
    const topic = (this.article.selected()?.title || '').replace('Article: ', '');
    this.knowledge.search(topic, { hybrid: true, limit: 5 }).subscribe((res: any) => this.results.set(res.data || []));
  }

  ngOnDestroy() {
    this.sub?.unsubscribe();
  }
}
