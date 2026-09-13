import { Component, inject, OnInit, signal } from '@angular/core';
import { ApprovalService } from '../../core/approval.service';

@Component({
  selector: 'app-approvals',
  standalone: true,
  template: `
    <div class="wrap">
      <h1>Approvals</h1>
      <p class="muted">Human-in-the-loop for high-risk actions. External side effects default to approval.</p>

      <div class="tabs">
        <button [class.active]="filter==='pending'" (click)="load('pending')">Pending ({{ pending().length }})</button>
        <button [class.active]="filter==='approved'" (click)="load('approved')">Approved</button>
        <button [class.active]="filter==='rejected'" (click)="load('rejected')">Rejected</button>
        <button [class.active]="filter===''" (click)="load('')">All</button>
      </div>

      @for (ap of approvals(); track ap.id) {
        <div class="card">
          <div class="head">
            <strong>{{ ap.title }}</strong>
            <span class="risk {{ ap.riskLevel }}">{{ ap.riskLevel }}</span>
            <span class="badge">{{ ap.status }}</span>
          </div>
          <p class="desc">{{ ap.description }}</p>
          <div class="meta">
            <span>Action: {{ ap.action }} → {{ ap.target }}</span>
            <span>By: {{ ap.requesterType }} • {{ ap.createdAt }}</span>
          </div>
          <details class="payload">
            <summary>Inspect intended action</summary>
            <pre class="code">{{ stringify(ap.payload) }}</pre>
            <pre class="code">Context: {{ stringify(ap.payload) }}</pre>
          </details>

          @if (ap.status==='pending') {
            <div class="actions">
              <button class="btn" (click)="approve(ap.id)">✓ Approve</button>
              <button class="btn secondary" (click)="reject(ap.id)">✕ Reject</button>
              <button class="btn secondary" (click)="cancel(ap.id)">Cancel</button>
            </div>
            <p class="muted small">Agent wants to publish: "5 LinkedIn posts for September campaign" — review before approving.</p>
          } @else {
            <p class="muted small">Decided • {{ ap.status }}</p>
          }
        </div>
      }

      @if (!approvals().length) {
        <div class="empty">
          <p>No approvals in this filter.</p>
          <p class="muted">Try creating a high-risk tool call: publish_social_post with LinkedIn will require approval.</p>
        </div>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .muted.small { font-size:11px; }
    .tabs { display:flex; gap:6px; margin:12px 0; }
    .tabs button { padding:6px 10px; border:1px solid #e8eaee; background:#fff; border-radius:999px; font-size:12px; cursor:pointer; }
    .tabs button.active { background:#111827; color:#fff; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; margin:10px 0; }
    .head { display:flex; gap:6px; align-items:center; }
    .risk { padding:2px 6px; border-radius:999px; font-size:10px; text-transform:capitalize; }
    .risk.low { background:#d1fae5; }
    .risk.medium { background:#fef3c7; }
    .risk.high { background:#fee2e2; color:#dc2626; }
    .risk.critical { background:#dc2626; color:#fff; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .desc { margin:6px 0; font-size:13px; }
    .meta { display:flex; justify-content:space-between; font-size:11px; color:#6b7280; }
    .payload { margin-top:8px; }
    .code { background:#f7f8f9; padding:8px; border-radius:6px; font-size:11px; white-space:pre-wrap; }
    .actions { display:flex; gap:6px; margin-top:10px; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .empty { text-align:center; padding:20px; background:#fff; border:1px solid #e8eaee; border-radius:10px; margin-top:12px; }
  `],
})
export class ApprovalsComponent implements OnInit {
  private svc = inject(ApprovalService);
  approvals = this.svc.approvals;
  pending = this.svc.pending;
  filter = 'pending';

  ngOnInit(){ this.load('pending'); }

  load(status:string){
    this.filter = status;
    this.svc.list(status).subscribe();
  }

  stringify(o:any){ return JSON.stringify(o, null, 2); }

  approve(id:string){ this.svc.approve(id).subscribe(); }
  reject(id:string){ this.svc.reject(id, 'Rejected via UI').subscribe(); }
  cancel(id:string){ this.svc.cancel(id).subscribe(); }
}
