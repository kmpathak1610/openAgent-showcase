import { Component, inject, OnInit, signal, computed } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { TaskService } from '../../core/task.service';
import { ProjectService } from '../../core/project.service';
import { AgentService } from '../../core/agent.service';
import { ChannelService } from '../../core/channel.service';
import { WsService } from '../../core/ws.service';

@Component({
  selector: 'app-tasks',
  standalone: true,
  imports: [FormsModule, RouterLink],
  template: `
    <div class="wrap">
      <div class="head">
        <h2>Tasks</h2>
        <button class="btn" (click)="showCreate.set(!showCreate())">+ New Task</button>
      </div>

      @if (showCreate()) {
        <div class="create">
          <input [(ngModel)]="title" placeholder="Title — e.g., Create September social campaign" class="input" />
          <textarea [(ngModel)]="description" rows="2" placeholder="Description" class="input"></textarea>
          <div class="row">
            <select [(ngModel)]="projectId" class="input">
              <option value="">Select project</option>
              @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
            </select>
            <select [(ngModel)]="channelId" class="input">
              <option value="">No channel</option>
              @for (c of channels(); track c.id) { <option [value]="c.id">#{{ c.name }}</option> }
            </select>
          </div>
          <div class="row">
            <select [(ngModel)]="assigneeAgent" class="input">
              <option value="">Assign to — none (pending)</option>
              @for (a of agents(); track a.id) { <option [value]="a.id">🤖 {{ a.name }}</option> }
            </select>
            <select [(ngModel)]="priority" class="input">
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="urgent">Urgent</option>
            </select>
          </div>
          <div class="row">
            <button class="btn" (click)="create()" [disabled]="creating()"> {{ creating() ? 'Creating…' : 'Create & Assign' }}</button>
            <button class="btn secondary" (click)="showCreate.set(false)">Cancel</button>
          </div>
          @if (createError()) { <div class="err">{{ createError() }}</div> }
        </div>
      }

      <div class="filters">
        <select [(ngModel)]="filterProject" (change)="reload()" class="input small">
          <option value="">All projects</option>
          @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
        </select>
        <select [(ngModel)]="filterStatus" (change)="reload()" class="input small">
          <option value="">All statuses</option>
          <option value="pending">pending</option>
          <option value="assigned">assigned</option>
          <option value="running">running</option>
          <option value="waiting">waiting</option>
          <option value="blocked">blocked</option>
          <option value="approval_required">approval_required</option>
          <option value="completed">completed</option>
          <option value="failed">failed</option>
          <option value="cancelled">cancelled</option>
        </select>
      </div>

      <div class="board">
        @for (col of columns; track col.key) {
          <div class="col">
            <h4>{{ col.label }} <span class="count">{{ countBy(col.key) }}</span></h4>
            @for (t of byStatus(col.key); track t.id) {
              <a class="card" [routerLink]="['/tasks', t.id]">
                <div class="title">{{ t.title }}</div>
                <div class="muted small">{{ t.description.slice(0,80) }}</div>
                <div class="meta">
                  <span class="prio" [class]="t.priority">{{ t.priority }}</span>
                  @if (t.assignedToAgent) { <span class="assignee">🤖 {{ agentName(t.assignedToAgent) }}</span> }
                  @if (t.parentTaskId) { <span class="sub">↳ subtask</span> }
                </div>
              </a>
            }
            @if (!byStatus(col.key).length) { <div class="muted small">No tasks</div> }
          </div>
        }
      </div>

      <div class="hierarchy">
        <h3>Hierarchy (Parent → Subtasks)</h3>
        @for (node of tree(); track node.id) {
          @if (node.children.length) {
            <div class="node">
              <div class="node-head"><span class="title">{{ node.title }}</span><span class="badge">{{ node.status }}</span>@if(node.assignedToAgent){<span class="assignee">{{ agentAvatar(node.assignedToAgent) }} {{ agentName(node.assignedToAgent) }}</span>}<span class="muted small"> {{ node.children.length }} subtasks</span></div>
              @for (child of node.children; track child.id) {
                <div class="child"><span>↳ {{ child.title }}</span><span class="badge">{{ child.status }}</span>@if(child.assignedToAgent){<span class="assignee">{{ agentAvatar(child.assignedToAgent) }} {{ agentName(child.assignedToAgent) }}</span>}</div>
              }
            </div>
          }
        }
        @if (!tree().length) { <p class="muted">No tasks</p> }
      </div>

      <div class="activity">
        <h3>Live Activity (agent.thinking / delegated / completed)</h3>
        @for (ev of activity(); track ev.id) {
          <div class="ev">
            <span class="time">{{ ev.time }}</span>
            <span class="type">{{ ev.type }}</span>
            <span class="msg">{{ ev.message }}</span>
          </div>
        }
        @if (!activity().length) { <p class="muted">No recent activity — create a task assigned to an agent to see runtime events.</p> }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:16px; }
    .head { display:flex; justify-content:space-between; align-items:center; }
    h2 { margin:0; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn:disabled { opacity:.5; }
    .create { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; display:flex; flex-direction:column; gap:8px; margin:12px 0; }
    .input { padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; }
    .input.small { padding:6px 8px; font-size:12px; }
    .row { display:flex; gap:8px; }
    .row .input { flex:1; }
    .err { color:#dc2626; font-size:12px; }
    .filters { display:flex; gap:8px; margin:12px 0; }
    .board { display:grid; grid-template-columns:repeat(5,1fr); gap:12px; }
    .col { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:10px; min-height:300px; }
    .col h4 { margin:0 0 8px; font-size:12px; text-transform:uppercase; letter-spacing:.06em; display:flex; justify-content:space-between; }
    .count { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .card { display:block; background:#f7f8f9; border:1px solid #e8eaee; border-radius:8px; padding:8px; margin:6px 0; text-decoration:none; color:inherit; }
    .card:hover { border-color:#111827; }
    .title { font-weight:600; font-size:13px; }
    .muted { color:#6b7280; font-size:12px; }
    .muted.small { font-size:11px; }
    .meta { display:flex; gap:6px; margin-top:4px; flex-wrap:wrap; }
    .prio { padding:2px 6px; border-radius:999px; font-size:10px; background:#e8eaee; }
    .prio.high { background:#fef3c7; }
    .prio.urgent { background:#fee2e2; color:#dc2626; }
    .assignee { font-size:11px; background:#e0e7ff; padding:2px 6px; border-radius:999px; }
    .sub { font-size:10px; color:#9aa0b2; }
    .hierarchy { margin-top:16px; background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; }
    .node { border:1px solid #e8eaee; border-radius:8px; padding:8px; margin:6px 0; background:#f7f8f9; }
    .node-head { display:flex; gap:6px; align-items:center; font-weight:600; font-size:13px; }
    .child { display:flex; gap:6px; align-items:center; margin-left:16px; padding:4px 0; border-bottom:1px solid #e8eaee; font-size:12px; }
    .activity { margin-top:16px; background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; }
    .ev { display:flex; gap:8px; font-size:12px; padding:4px 0; border-bottom:1px solid #f7f8f9; }
    .time { color:#9aa0b2; font-size:11px; }
    .type { background:#111827; color:#fff; padding:2px 6px; border-radius:999px; font-size:10px; }
    .msg { flex:1; }
    @media(max-width:1000px){ .board{ grid-template-columns:repeat(2,1fr)} }
    @media(max-width:600px){ .board{ grid-template-columns:1fr} }
  `],
})
export class TasksComponent implements OnInit {
  private taskService = inject(TaskService);
  private projectService = inject(ProjectService);
  private agentService = inject(AgentService);
  private channelService = inject(ChannelService);
  private ws = inject(WsService);

