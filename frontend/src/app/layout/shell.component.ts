import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet, Router } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { WsService } from '../core/ws.service';
import { MessageService } from '../core/message.service';
import { ProjectService } from '../core/project.service';
import { ChannelService } from '../core/channel.service';
import { TaskService } from '../core/task.service';
import { ApprovalService } from '../core/approval.service';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, FormsModule],
  template: `
    <div class="shell">
      <!-- Mobile topbar -->
      <div class="mobile-top">
        <button class="icon-btn" (click)="showSidebar.set(!showSidebar())">☰</button>
        <span class="mobile-brand">OpenAgent</span>
        <span class="mobile-org">{{ auth.org()?.name || 'Workspace' }}</span>
      </div>

      <!-- Left Sidebar -->
      <nav class="sidebar" [class.open]="showSidebar()">
        <div class="org-switcher">
          <div class="org-name">{{ auth.org()?.name || 'Select Workspace' }}</div>
          <div class="org-slug">{{ auth.org()?.slug || '' }}</div>
          @if (auth.orgs().length > 1) {
            <select class="org-select" [value]="auth.org()?.id" (change)="switchOrg($any($event.target).value)">
              @for (o of auth.orgs(); track o.id) {
                <option [value]="o.id" [selected]="o.id===auth.org()?.id">{{ o.name }}</option>
              }
            </select>
          }
        </div>

        <!-- Projects -->
        <div class="section">
          <div class="section-head">
            <span>Projects</span>
            <button class="mini-btn" (click)="showProjectCreate.set(!showProjectCreate())">＋</button>
          </div>
          @if (showProjectCreate()) {
            <div class="inline-create">
              <input [(ngModel)]="newProjectName" placeholder="Project name" class="mini-input" />
              <input [(ngModel)]="newProjectObjective" placeholder="Objective (optional)" class="mini-input" />
              <button class="btn small" (click)="createProject()">Create</button>
            </div>
          }
          @for (p of projectService.projects(); track p.id) {
            <a class="nav-item" [routerLink]="['/projects', p.id]" routerLinkActive="active" (click)="closeMobile()">
              <span class="icon">{{ p.icon || '📁' }}</span> {{ p.name }}
            </a>
          }
          @if (!projectService.projects().length && !projectService.loading()) {
            <div class="empty">No projects</div>
          }
        </div>

        <!-- Channels -->
        <div class="section">
          <div class="section-head">
            <span>Channels</span>
            <button class="mini-btn" (click)="showChannelCreate.set(!showChannelCreate())">＋</button>
          </div>
          @if (showChannelCreate()) {
            <div class="inline-create">
              <input [(ngModel)]="newChannelName" placeholder="channel-name (lowercase)" class="mini-input" />
              <select [(ngModel)]="newChannelProjectId" class="mini-input">
                <option value="">Organization channel</option>
                @for (p of projectService.projects(); track p.id) {
                  <option [value]="p.id">{{ p.name }}</option>
                }
              </select>
              <button class="btn small" (click)="createChannel()">Create</button>
            </div>
          }
          @for (c of channelService.channels(); track c.id) {
            <a class="nav-item channel" 
               [class.active]="channelService.selectedId()===c.id"
               (click)="openChannel(c.id)">
              <span class="hash">#</span> {{ c.name }}
              <span class="ch-type">{{ c.projectId ? '•' : '○' }}</span>
            </a>
          }
          @if (!channelService.channels().length && !channelService.loading()) {
            <div class="empty">No channels — create one</div>
          }
        </div>

        <!-- Agents -->
        <div class="section agents">
          <div class="section-head"><span>Agents</span><a routerLink="/agents" class="mini-btn">Manage</a></div>
          <a routerLink="/agents/builder" class="nav-item" (click)="closeMobile()">＋ New Agent</a>
          <a routerLink="/agents" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🤖 All Agents</a>
        </div>

        <!-- Knowledge -->
        <div class="section">
          <div class="section-head"><span>Knowledge</span><a routerLink="/knowledge" class="mini-btn">Manage</a></div>
          <a routerLink="/knowledge" routerLinkActive="active" class="nav-item" (click)="closeMobile()">📚 Knowledge Base</a>
        </div>

        <!-- Articles -->
        <div class="section">
          <div class="section-head"><span>Articles</span><a routerLink="/articles" class="mini-btn">Manage</a></div>
          <a routerLink="/articles" routerLinkActive="active" class="nav-item" (click)="closeMobile()">✍️ Articles</a>
        </div>

        <!-- Teams -->
        <div class="section">
          <div class="section-head"><span>Teams</span><a routerLink="/teams" class="mini-btn">Manage</a></div>
          <a routerLink="/teams/builder" class="nav-item" (click)="closeMobile()">＋ New Team</a>
          <a routerLink="/teams" routerLinkActive="active" class="nav-item" (click)="closeMobile()">👥 All Teams</a>
        </div>

        <!-- Tools & Integrations -->
        <div class="section">
          <div class="section-head"><span>Tools</span></div>
          <a routerLink="/tools" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🛠️ Tools</a>
          <a routerLink="/integrations" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🔌 Integrations</a>
          <a routerLink="/approvals" routerLinkActive="active" class="nav-item" (click)="closeMobile()">✅ Approvals @if (pendingApprovals()) { <span class="badge">{{ pendingApprovals() }}</span> }</a>
        </div>

        <!-- Browser & Assistant -->
        <div class="section">
          <div class="section-head"><span>Browser & Assistant</span></div>
          <a routerLink="/browser" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🌐 Browser</a>
          <a routerLink="/assistant" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🤖 Assistant</a>
        </div>

        <!-- Phase 7: Memory & Autonomy -->
        <div class="section">
          <div class="section-head"><span>Memory & Autonomy</span></div>
          <a routerLink="/memories" routerLinkActive="active" class="nav-item" (click)="closeMobile()">🧠 Memories</a>
          <a routerLink="/schedules" routerLinkActive="active" class="nav-item" (click)="closeMobile()">⏰ Schedules</a>
          <a routerLink="/status" routerLinkActive="active" class="nav-item" (click)="closeMobile()">📊 Agent Status</a>
          <a routerLink="/settings" routerLinkActive="active" class="nav-item" (click)="closeMobile()">⚙️ Settings</a>
        </div>

        <div class="spacer"></div>

        <div class="sidebar-footer">
          <div class="presence" [class.online]="ws.connected()">{{ ws.connected() ? '● Live' : '○ Offline' }}</div>
          <div class="user-row">
            <div class="avatar">{{ (auth.user()?.displayName||auth.user()?.email||'U').slice(0,1).toUpperCase() }}</div>
            <div class="user-meta">
              <div class="user-name">{{ auth.user()?.displayName || auth.user()?.email }}</div>
              <div class="user-email">{{ auth.user()?.email }}</div>
            </div>
            <button class="icon-btn" (click)="logout()" title="Logout">⎋</button>
          </div>
        </div>
      </nav>

      <!-- Center -->
      <main class="main">
        <header class="topbar">
          <div class="top-left">
            <h2 class="top-title">{{ topTitle() }}</h2>
            @if (currentChannelTopic()) {
              <span class="top-topic">{{ currentChannelTopic() }}</span>
            }
          </div>
          <div class="top-right">
            <input class="search" placeholder="Search or ask an agent… (@mentions supported)" />
            <button class="icon-btn" (click)="toggleRight()" title="Details">⧉</button>
          </div>
        </header>
        <section class="content">
          <router-outlet />
        </section>
      </main>

      <!-- Right drawer -->
      @if (showRight()) {
        <aside class="right">
          <div class="right-head">
            <strong>Details</strong>
            <button class="icon-btn" (click)="showRight.set(false)">✕</button>
          </div>
          <div class="right-body">
            @if (threadMessageId()) {
              <h4>Thread</h4>
              <p class="muted">Replies for {{ threadMessageId()?.slice(0,8) }}</p>
              <!-- Thread view is handled inside channel component; this is placeholder -->
              <button class="btn small" (click)="threadMessageId.set(null)">Close thread</button>
            } @else {
              <h4>Workspace</h4>
              <p class="muted">Humans + AI agents collaborating. Channels can be project-scoped or org-wide. Private channels enforce membership.</p>
              <hr />
              <h4>Project Activity</h4>
              <p class="muted">Select a project to see members, channels, and tasks.</p>
              <hr />
              <h4>Knowledge (soon)</h4>
              <p class="muted">Documents & RAG will appear here.</p>
            }
          </div>
        </aside>
      }
    </div>

    @if (showSidebar()) {
      <div class="backdrop" (click)="showSidebar.set(false)"></div>
    }
  `,
  styles: [`
    .shell { display:flex; min-height:100vh; font-family: Inter, system-ui, sans-serif; background:#f7f8f9; }
    .sidebar { width:280px; background:#0f1117; color:#e6e8ee; display:flex; flex-direction:column; padding:14px; gap:10px; overflow-y:auto; }
    .mobile-top { display:none; }
    .org-switcher { background:#1c1f2a; border-radius:10px; padding:10px; }
    .org-name { font-weight:700; font-size:14px; }
    .org-slug { font-size:11px; color:#9aa0b2; }
    .org-select { width:100%; margin-top:6px; background:#0f1117; color:#fff; border:1px solid #2a2f41; border-radius:6px; padding:4px; font-size:12px; }
    .section { display:flex; flex-direction:column; gap:4px; }
    .section-head { display:flex; align-items:center; justify-content:space-between; font-size:11px; text-transform:uppercase; letter-spacing:0.06em; color:#9aa0b2; margin-top:8px; }
    .mini-btn { background:transparent; color:#9aa0b2; border:1px solid #2a2f41; border-radius:6px; padding:2px 6px; cursor:pointer; font-size:12px; }
    .mini-btn:hover { background:#1c1f2a; color:#fff; }
    .inline-create { background:#1c1f2a; border-radius:8px; padding:8px; display:flex; flex-direction:column; gap:6px; }
    .mini-input { background:#0f1117; color:#fff; border:1px solid #2a2f41; border-radius:6px; padding:6px 8px; font-size:12px; }
    .btn.small { background:#fff; color:#0f1117; border:0; padding:6px 10px; border-radius:6px; cursor:pointer; font-size:12px; font-weight:600; }
    .nav-item { color:#c2c7d6; text-decoration:none; padding:7px 8px; border-radius:8px; font-size:13px; display:flex; align-items:center; gap:8px; cursor:pointer; }
    .nav-item:hover { background:#1c1f2a; color:#fff; }
    .nav-item.active { background:#1c1f2a; color:#fff; }
    .nav-item.channel.active { background:#2a2f41; }
    .hash { color:#7a8191; font-weight:700; }
    .ch-type { margin-left:auto; font-size:10px; color:#7a8191; }
    .empty { font-size:11px; color:#7a8191; padding:6px 8px; }
    .agents .badge { background:#2a2f41; color:#9aa0b2; font-size:10px; padding:2px 6px; border-radius:999px; }
    .spacer { flex:1; }
    .sidebar-footer { border-top:1px solid #1c1f2a; padding-top:10px; display:flex; flex-direction:column; gap:8px; }
    .presence { font-size:11px; color:#7a8191; }
    .presence.online { color:#3dd68c; }
    .user-row { display:flex; align-items:center; gap:8px; }
    .avatar { width:32px; height:32px; border-radius:50%; background:#2a2f41; display:grid; place-items:center; font-weight:700; font-size:13px; }
    .user-meta { flex:1; overflow:hidden; }
    .user-name { font-size:12px; font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .user-email { font-size:11px; color:#9aa0b2; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .icon-btn { background:transparent; border:0; color:#9aa0b2; cursor:pointer; padding:4px 6px; border-radius:6px; }
    .icon-btn:hover { background:#1c1f2a; color:#fff; }

    .main { flex:1; display:flex; flex-direction:column; min-width:0; }
    .topbar { height:56px; background:#fff; border-bottom:1px solid #e8eaee; display:flex; align-items:center; padding:0 16px; gap:12px; }
    .top-left { flex:1; min-width:0; display:flex; align-items:center; gap:12px; }
    .top-title { font-size:15px; font-weight:700; margin:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .top-topic { font-size:12px; color:#6b7280; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .top-right { display:flex; align-items:center; gap:8px; }
    .search { width:320px; max-width:40vw; padding:8px 12px; border:1px solid #e8eaee; border-radius:999px; font-size:13px; background:#f7f8f9; }
    .content { flex:1; overflow:auto; padding:0; background:#fff; }

    .right { width:340px; background:#f7f8f9; border-left:1px solid #e8eaee; display:flex; flex-direction:column; }
    .right-head { height:56px; display:flex; align-items:center; justify-content:space-between; padding:0 12px; border-bottom:1px solid #e8eaee; background:#fff; }
    .right-body { padding:16px; overflow:auto; font-size:13px; }
    .muted { color:#6b7280; }

    .backdrop { display:none; }

    @media (max-width: 900px) {
      .shell { flex-direction:column; }
      .sidebar { position:fixed; inset:0 auto 0 0; z-index:20; transform:translateX(-100%); transition:transform .2s; }
      .sidebar.open { transform:translateX(0); }
      .mobile-top { display:flex; align-items:center; gap:10px; padding:10px 12px; background:#0f1117; color:#fff; }
      .mobile-brand { font-weight:800; }
      .mobile-org { margin-left:auto; font-size:12px; color:#9aa0b2; }
      .search { width:160px; }
      .right { position:fixed; inset:0 0 0 auto; z-index:15; width:86%; max-width:360px; }
      .backdrop { display:block; position:fixed; inset:0; background:rgba(0,0,0,.4); z-index:10; }
    }
  `],
})
export class ShellComponent implements OnInit {
  auth = inject(AuthService);
  ws = inject(WsService);
  msgService = inject(MessageService);
  taskService = inject(TaskService);
  projectService = inject(ProjectService);
  channelService = inject(ChannelService);
  approvalService = inject(ApprovalService);
  pendingApprovals = computed(() => this.approvalService.pending().length);
  private router = inject(Router);

