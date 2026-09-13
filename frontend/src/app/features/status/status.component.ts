import { Component, inject, OnInit, signal } from '@angular/core';
import { StatusService } from '../../core/status.service';

@Component({
  selector: 'app-status',
  standalone: true,
  template: `
    <div class="wrap">
      <h1>Agent Status</h1>
      <p class="muted">Useful states: idle, working, waiting, blocked, approval_required, failed, offline — not internal reasoning.</p>
      <button class="btn small" (click)="reload()">Refresh</button>
      <div class="grid">
        @for (s of statuses(); track s.agentId) {
          <div class="card" [class]="s.status">
            <div class="head"><strong>{{ s.name }}</strong> <span class="badge">{{ s.status }}</span></div>
            <div class="muted">Autonomy: {{ s.autonomy }} • {{ s.agentId.slice(0,8) }}</div>
            <div class="muted small">Guardrails: max depth 5, retries 3, timeout 300s, rate 60/min</div>
          </div>
        }
      </div>
      @if (!statuses().length) { <p class="muted">No agents.</p> }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(240px,1fr)); gap:12px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .card.working { border-color:#3b82f6; }
    .card.waiting { border-color:#f59e0b; }
    .card.blocked { border-color:#ef4444; }
    .card.approval_required { border-color:#8b5cf6; }
    .head { display:flex; justify-content:space-between; align-items:center; }
    .badge { padding:2px 6px; border-radius:999px; font-size:10px; background:#e8eaee; }
    .btn.small { background:#111827; color:#fff; border:0; padding:6px 10px; border-radius:6px; font-size:12px; cursor:pointer; margin-top:8px; }
  `],
})
export class StatusComponent implements OnInit {
  private svc = inject(StatusService);
  statuses = this.svc.statuses;
  ngOnInit(){ this.reload(); }
  reload(){ this.svc.list(); }
}
