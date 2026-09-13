import { Component, inject, OnInit } from '@angular/core';
import { SettingsService } from '../../core/settings.service';

@Component({
  selector: 'app-settings',
  standalone: true,
  template: `
    <div class="wrap">
      <h2>Settings — LLM</h2>
      <p class="muted">Read-only in v1 (configured via .env / compose). Default applies to new agents.</p>
      @if (svc.loading()) { <p class="muted">Loading…</p> }
      @if (svc.config(); as c) {
        <div class="card">
          <h3>Default model</h3>
          <code>{{ c.defaultModel }}</code>
        </div>
        <div class="card">
          <h3>Providers (keys never shown)</h3>
          @for (p of c.providers; track p.name) {
            <div class="row"><span>{{ p.name }}</span><span class="badge" [class.on]="p.real">{{ p.real ? 'real' : 'stub' }}</span></div>
          }
        </div>
        <div class="card">
          <h3>Available models</h3>
          @for (m of c.availableModels; track m) { <div class="row"><code>{{ m }}</code></div> }
          <p class="muted small">Agents save model per version. Existing agents keep their version until you Save (new version) with a new model.</p>
        </div>
      }
    </div>
  `,
  styles: [`
    .wrap{padding:24px;max-width:900px}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:12px;margin-top:10px}
    .row{display:flex;justify-content:space-between;font-size:13px;padding:4px 0}
    .muted{color:#6b7280}.small{font-size:11px}
    .badge{background:#e8eaee;padding:2px 8px;border-radius:999px;font-size:11px}
    .badge.on{background:#d1fae5;color:#065f46}
  `],
})
export class SettingsComponent implements OnInit {
  svc = inject(SettingsService);
  ngOnInit() { this.svc.llm().subscribe(); }
}
