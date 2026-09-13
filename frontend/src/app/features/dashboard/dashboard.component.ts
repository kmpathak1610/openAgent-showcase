import { Component, inject, OnInit, signal } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { ProjectService } from '../../core/project.service';
import { ChannelService } from '../../core/channel.service';
import { AuthService } from '../../core/auth.service';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [RouterLink],
  template: `
    <div class="dash">
      <h1>Welcome, {{ auth.user()?.displayName || 'there' }} 👋</h1>
      <p class="muted">Workspace: <strong>{{ auth.org()?.name }}</strong> • {{ auth.org()?.slug }}</p>

      <div class="cards">
        <div class="card">
          <h3>Create an agent</h3>
          <p>Describe what you need: <em>"I need an agent that manages our social media"</em></p>
          <input [value]="intent()" (input)="intent.set($any($event.target).value)" placeholder="Describe your agent…" class="input" />
          <button class="btn" (click)="create()">Create agent →</button>
          <p class="muted small">Agents are first-class members — they'll appear alongside humans in channels and tasks.</p>
        </div>
        <div class="card">
          <h3>Quick actions</h3>
          <a routerLink="/projects" class="action">→ Browse projects ({{ projects().length }})</a>
          <a routerLink="/channels" class="action">→ Browse channels ({{ channels().length }})</a>
          <p class="muted small">Tip: channels can be org-wide or tied to a project. Private channels enforce membership.</p>
        </div>
      </div>

      <div class="sections">
        <div class="section">
          <h3>Recent projects</h3>
          @if (projects().length) {
            @for (p of projects().slice(0,3); track p.id) {
              <a [routerLink]="['/projects', p.id]" class="row"><span class="icon">{{ p.icon }}</span> {{ p.name }}<span class="meta">{{ p.objective }}</span></a>
            }
          } @else { <p class="muted">No projects yet — create one from the sidebar.</p> }
        </div>
        <div class="section">
          <h3>Channels</h3>
          @if (channels().length) {
            @for (c of channels().slice(0,5); track c.id) {
              <a [routerLink]="['/channels', c.id]" class="row"># {{ c.name }} <span class="meta">{{ c.topic || c.description }}</span></a>
            }
          } @else { <p class="muted">No channels yet — create one to start chatting.</p> }
        </div>
      </div>
    </div>
  `,
  styles: [`
    .dash { padding:24px; }
    h1 { font-size:22px; margin:0 0 4px; }
    .muted { color:#6b7280; font-size:13px; }
    .muted.small { font-size:11px; margin-top:8px; }
    .cards { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; margin:8px 0; font-size:13px; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .action { display:block; padding:6px 0; color:#111827; text-decoration:none; font-size:13px; }
    .action:hover { text-decoration:underline; }
    .sections { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .section { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .row { display:flex; align-items:center; gap:8px; padding:6px 0; text-decoration:none; color:inherit; font-size:13px; border-bottom:1px solid #f7f8f9; }
    .icon { font-size:14px; }
    .meta { margin-left:auto; color:#9aa0b2; font-size:11px; max-width:140px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    @media(max-width:800px){ .cards,.sections{grid-template-columns:1fr} }
  `],
})
export class DashboardComponent implements OnInit {
  auth = inject(AuthService);
  private router = inject(Router);
  private projectService = inject(ProjectService);
  private channelService = inject(ChannelService);
  projects = this.projectService.projects;
  channels = this.channelService.channels;
  intent = signal('');

  ngOnInit() {
    this.projectService.list().subscribe();
    this.channelService.list().subscribe();
  }

  create() {
    const v = this.intent().trim();
    if (!v) { this.router.navigate(['/agents', 'builder']); return; }
    // Store intent for builder to pick up via query param or state
    this.router.navigate(['/agents', 'builder'], { queryParams: { intent: v } });
  }
}
