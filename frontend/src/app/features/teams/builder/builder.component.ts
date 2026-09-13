import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { TeamService } from '../../../core/team.service';

@Component({
  selector: 'app-team-builder',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="builder">
      <h1>Team Builder</h1>
      <p class="muted">Describe a business outcome — we'll propose an AI agent team. You review before creation.</p>

      @if (step()===1) {
        <div class="card">
          <h2>What outcome do you want?</h2>
          <p class="muted">Example: "I want AI to manage our company's social media."</p>
          <textarea [(ngModel)]="outcome" rows="4" placeholder="Describe the outcome…" class="input"></textarea>
          <div class="actions"><button class="btn" (click)="propose()" [disabled]="loading()">{{ loading() ? 'Thinking…' : 'Propose team →' }}</button></div>
          @if (error()) { <div class="err">{{ error() }}</div> }
        </div>
      }

      @if (step()===2 && preview()) {
        <div class="card">
          <h2>Proposed team: {{ preview()!.name }}</h2>
          <p class="muted">{{ preview()!.objective }}</p>
          <p>{{ preview()!.description }}</p>

          <h3>Recommended agents ({{ preview()!.agents.length }})</h3>
          @for (agent of preview()!.agents; track agent.name) {
            <div class="agent-card">
              <strong>{{ agent.name }}</strong> — {{ agent.role }}
              <div class="muted">{{ agent.responsibilities }}</div>
              <div class="meta">Depends: {{ agent.dependencies.join(', ') || 'none' }} | Tools: {{ agent.tools.join(', ') }} | Autonomy: {{ agent.autonomy }}</div>
            </div>
          }

          <h3>Workflow</h3>
          <div class="workflow">
            @for (step of preview()!.workflow; track step.from) {
              <div class="step">{{ step.from }} → {{ step.to }} ({{ step.action }})</div>
            }
          </div>

          <h3>Communication & Delegation</h3>
          <p class="muted">{{ preview()!.communicationRules.join('; ') }}</p>
          <p class="muted">{{ preview()!.delegationRules.join('; ') }}</p>

          <div class="actions">
            <button class="btn secondary" (click)="step.set(1)">← Back</button>
            <button class="btn" (click)="step.set(3)">Customize →</button>
          </div>
        </div>
      }

      @if (step()===3 && preview()) {
        <div class="card">
          <h2>Customize team</h2>
          <p class="muted">Add, remove, or modify agents, responsibilities, autonomy, approval policy.</p>

          @for (agent of preview()!.agents; track agent.name) {
            <div class="agent-edit">
              <input [(ngModel)]="agent.name" class="input" placeholder="Agent name" />
              <input [(ngModel)]="agent.role" class="input" placeholder="Role" />
              <textarea [(ngModel)]="agent.responsibilities" rows="2" class="input" placeholder="Responsibilities"></textarea>
              <div class="row">
                <select [(ngModel)]="agent.autonomy" class="input">
                  <option value="assistant">assistant</option>
                  <option value="task_executor">task_executor</option>
                  <option value="collaborative">collaborative</option>
                  <option value="autonomous">autonomous</option>
                </select>
                <button class="btn small danger" (click)="removeAgent(agent.name)">Remove</button>
              </div>
            </div>
          }

          <div class="add">
            <h4>Add agent</h4>
            <input [(ngModel)]="newAgentName" placeholder="Name" class="input" />
            <input [(ngModel)]="newAgentRole" placeholder="Role" class="input" />
            <button class="btn small" (click)="addAgent()">Add</button>
          </div>

          <div class="actions">
            <button class="btn secondary" (click)="step.set(2)">← Back</button>
            <button class="btn" (click)="step.set(4)">Preview →</button>
          </div>
        </div>
      }

      @if (step()===4 && preview()) {
        <div class="card preview">
          <h2>Preview</h2>
          <div class="grid">
            <div><label>Team name</label><input [(ngModel)]="preview()!.name" class="input" /></div>
            <div><label>Objective</label><textarea [(ngModel)]="preview()!.objective" rows="2" class="input"></textarea></div>
          </div>
          <h3>Agents</h3>
          @for (a of preview()!.agents; track a.name) { <div class="pill">{{ a.name }} ({{ a.role }}) — {{ a.autonomy }}</div> }
          <h3>Workflow</h3>
          @for (s of preview()!.workflow; track s.from) { <div class="pill">{{ s.from }} → {{ s.to }}</div> }
          @if ((preview()?.agents?.length||0)===0) { <div class="err">Add at least one agent</div> }
          <div class="actions">
            <button class="btn secondary" (click)="step.set(3)">← Back</button>
            <button class="btn" (click)="step.set(5)" [disabled]="(preview()?.agents?.length||0)===0">Continue →</button>
          </div>
        </div>
      }

      @if (step()===5) {
        <div class="card">
          <h2>Ready to create</h2>
          <p>Team <strong>{{ preview()?.name }}</strong> with {{ preview()?.agents?.length || 0 }} agents will be created. Agents will <b>not</b> be auto-created without your approval — you are reviewing now.</p>
          <div class="actions">
            <button class="btn secondary" (click)="step.set(4)">← Back</button>
            <button class="btn" (click)="create()" [disabled]="creating()">{{ creating() ? 'Creating…' : 'Create team' }}</button>
          </div>
          @if (createError()) { <div class="err">{{ createError() }}</div> }
        </div>
      }
    </div>
  `,
  styles: [`
    .builder { max-width:900px; margin:0 auto; padding:24px; }
    .muted { color:#6b7280; font-size:13px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; margin-top:16px; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; margin-top:6px; box-sizing:border-box; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn.danger { background:#fee2e2; color:#dc2626; }
    .btn:disabled { opacity:.5; }
    .actions { display:flex; gap:8px; justify-content:flex-end; margin-top:12px; }
    .err { color:#dc2626; font-size:12px; margin-top:6px; }
    .agent-card { background:#f7f8f9; border:1px solid #e8eaee; border-radius:8px; padding:10px; margin:6px 0; }
    .meta { font-size:11px; color:#6b7280; margin-top:4px; }
    .workflow { display:flex; flex-direction:column; gap:4px; margin:8px 0; }
    .step { background:#e0e7ff; padding:6px 8px; border-radius:6px; font-size:12px; }
    .agent-edit { background:#f7f8f9; padding:10px; border-radius:8px; margin:6px 0; display:flex; flex-direction:column; gap:6px; }
    .row { display:flex; gap:6px; }
    .preview .grid { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
    .pill { background:#e8eaee; padding:4px 8px; border-radius:999px; font-size:11px; display:inline-block; margin:2px; }
    .add { background:#f7f8f9; padding:10px; border-radius:8px; margin-top:12px; display:flex; flex-direction:column; gap:6px; }
    @media(max-width:700px){ .preview .grid{ grid-template-columns:1fr } }
  `],
})
export class BuilderComponent {
  private svc = inject(TeamService);
  private router = inject(Router);
  step = signal(1);
  outcome = '';
  preview = this.svc.preview;
  loading = signal(false);
  error = signal('');
  creating = signal(false);
  createError = signal('');
  newAgentName = '';
  newAgentRole = '';

  propose(){
    if (!this.outcome.trim()) { this.error.set('Describe the outcome'); return; }
    this.loading.set(true);
    this.error.set('');
    this.svc.builderPreview(this.outcome).subscribe({
      next: ()=> { this.loading.set(false); this.step.set(2); },
      error: e=> { this.loading.set(false); this.error.set(e.error?.error?.message || 'Failed'); }
    });
  }

  removeAgent(name:string){
    this.preview.update(p=> p ? {...p, agents: p.agents.filter(a=> a.name!==name)} : p);
  }
  addAgent(){
    if (!this.newAgentName.trim()) return;
    this.preview.update(p=> {
      if (!p) return p;
      return {...p, agents: [...p.agents, { name: this.newAgentName, role: this.newAgentRole || 'Member', responsibilities: 'Custom', dependencies: [], tools: [], knowledge: [], autonomy: 'task_executor', approvalPolicy: 'low' }]};
    });
    this.newAgentName=''; this.newAgentRole='';
  }

  create(){
    const p = this.preview();
    if (!p) return;
    this.creating.set(true);
    this.createError.set('');
    this.svc.create(p).subscribe({
      next: res=> { this.creating.set(false); this.router.navigate(['/teams', res.data.id]); },
      error: e=> { this.creating.set(false); this.createError.set(e.error?.error?.message || 'Create failed'); }
    });
  }
}