  showSidebar = signal(false);
  showRight = signal(false);
  showProjectCreate = signal(false);
  showChannelCreate = signal(false);
  newProjectName = '';
  newProjectObjective = '';
  newChannelName = '';
  newChannelProjectId = '';
  threadMessageId = signal<string | null>(null);

  topTitle = computed(() => {
    const ch = this.channelService.selected();
    if (ch) return `# ${ch.name}`;
    const proj = this.projectService.selected();
    if (proj) return proj.name;
    return this.auth.org()?.name || 'Workspace';
  });
  currentChannelTopic = computed(() => this.channelService.selected()?.topic || '');

  ngOnInit() {
    this.projectService.list().subscribe();
    this.channelService.list().subscribe();
    this.approvalService.list('pending').subscribe();
    this.ws.connect();
    // forward WS events to message and task services
    this.ws.events.subscribe(ev => {
      this.msgService.handleIncoming(ev as any);
      this.taskService.handleIncoming(ev as any);
      if (ev.type === 'approval.decided' || ev.type === 'tool.approved') {
        this.approvalService.list('pending').subscribe();
      }
    });
    // also listen for org-level events like channel created to refresh list
    this.ws.events.subscribe(ev => {
      if (ev.type === 'channel.created') this.channelService.list().subscribe();
    });
  }

