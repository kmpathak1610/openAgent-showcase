import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SchedulerService } from '../../core/scheduler.service';
import { ProjectService } from '../../core/project.service';
import { AgentService } from '../../core/agent.service';

@Component({
  selector: 'app-scheduler',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <h1>Scheduling & Triggers</h1>
      <p class="muted">Triggers create tasks/events, not direct agent coupling. Supports one-time, recurring, timezone-aware.</p>

      <div class="card">
        <h3>Create Trigger</h3>
        <input [(ngModel)]="name" placeholder="Name — e.g., Weekly social review" class="input" />
        <select [(ngModel)]="triggerType" class="input">
          <option value="schedule">schedule</option>
          <option value="message_received">message_received</option>
          <option value="task_completed">task_completed</option>
          <option value="task_failed">task_failed</option>
          <option value="new_document">new_document</option>
          <option value="approval_received">approval_received</option>
          <option value="project_event">project_event</option>
        </select>

        @if (triggerType==='schedule') {
          <div class="schedule">
            <label class="muted small">Schedule type</label>
            <select [(ngModel)]="scheduleType" class="input">
              <option value="once">One-time</option>
              <option value="recurring">Recurring</option>
            </select>
            @if (scheduleType==='once') {
              <input type="datetime-local" [(ngModel)]="scheduledAt" class="input" />
            } @else {
              <input [(ngModel)]="cron" placeholder="Cron e.g., 0 9 * * 1 (Monday 9am)" class="input" />
              <p class="muted small">Example: "Every Monday at 9 AM, review our social performance." → cron "0 9 * * 1"</p>
            }
            <input [(ngModel)]="timezone" placeholder="Timezone e.g., UTC, America/New_York" class="input" />
          </div>
        }

        <input [(ngModel)]="title" placeholder="Task title to create" class="input" />
        <textarea [(ngModel)]="description" rows="2" placeholder="Task description" class="input"></textarea>
        <select [(ngModel)]="projectId" class="input">
          <option value="">Select project</option>
          @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
        </select>
        <select [(ngModel)]="agentId" class="input">
          <option value="">Select agent (for autonomous)</option>
          @for (a of agents(); track a.id) { <option [value]="a.id">{{ a.name }} ({{ a.autonomyLevel }})</option> }
        </select>
        <button class="btn" (click)="create()" [disabled]="creating()">{{ creating() ? 'Creating…' : 'Create Trigger' }}</button>
        @if (error()) { <div class="err">{{ error() }}</div> }
      </div>

      <div class="card">
        <h3>Triggers</h3>
        @for (t of triggers(); track t.id) {
          <div class="row">
            <span><strong>{{ t.name }}</strong> — {{ t.triggerType }} @if (t.nextRunAt) { <span class="muted">next {{ t.nextRunAt }}</span> }</span>
            <button class="mini danger" (click)="remove(t.id)">Delete</button>
          </div>
        }
        @if (!triggers().length) { <p class="muted">No triggers. Create "Every Monday at 9 AM" as recurring schedule.</p> }
      </div>

      <div class="card">
        <h3>Scheduled Tasks</h3>
        @for (s of schedules(); track s.triggerId) {
          <div class="row"><span>{{ s.name }}</span><span class="muted">{{ s.nextRunAt }}</span></div>
        }
        @if (!schedules().length) { <p class="muted">No scheduled tasks. Scheduler creates tasks/events, not direct agent coupling.</p> }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .muted.small { font-size:11px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; margin:12px 0; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; margin-top:6px; box-sizing:border-box; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; margin-top:8px; }
    .btn:disabled { opacity:.5; }
    .err { color:#dc2626; font-size:12px; }
    .row { display:flex; justify-content:space-between; padding:6px 0; border-bottom:1px solid #f7f8f9; font-size:13px; }
    .mini { border:1px solid #e8eaee; background:#fff; border-radius:6px; padding:4px 8px; font-size:11px; cursor:pointer; }
    .mini.danger { color:#dc2626; border-color:#fecaca; }
    .schedule { background:#f7f8f9; padding:10px; border-radius:8px; margin:6px 0; }
  `],
})
export class SchedulerComponent implements OnInit {
  private svc = inject(SchedulerService);
  private projectSvc = inject(ProjectService);
  private agentSvc = inject(AgentService);
  triggers = this.svc.triggers;
  schedules = this.svc.schedules;
  projects = this.projectSvc.projects;
  agents = this.agentSvc.agents;

  name = '';
  triggerType: string = 'schedule';
  scheduleType: string = 'once';
  scheduledAt = '';
  cron = '0 9 * * 1';
  timezone = 'UTC';
  title = 'Review social performance';
  description = 'Every Monday at 9 AM, review our social performance.';
  projectId = '';
  agentId = '';
  creating = signal(false);
  error = signal('');

  ngOnInit(){
    this.svc.list().subscribe();
    this.svc.listSchedules().subscribe();
    this.projectSvc.list().subscribe();
    this.agentSvc.list().subscribe();
  }

  create(){
    if (!this.name.trim()) { this.error.set('Name required'); return; }
    let config:any = { title: this.title, description: this.description, timezone: this.timezone };
    if (this.triggerType==='schedule') {
      if (this.scheduleType==='once' && this.scheduledAt) {
        config.scheduledAt = new Date(this.scheduledAt).toISOString();
      } else if (this.scheduleType==='recurring' && this.cron) {
        config.cron = this.cron;
        config.timezone = this.timezone;
      }
      config.title = this.title;
      config.description = this.description;
    }
    this.creating.set(true);
    this.error.set('');
    this.svc.create({ name: this.name, triggerType: this.triggerType, config, agentId: this.agentId || undefined, projectId: this.projectId || undefined }).subscribe({
      next: ()=> { this.creating.set(false); this.name=''; },
      error: e=> { this.creating.set(false); this.error.set(e.error?.error?.message || 'Failed'); }
    });
  }

  remove(id:string){ this.svc.remove(id).subscribe(); }
}
