import { Component, inject, OnInit, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';

@Component({
  selector: 'app-run-detail',
  standalone: true,
  template: `
    <div class="wrap">
      <h1>Run {{ runId }}</h1>
      <p class="muted">Timeline — safe telemetry (no chain-of-thought). Shows context, LLM, tool, delegation, approval, resume.</p>
      <div class="timeline">
        @for (ev of timeline(); track ev.seq) {
          <div class="event" [class]="ev.status">
            <span class="seq">{{ ev.seq }}</span>
            <span class="type">{{ ev.actionType }}</span>
            <span class="tool">{{ ev.toolName || '' }}</span>
            <span class="status">{{ ev.status }}</span>
            <span class="time">{{ ev.createdAt }}</span>
          </div>
        }
        @if (!timeline().length) { <p class="muted">No actions yet. Run will populate as agent executes.</p> }
      </div>
      <div class="meta">
        <h3>Token Usage</h3>
        <p class="muted">Prompt + completion tokens and estimated cost are tracked per run and shown in task detail as well.</p>
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .timeline { margin-top:16px; }
    .event { display:flex; gap:8px; padding:8px; border:1px solid #e8eaee; border-radius:8px; margin:6px 0; font-size:12px; }
    .seq { background:#111827; color:#fff; padding:2px 6px; border-radius:999px; font-size:10px; }
    .type { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .tool { color:#6b7280; }
    .status { margin-left:auto; font-size:10px; }
  `],
})
export class RunDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);
  runId = this.route.snapshot.paramMap.get('id') || '';
  timeline = signal<any[]>([]);
  ngOnInit() {
    // Would fetch /api/v1/runs/:id and display actions
  }
}
