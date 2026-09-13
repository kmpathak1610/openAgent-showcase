import { Component, inject, OnInit, signal, OnDestroy } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { TeamService } from '../../../core/team.service';
import { ProjectService } from '../../../core/project.service';
import { WsService } from '../../../core/ws.service';

@Component({
  selector: 'app-team-dashboard',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      @if (team()) {
        <div class="hero">
          <h1>{{ team()!.name }}</h1>
          <p class="muted">{{ team()!.objective }}</p>
          <span class="badge">{{ team()!.status }}</span>
        </div>

        <div class="grid">
          <div class="card">
            <h3>Team Status</h3>
            <div class="row"><span>Members</span><span>{{ dashboard()?.members?.length || 0 }}</span></div>
            <div class="row"><span>Active tasks</span><span>{{ dashboard()?.activeTasks?.length || 0 }}</span></div>
            <div class="row"><span>Waiting</span><span>{{ dashboard()?.waitingTasks?.length || 0 }}</span></div>
            <div class="row"><span>Blocked</span><span class="warn">{{ dashboard()?.blockedTasks?.length || 0 }}</span></div>
            <div class="row"><span>Completed</span><span>{{ dashboard()?.completedTasks?.length || 0 }}</span></div>
          </div>

          <div class="card">
            <h3>Agents</h3>
            <div class="row"><span>Working</span><span class="badge on">{{ dashboard()?.agentsWorking || 0 }}</span></div>
            <div class="row"><span>Waiting</span><span class="badge">{{ dashboard()?.agentsWaiting || 0 }}</span></div>
            <div class="row"><span>Pending approvals</span><span class="badge warn">{{ dashboard()?.pendingApprovals || 0 }}</span></div>
            @for (m of dashboard()?.members || []; track m.agentId) {
              <div class="member"><span>{{ m.role }}: {{ m.agent?.name || m.agentId.slice(0,8) }}</span><span class="muted">{{ m.responsibilities.slice(0,40) }}</span></div>
            }
          </div>

          <div class="card">
            <h3>Start Team Task</h3>
            <input [(ngModel)]="newTaskTitle" placeholder="Task title" class="input" />
            <textarea [(ngModel)]="newTaskDesc" rows="2" placeholder="Description" class="input"></textarea>
            <select [(ngModel)]="selectedProject" class="input">
              <option value="">Select project</option>
              @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
            </select>
            <button class="btn small" (click)="startTask()" style="margin-top:6px">Start</button>
            @if (startError()) { <div class="err">{{ startError() }}</div> }
          </div>

          <div class="card">
            <h3>Recent Activity</h3>
            @for (ev of dashboard()?.recentActivity || []; track ev.id) {
              <div class="ev"><span class="time">{{ ev.createdAt }}</span><span class="type">{{ ev.eventType }}</span><span>{{ stringify(ev.payload) }}</span></div>
            }
            @if (!(dashboard()?.recentActivity?.length)) { <p class="muted">No recent activity — start a task to see orchestration.</p> }
          </div>
        </div>

        <div class="card" style="margin-top:16px">
          <h3>Active Tasks</h3>
          @for (t of dashboard()?.activeTasks || []; track t.id) {
            <div class="row"><span>{{ t.title }}</span><span class="badge">{{ t.status }}</span><span class="muted">{{ t.assignedToAgent?.slice(0,8) || 'unassigned' }}</span></div>
          }
          @if (!(dashboard()?.activeTasks?.length)) { <p class="muted">No active tasks.</p> }
          <h3 style="margin-top:12px">Blocked Tasks</h3>
          @for (t of dashboard()?.blockedTasks || []; track t.id) {
            <div class="row"><span>{{ t.title }}</span><span class="badge warn">blocked</span></div>
          }
          @if (!(dashboard()?.blockedTasks?.length)) { <p class="muted">No blocked tasks.</p> }
        </div>

        <div class="card" style="margin-top:16px">
          <h3>Delegation Hierarchy (Live)</h3>
          @for (node of teamTree(); track node.id) {
            @if (node.children.length) {
              <div class="node">
                <div class="node-head"><span>{{ node.title }}</span><span class="badge">{{ node.status }}</span><span class="muted">{{ node.children.length }} children</span></div>
                @for (child of node.children; track child.id) {
                  <div class="child"><span>↳ {{ child.title }}</span><span class="badge">{{ child.status }}</span><span class="muted">{{ child.assignedToAgent?.slice(0,8) || 'unassigned' }}</span></div>
                }
              </div>
            }
          }
          @if (!teamTree().length) { <p class="muted">No hierarchy — tasks are flat.</p> }
          <h4 style="margin-top:12px">Live Team Events</h4>
          @for (ev of liveEvents(); track ev.id) {
            <div class="ev"><span class="time">{{ ev.time }}</span><span class="type">{{ ev.type }}</span><span class="payload">{{ ev.message }}</span></div>
          }
          @if (!liveEvents().length) { <p class="muted">No live events — delegation will appear here.</p> }
        </div>
      } @else {
        <p class="muted">Loading team…</p>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:16px; max-width:1100px; margin:0 auto; }
    .hero { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; }
    .muted { color:#6b7280; font-size:12px; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .badge.on { background:#d1fae5; color:#065f46; }
    .badge.warn { background:#fee2e2; color:#dc2626; }
    .grid { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; }
    .row { display:flex; justify-content:space-between; font-size:13px; padding:4px 0; border-bottom:1px solid #f7f8f9; }
    .member { display:flex; flex-direction:column; padding:4px 0; border-bottom:1px solid #f7f8f9; }
    .input { width:100%; padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; margin-top:6px; box-sizing:border-box; }
    .btn.small { background:#111827; color:#fff; border:0; padding:6px 10px; border-radius:6px; font-size:12px; cursor:pointer; }
    .node { border:1px solid #e8eaee; border-radius:8px; padding:8px; margin:6px 0; background:#f7f8f9; }
    .node-head { display:flex; gap:6px; align-items:center; font-weight:600; font-size:13px; }
    .child { display:flex; gap:6px; align-items:center; margin-left:16px; padding:4px 0; border-bottom:1px solid #e8eaee; font-size:12px; }
    .ev { display:flex; gap:6px; font-size:11px; padding:4px 0; border-bottom:1px solid #f7f8f9; flex-wrap:wrap; }
    .time { color:#9aa0b2; }
    .type { background:#e0e7ff; padding:2px 6px; border-radius:999px; font-size:10px; }
    .err { color:#dc2626; font-size:12px; }
    @media(max-width:800px){ .grid{ grid-template-columns:1fr } }
  `],
})
export class DashboardComponent implements OnInit, OnDestroy {
  private route = inject(ActivatedRoute);
  private teamService = inject(TeamService);
  private projectService = inject(ProjectService);
  private ws = inject(WsService);
  team = this.teamService.selected;
  dashboard = this.teamService.dashboard;
  projects = this.projectService.projects;
  newTaskTitle = '';
  newTaskDesc = '';
  selectedProject = '';
  startError = signal('');
  liveEvents = signal<{id:string,type:string,message:string,time:string}[]>([]);
  private wsSub:any;

  teamTree = () => {
    const all = [...(this.dashboard()?.activeTasks||[]), ...(this.dashboard()?.waitingTasks||[]), ...(this.dashboard()?.completedTasks||[]), ...(this.dashboard()?.blockedTasks||[])];
    const map = new Map<string, any>();
    all.forEach(t => map.set(t.id, {...t, children:[]}));
    const roots:any[]=[];
    map.forEach(node => {
      if (node.parentTaskId && map.has(node.parentTaskId)) map.get(node.parentTaskId)!.children.push(node);
      else if (node.parentTaskId) roots.push(node); // orphan subtask as root if parent not in active set
      else roots.push(node);
    });
    // Only show nodes with children or that are parents
    return roots.filter(r => r.children.length || all.some(t => t.parentTaskId===r.id));
  }

  ngOnInit(){
    const id = this.route.snapshot.paramMap.get('id')!;
    this.teamService.get(id).subscribe();
    this.teamService.dashboardData(id).subscribe();
    this.projectService.list().subscribe();
    this.wsSub = this.ws.events.subscribe(ev => {
      if (ev.type.startsWith('team.') || ev.type.startsWith('agent.')) {
        const time = new Date().toLocaleTimeString();
        const msg = (ev.payload as any)?.title || (ev.payload as any)?.tool || JSON.stringify(ev.payload).slice(0,100);
        this.liveEvents.update(arr => [{id: Math.random().toString(36), type: ev.type, message: msg, time}, ...arr].slice(0,15));
        this.teamService.dashboardData(id).subscribe();
      }
    });
  }
  ngOnDestroy(){ if(this.wsSub) this.wsSub.unsubscribe(); }

  stringify(o:any){ return JSON.stringify(o); }

  startTask(){
    const id = this.route.snapshot.paramMap.get('id')!;
    if (!this.newTaskTitle.trim() || !this.selectedProject) { this.startError.set('Title and project required'); return; }
    this.teamService.startTask(id, this.selectedProject, this.newTaskTitle, this.newTaskDesc).subscribe({
      next: ()=> { this.newTaskTitle=''; this.newTaskDesc=''; this.teamService.dashboardData(id).subscribe(); },
      error: e=> this.startError.set(e.error?.error?.message || 'Failed')
    });
  }
}