  switchOrg(orgId: string) {
    this.auth.switchOrg(orgId).subscribe(() => {
      this.projectService.list().subscribe();
      this.channelService.list().subscribe();
    });
  }

  createProject() {
    if (!this.newProjectName.trim()) return;
    this.projectService
      .create({ name: this.newProjectName.trim(), objective: this.newProjectObjective.trim() })
      .subscribe(() => {
        this.newProjectName = '';
        this.newProjectObjective = '';
        this.showProjectCreate.set(false);
      });
  }

  createChannel() {
    const name = this.newChannelName.trim().toLowerCase();
    if (!name) return;
    this.channelService
      .create({
        name,
        displayName: name,
        projectId: this.newChannelProjectId || undefined,
        channelType: this.newChannelProjectId ? 'standard' : 'standard',
      })
      .subscribe(() => {
        this.newChannelName = '';
        this.newChannelProjectId = '';
        this.showChannelCreate.set(false);
      });
  }

  openChannel(id: string) {
    this.channelService.select(id);
    this.router.navigate(['/channels', id]);
    this.closeMobile();
  }

  toggleRight() {
    this.showRight.update(v => !v);
  }

  closeMobile() {
    this.showSidebar.set(false);
  }

  logout() {
    this.ws.disconnect();
    this.auth.logout();
  }
}
