import { Component, inject, OnInit, signal, OnDestroy } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { TaskService } from '../../core/task.service';
import { AgentService } from '../../core/agent.service';
import { WsService } from '../../core/ws.service';

@Component({
  selector: 'app-task-detail',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      @if (task()) {
        <div class="hero">
          <div>
            <h1>{{ task()!.title }}</h1>
            <p class="muted">{{ task()!.description || 'No description' }}</p>
            <div class="badges">
              <span class="badge">{{ task()!.status }}</span>
              <span class="badge prio">{{ task()!.priority }}</span>
              @if (task()!.assignedToAgent) { <span class="badge dark">🤖 {{ agentName(task()!.assignedToAgent!) }}</span> }
              @if (task()!.parentTaskId) { <span class="badge">↳ subtask of {{ task()!.parentTaskId!.slice(0,8) }}</span> }
            </div>
          </div>
          <div class="actions">
            <select [(ngModel)]="newAssignee" class="input">
              <option value="">Assign to…</option>
              @for (a of agents(); track a.id) { <option [value]="a.id">🤖 {{ a.name }}</option> }
            </select>
            <button class="btn small" (click)="assign()">Assign</button>
            <select [(ngModel)]="newStatus" class="input">
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
            <button class="btn small" (click)="changeStatus()">Update</button>
          </div>
        </div>

        <div class="grid">
          <div class="card">
            <h3>Subtasks & Delegation</h3>
            <div class="delegate">
              <input [(ngModel)]="delegateTitle" placeholder="Subtask title" class="input" />
              <input [(ngModel)]="delegateDesc" placeholder="Description" class="input" />
              <select [(ngModel)]="delegateAgent" class="input">
                <option value="">Delegate to agent…</option>
                @for (a of agents(); track a.id) { <option [value]="a.id">🤖 {{ a.name }}</option> }
              </select>
              <button class="btn small" (click)="delegate()">Delegate</button>
            </div>
            @for (st of subtasks(); track st.id) {
              <div class="row"><span>{{ st.title }}</span><span class="badge">{{ st.status }}</span><span class="muted">{{ agentName(st.assignedToAgent || '') }}</span></div>
            }
            @if (!subtasks().length) { <p class="muted">No subtasks — delegate to create agent↔agent work.</p> }
          </div>

          <div class="card">
            <h3>Events (Live WS: thinking → delegated → completed)</h3>
            @for (ev of liveEvents(); track ev.id) {
              <div class="ev live"><span class="time">{{ ev.time }}</span><span class="type">{{ ev.type }}</span><span class="payload">{{ ev.message }}</span></div>
            }
            @for (ev of events(); track ev.id) {
              <div class="ev">
                <span class="time">{{ ev.createdAt }}</span>
                <span class="type">{{ ev.eventType }}</span>
                <span class="actor">{{ ev.actorType }}:{{ (ev.actorAgentId || ev.actorUserId || '').slice(0,8) }}</span>
                <span class="payload">{{ stringify(ev.payload) }}</span>
              </div>
            }
            @if (!events().length && !liveEvents().length) { <p class="muted">No events yet.</p> }
          </div>

          <div class="card">
            <h3>Agent Runs — Timeline & Cost</h3>
            @for (run of runs(); track run.id) {
              <div class="run">
                <div class="row"><span>Run {{ run.id.slice(0,8) }}</span><span class="badge">{{ run.status }}</span><span class="muted small">{{ run.triggerType || '' }}</span></div>
                <div class="muted small">Agent {{ run.agentId.slice(0,8) }} • {{ run.createdAt }} • {{ run.correlationId?.slice(0,8) || '' }}</div>
                @if (run.tokenMetadata) {
                  <div class="muted small" style="background:#f0f9ff; padding:4px 6px; border-radius:6px; margin:4px 0;">
                    Tokens: {{ run.tokenMetadata.promptTokens || 0 }} prompt + {{ run.tokenMetadata.completionTokens || 0 }} completion = {{ run.tokenMetadata.totalTokens || 0 }} • Cost $ {{ (run.tokenMetadata.estimatedCost || 0).toFixed(4) }}
                  </div>
                }
                @if (run.actions?.length) {
                  <ul class="actions">
                    @for (act of run.actions; track act.seq) {
                      <li>{{ act.seq }}: {{ act.actionType }} {{ act.toolName || '' }} — {{ act.status }}</li>
                    }
                  </ul>
                }
              </div>
            }
            @if (!runs().length) { <p class="muted">No runs yet — assign task to an agent to start runtime.</p> }
          </div>

          <div class="card">
            <h3>Context</h3>
            <div class="row"><span>Project</span><span>{{ task()!.projectId.slice(0,8) }}</span></div>
            <div class="row"><span>Channel</span><span>{{ task()!.channelId || '—' }}</span></div>
            <div class="row"><span>Correlation</span><span class="muted small">{{ task()!.correlationId || '—' }}</span></div>
            <div class="row"><span>Created</span><span>{{ task()!.createdAt }}</span></div>
          </div>
        </div>
      } @else {
        <p class="muted">Loading task…</p>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:16px; max-width:1100px; margin:0 auto; }
    .hero { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; display:flex; justify-content:space-between; gap:12px; }
    h1 { margin:0; font-size:18px; }
    .muted { color:#6b7280; font-size:12px; }
    .badges { display:flex; gap:6px; margin-top:6px; flex-wrap:wrap; }
    .badge { background:#e8eaee; padding:4px 8px; border-radius:999px; font-size:11px; }
    .badge.dark { background:#111827; color:#fff; }
    .badge.prio { text-transform:capitalize; }
    .actions { display:flex; gap:6px; flex-wrap:wrap; align-items:center; }
    .input { padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .btn.small { background:#111827; color:#fff; border:0; padding:6px 10px; border-radius:6px; font-size:12px; cursor:pointer; }
    .grid { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; }
    .delegate { display:flex; flex-direction:column; gap:6px; margin:8px 0; }
    .row { display:flex; justify-content:space-between; font-size:13px; padding:4px 0; border-bottom:1px solid #f7f8f9; }
    .ev { display:flex; gap:6px; font-size:11px; padding:4px 0; border-bottom:1px solid #f7f8f9; flex-wrap:wrap; }
    .ev.live { background:#f0f9ff; border-color:#dbeafe; }
    .time { color:#9aa0b2; }
    .type { background:#e0e7ff; padding:2px 6px; border-radius:999px; font-size:10px; }
    .actor { color:#6b7280; }
    .payload { flex:1; white-space:pre-wrap; }
    .run { border:1px solid #e8eaee; border-radius:8px; padding:8px; margin:6px 0; }
    .actions { list-style:none; padding:0; margin:6px 0; }
    .actions li { font-size:11px; padding:2px 0; }
    @media(max-width:800px){ .grid{ grid-template-columns:1fr } .hero{ flex-direction:column } }
  `],
})
export class TaskDetailComponent implements OnInit, OnDestroy {
  private route = inject(ActivatedRoute);
  private taskService = inject(TaskService);
  private agentService = inject(AgentService);
  private ws = inject(WsService);
  task = this.taskService.selected;
  events = this.taskService.events;
  runs = this.taskService.runs;
  agents = this.agentService.agents;
  subtasks = signal<any[]>([]);
  liveEvents = signal<{id:string,type:string,message:string,time:string}[]>([]);
  newAssignee = '';
  newStatus = 'pending';
  delegateTitle = '';
  delegateDesc = '';
  delegateAgent = '';
  private wsSub: any;

  ngOnInit(){
    const id = this.route.snapshot.paramMap.get('id')!;
    this.taskService.get(id).subscribe();
    this.taskService.listEvents(id).subscribe();
    this.taskService.listRuns(id).subscribe();
    this.agentService.list().subscribe();
    this.taskService.list().subscribe(res => {
      const all = (res as any).data || [];
      setTimeout(()=> {
        const tasks = this.taskService.tasks();
        this.subtasks.set(tasks.filter(t=> t.parentTaskId===id));
      }, 500);
    });
    // Live WS for this task
    this.wsSub = this.ws.events.subscribe(ev => {
      const payload: any = ev.payload;
      const taskId = payload?.taskId || payload?.task?.id;
      if (taskId !== id && ev.type !== 'agent.message' && !ev.type.startsWith('agent.')) return;
      if (['agent.thinking','agent.tool_completed','agent.delegated','agent.completed','agent.failed','agent.waiting','agent.message','agent.task_created'].includes(ev.type)) {
        const time = new Date().toLocaleTimeString();
        const msg = payload?.response || payload?.reason || payload?.tool || JSON.stringify(payload).slice(0,120);
        this.liveEvents.update(arr => [{id: Math.random().toString(36), type: ev.type, message: msg, time}, ...arr].slice(0,20));
        // refresh subtasks/events on delegation/completion
        if (ev.type === 'agent.delegated' || ev.type === 'agent.task_created') {
          this.taskService.list().subscribe(r => {
            const tasks = this.taskService.tasks();
            this.subtasks.set(tasks.filter(t=> t.parentTaskId===id));
          });
        }
        if (ev.type === 'agent.completed' || ev.type === 'agent.failed') {
          this.taskService.listEvents(id).subscribe();
          this.taskService.listRuns(id).subscribe();
        }
      }
    });
  }
  ngOnDestroy(){ if (this.wsSub) this.wsSub.unsubscribe(); }

  agentName(id:string){ return this.agents().find(a=>a.id===id)?.name || id.slice(0,8); }
  stringify(o:any){ return JSON.stringify(o); }

  assign(){
    const id = this.route.snapshot.paramMap.get('id')!;
    if (!this.newAssignee) return;
    this.taskService.assign(id, this.newAssignee).subscribe(()=> this.taskService.get(id).subscribe());
  }
  changeStatus(){
    const id = this.route.snapshot.paramMap.get('id')!;
    this.taskService.updateStatus(id, this.newStatus).subscribe(()=> this.taskService.get(id).subscribe());
  }
  delegate(){
    const id = this.route.snapshot.paramMap.get('id')!;
    if (!this.delegateTitle.trim()) return;
    this.taskService.delegate(id, { title: this.delegateTitle, description: this.delegateDesc, assignedToAgent: this.delegateAgent || undefined }).subscribe(()=> {
      this.delegateTitle=''; this.delegateDesc='';
      this.taskService.list().subscribe();
      this.taskService.listEvents(id).subscribe();
    });
  }
}