  tasks = this.taskService.tasks;
  projects = this.projectService.projects;
  agents = this.agentService.agents;
  channels = this.channelService.channels;

  showCreate = signal(false);
  title = '';
  description = '';
  projectId = '';
  channelId = '';
  assigneeAgent = '';
  priority: string = 'medium';
  creating = signal(false);
  createError = signal('');
  filterProject = '';
  filterStatus = '';
  activity = signal<{id:string,type:string,message:string,time:string}[]>([]);

  columns = [
    { key: 'pending', label: 'Pending' },
    { key: 'assigned', label: 'Assigned' },
    { key: 'running', label: 'Running' },
    { key: 'waiting', label: 'Waiting' },
    { key: 'completed', label: 'Completed' },
  ];

  ngOnInit(){
    this.projectService.list().subscribe();
    this.agentService.list().subscribe();
    this.channelService.list().subscribe();
    this.reload();
    this.ws.events.subscribe(ev => {
      if (ev.type.startsWith('agent.')) {
        const time = new Date().toLocaleTimeString();
        this.activity.update(arr => [{ id: Math.random().toString(36), type: ev.type, message: JSON.stringify(ev.payload).slice(0,120), time }, ...arr].slice(0,20));
        this.taskService.handleIncoming(ev as any);
        this.reload();
      }
    });
  }

  reload(){
    this.taskService.list(this.filterProject || undefined, this.filterStatus || undefined).subscribe();
  }

  byStatus(status:string){
    return this.tasks().filter(t => t.status === status);
  }
  countBy(status:string){ return this.byStatus(status).length; }
  agentName(id:string){ return this.agents().find(a=>a.id===id)?.name || id.slice(0,8); }
  agentAvatar(id:string){ return this.agents().find(a=>a.id===id)?.avatarUrl || '🤖'; }

  // Hierarchy — parent → subtasks tree
  buildTree(tasks: any[]): any[] {
    const map = new Map<string, any>();
    tasks.forEach(t => map.set(t.id, {...t, children:[]}));
    const roots: any[] = [];
    map.forEach(node => {
      if (node.parentTaskId && map.has(node.parentTaskId)) {
        map.get(node.parentTaskId)!.children.push(node);
      } else {
        roots.push(node);
      }
    });
    return roots;
  }
  tree(){ return this.buildTree(this.tasks()); }

  create(){
    if (!this.title.trim() || !this.projectId) { this.createError.set('Title and project required'); return; }
    this.creating.set(true);
    this.createError.set('');
    this.taskService.create({
      title: this.title.trim(),
      description: this.description.trim(),
      projectId: this.projectId,
      channelId: this.channelId || undefined,
      assignedToAgent: this.assigneeAgent || undefined,
      priority: this.priority,
    }).subscribe({
      next: () => { this.creating.set(false); this.title=''; this.description=''; this.showCreate.set(false); this.reload(); },
      error: e => { this.creating.set(false); this.createError.set(e.error?.error?.message || 'Create failed'); }
    });
  }
}
