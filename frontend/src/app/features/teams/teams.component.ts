import { Component, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TeamService } from '../../core/team.service';

@Component({
  selector: 'app-teams',
  standalone: true,
  imports: [RouterLink],
  template: `
    <div class="wrap">
      <div class="head">
        <div>
          <h1>Agent Teams</h1>
          <p class="muted">Describe a business outcome — we'll propose a team. Review, modify, then create.</p>
        </div>
        <a routerLink="/teams/builder" class="btn">+ New Team</a>
      </div>
      <div class="grid">
        @for (team of teams(); track team.id) {
          <a class="card" [routerLink]="['/teams', team.id]">
            <h3>{{ team.name }}</h3>
            <p class="muted">{{ team.objective }}</p>
            <div class="meta"><span class="badge">{{ team.status }}</span> {{ team.slug }}</div>
          </a>
        }
      </div>
      @if (!teams().length) {
        <div class="empty">
          <p>No teams yet.</p>
          <p class="muted">Try: "I want AI to manage our company's social media."</p>
          <a routerLink="/teams/builder" class="btn">Build your first team</a>
        </div>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:1100px; margin:0 auto; }
    .head { display:flex; justify-content:space-between; align-items:flex-start; }
    .muted { color:#6b7280; font-size:13px; }
    .btn { background:#111827; color:#fff; padding:8px 14px; border-radius:8px; text-decoration:none; font-size:13px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:12px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:16px; text-decoration:none; color:inherit; display:block; }
    .card:hover { border-color:#111827; }
    .meta { margin-top:8px; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .empty { text-align:center; padding:40px; background:#fff; border:1px solid #e8eaee; border-radius:12px; margin-top:16px; }
  `],
})
export class TeamsComponent implements OnInit {
  private svc = inject(TeamService);
  teams = this.svc.teams;
  ngOnInit(){ this.svc.list().subscribe(); }
}
