import { Component, inject, OnInit, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-runs',
  standalone: true,
  imports: [RouterLink, FormsModule],
  template: `
    <div class="wrap">
      <h1>Agent Runs — Debugger</h1>
      <p class="muted">Safe execution telemetry — no chain-of-thought. Shows timeline, tool calls, delegations, approvals, token usage.</p>
      <div class="filters">
        <input [(ngModel)]="filterAgent" placeholder="Filter by agent ID" class="input" />
        <button class="btn small" (click)="load()">Refresh</button>
      </div>
      <div class="list">
        @for (run of runs(); track run.id) {
          <a [routerLink]="['/runs', run.id]" class="card">
            <div class="head"><strong>Run {{ run.id.slice(0,8) }}</strong> <span class="badge" [class]="run.status">{{ run.status }}</span></div>
            <div class="muted">Agent {{ run.agentId?.slice(0,8) }} • Task {{ run.taskId?.slice(0,8) }} • {{ run.triggerType }} • Iter {{ run.currentIteration }} • {{ run.createdAt }}</div>
            @if (run.tokenMetadata) {
              <div class="meta">Tokens {{ run.tokenMetadata.totalTokens || 0 }} • Cost \${{ (run.tokenMetadata.estimatedCost || 0).toFixed(4) }}</div>
            }
            @if (run.waitingReason) { <div class="waiting">Waiting: {{ run.waitingReason }}</div> }
          </a>
        }
        @if (!runs().length) { <p class="muted">No runs yet. Trigger an agent via tasks.</p> }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .filters { display:flex; gap:8px; margin:12px 0; }
    .input { padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .btn.small { background:#111827; color:#fff; border:0; padding:6px 10px; border-radius:6px; font-size:12px; cursor:pointer; }
    .card { display:block; background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; margin:8px 0; text-decoration:none; color:inherit; }
    .head { display:flex; justify-content:space-between; }
    .badge { padding:2px 6px; border-radius:999px; font-size:10px; background:#e8eaee; }
    .badge.running { background:#dbeafe; color:#1e40af; }
    .badge.awaiting_approval, .badge.WAITING_FOR_APPROVAL { background:#fef3c7; color:#92400e; }
    .badge.succeeded, .badge.completed { background:#d1fae5; color:#065f46; }
    .badge.failed { background:#fee2e2; color:#dc2626; }
    .meta { font-size:11px; color:#6b7280; margin-top:4px; }
    .waiting { font-size:11px; color:#92400e; background:#fef3c7; padding:4px 6px; border-radius:6px; margin-top:4px; }
  `],
})
export class RunsComponent implements OnInit {
  filterAgent = '';
  runs = signal<any[]>([]);
  ngOnInit() { this.load(); }
  load() {
    // In real app, call /api/v1/runs?agentId=...
    // For now, placeholder that would be wired to ApiService
    this.runs.set([]);
  }
}
