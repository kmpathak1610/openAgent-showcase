import { Component, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AgentService } from '../../core/agent.service';

@Component({
  selector: 'app-agents',
  standalone: true,
  imports: [RouterLink],
  template: `
    <div class="wrap">
      <div class="head">
        <div>
          <h2>Agents</h2>
          <p class="muted">Agents are first-class workspace members — not chatbots attached to channels.</p>
        </div>
        <a routerLink="/agents/builder" class="btn">+ New Agent</a>
      </div>

      @if (agentService.loading()) { <p class="muted">Loading…</p> }

      <div class="cards">
        @for (a of agentService.agents(); track a.id) {
          <a class="card" [routerLink]="['/agents', a.id]">
            <div class="avatar">{{ a.avatar || '🤖' }}</div>
            <div class="info">
              <b>{{ a.name }}</b>
              <div class="muted">{{ a.purpose || a.description }}</div>
              <div class="meta"><span class="badge">{{ a.autonomyLevel }}</span> <span class="badge">{{ a.status }}</span> v{{ a.currentVersion }}</div>
            </div>
          </a>
        }
      </div>

      @if (!agentService.agents().length && !agentService.loading()) {
        <div class="empty">
          <p>No agents yet.</p>
          <p class="muted">Try: “I need an agent that manages our company's social media accounts, creates content, analyzes performance and coordinates with other agents.”</p>
          <a routerLink="/agents/builder" class="btn">Build your first agent</a>
        </div>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; }
    .head { display:flex; justify-content:space-between; align-items:flex-start; }
    h2 { margin:0; }
    .muted{color:#6b7280; font-size:13px; }
    .btn{ background:#111827; color:#fff; padding:8px 14px; border-radius:8px; text-decoration:none; font-size:13px; }
    .cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:12px;margin-top:16px}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:16px;display:flex;gap:12px;text-decoration:none;color:inherit}
    .card:hover{ border-color:#111827 }
    .avatar{font-size:24px; width:40px; height:40px; display:grid; place-items:center; background:#f7f8f9; border-radius:8px; }
    .meta{ display:flex; gap:6px; margin-top:6px; }
    .badge{ background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .empty{ text-align:center; padding:40px; background:#fff; border:1px solid #e8eaee; border-radius:12px; margin-top:16px; }
  `],
})
export class AgentsComponent implements OnInit {
  agentService = inject(AgentService);
  ngOnInit(){ this.agentService.list().subscribe(); }
}
