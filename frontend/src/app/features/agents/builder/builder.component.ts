import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AgentService, BuilderInput } from '../../../core/agent.service';
import { SettingsService } from '../../../core/settings.service';

@Component({
  selector: 'app-agent-builder',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="builder">
      <div class="header">
        <h1>Agent Builder</h1>
        <p class="muted">Describe what you need in plain English — we’ll turn it into a structured agent.</p>
        <div class="steps">
          @for (s of stepLabels; track s) {
            <div class="step" [class.active]="step()=== $index+1" [class.done]="step() > $index+1">
              <span class="num">{{ $index + 1 }}</span> {{ s }}
            </div>
          }
        </div>
      </div>

      <!-- Step 1 -->
      @if (step()===1) {
        <div class="card">
          <h2>What do you want this agent to do?</h2>
          <p class="muted">Example: “I need an agent that manages our company's social media accounts, creates content, analyzes performance and coordinates with other agents.”</p>
          <textarea [(ngModel)]="description" rows="4" placeholder="Describe the agent’s job in your own words…" class="input"></textarea>
          @if (!description.trim() && triedNext()) { <div class="err">Please describe the agent.</div> }
          <div class="actions"><button class="btn" (click)="next()">Next →</button></div>
        </div>
      }

      <!-- Step 2 -->
      @if (step()===2) {
        <div class="card">
          <h2>What should it be responsible for?</h2>
          <p class="muted">Add responsibilities in natural language. Pick suggestions or write your own.</p>
          <div class="chips">
            @for (s of suggestedResponsibilities; track s) {
              <button class="chip" [class.selected]="responsibilities().includes(s)" (click)="toggleResp(s)">{{ s }}</button>
            }
          </div>
          <div class="add-row">
            <input [(ngModel)]="newResp" placeholder="Add custom responsibility + Enter" class="input" (keydown.enter)="addResp()" />
            <button class="btn small" (click)="addResp()">Add</button>
          </div>
          @if (responsibilities().length) {
            <ul class="list">@for (r of responsibilities(); track r) { <li>{{ r }} <button class="x" (click)="removeResp(r)">×</button></li> }</ul>
          }
          <div class="actions"><button class="btn secondary" (click)="prev()">← Back</button><button class="btn" (click)="next()">Next →</button></div>
        </div>
      }

      <!-- Step 3 -->
      @if (step()===3) {
        <div class="card">
          <h2>What information can it use?</h2>
          <p class="muted">Knowledge sources — docs, guidelines, analytics, etc.</p>
          <div class="chips">
            @for (s of suggestedKnowledge; track s) {
              <button class="chip" [class.selected]="knowledge().includes(s)" (click)="toggleKnowledge(s)">{{ s }}</button>
            }
          </div>
          <div class="add-row">
            <input [(ngModel)]="newKnowledge" placeholder="Add knowledge source" class="input" (keydown.enter)="addKnowledge()" />
            <button class="btn small" (click)="addKnowledge()">Add</button>
          </div>
          @if (knowledge().length) { <ul class="list">@for (k of knowledge(); track k) { <li>{{ k }} <button class="x" (click)="removeKnowledge(k)">×</button></li> }</ul> }
          <div class="actions"><button class="btn secondary" (click)="prev()">← Back</button><button class="btn" (click)="next()">Next →</button></div>
        </div>
      }

      <!-- Step 4 -->
      @if (step()===4) {
        <div class="card">
          <h2>What actions can it perform?</h2>
          <p class="muted">Tools & actions — publishing will require approval by default.</p>
          <div class="chips">
            @for (s of suggestedActions; track s) {
              <button class="chip" [class.selected]="actions().includes(s)" (click)="toggleAction(s)">{{ s }}</button>
            }
          </div>
          <div class="add-row">
            <input [(ngModel)]="newAction" placeholder="Add action" class="input" (keydown.enter)="addAction()" />
            <button class="btn small" (click)="addAction()">Add</button>
          </div>
          @if (actions().length) { <ul class="list">@for (a of actions(); track a) { <li>{{ a }} <button class="x" (click)="removeAction(a)">×</button></li> }</ul> }
          <div class="actions"><button class="btn secondary" (click)="prev()">← Back</button><button class="btn" (click)="next()">Next →</button></div>
        </div>
      }

      <!-- Step 5 -->
      @if (step()===5) {
        <div class="card">
          <h2>How independently should it work?</h2>
          <div class="autonomy-grid">
            @for (lvl of autonomyLevels; track lvl.id) {
              <label class="autonomy-card" [class.selected]="autonomy()===lvl.id">
                <input type="radio" name="autonomy" [value]="lvl.id" [(ngModel)]="autonomyModel" />
                <strong>{{ lvl.label }}</strong>
                <p class="muted">{{ lvl.desc }}</p>
                @if (lvl.warning) { <span class="warn">{{ lvl.warning }}</span> }
              </label>
            }
          </div>
          @if (autonomy()==='autonomous' && hasDangerousPreview()) {
            <div class="warn-box">⚠️ Autonomous not allowed with publishing/external actions — will be downgraded to collaborative.</div>
          }
          <div class="actions"><button class="btn secondary" (click)="prev()">← Back</button><button class="btn" (click)="generatePreview()">Generate preview →</button></div>
        </div>
      }

      <!-- Step 6 Preview -->
      @if (step()===6) {
        <div class="card preview">
          <h2>Preview</h2>
          @if (loadingPreview()) { <p class="muted">Generating structured agent…</p> }
          @if (preview()) {
            <div class="preview-grid">
              <div class="p-card"><label>Name</label><input [(ngModel)]="editName" class="input" /></div>
              <div class="p-card"><label>Purpose</label><textarea [(ngModel)]="editPurpose" rows="2" class="input"></textarea></div>
              <div class="p-card"><label>Role</label><input [(ngModel)]="editRole" class="input" /></div>
              <div class="p-card"><label>Objective</label><textarea [(ngModel)]="editObjective" rows="2" class="input"></textarea></div>
              <div class="p-card full"><label>Responsibilities</label>
                @for (r of preview()!.responsibilities; track r) { <div class="pill">{{ r }}</div> }
                <input [(ngModel)]="editRespAdd" placeholder="Add responsibility" class="input" (keydown.enter)="addPreviewResp()" />
              </div>
              <div class="p-card"><label>Knowledge needed</label>
                @for (k of preview()!.knowledgeNeeds; track k) { <div class="pill">{{ k }}</div> }
              </div>
              <div class="p-card"><label>Capabilities</label>
                @for (c of preview()!.capabilities; track c.name) { <div class="pill" [class.danger]="isDangerous(c.name)">{{ c.name }} — {{ c.description }}</div> }
              </div>
              <div class="p-card"><label>Tools</label>
                @for (t of preview()!.recommendedTools; track t) { <div class="pill">{{ t }}</div> }
              </div>
              <div class="p-card"><label>Permissions</label>
                @for (p of preview()!.permissions; track p.resourceType) { <div class="pill">{{ p.resourceType }}:{{ p.permission }}</div> }
              </div>
              <div class="p-card"><label>Model</label>
                <select [(ngModel)]="editModel" class="input">
                  @for (m of availableModels; track m) { <option [value]="m">{{ m }}</option> }
                </select>
                <p class="muted small">Used via OPENROUTER_MODEL if you set it in .env, else this selection.</p>
              </div>
              <div class="p-card"><label>Approval rules</label><pre class="code">{{ stringify(preview()!.approvalPolicy) }}</pre></div>
              <div class="p-card"><label>Autonomy</label><span class="badge">{{ preview()!.autonomyLevel }}</span></div>
              @if (preview()!.warnings?.length) { <div class="p-card full warn-box">@for (w of preview()!.warnings; track w) { <div>⚠️ {{ w }}</div> }</div> }
            </div>
            <p class="muted small">You can edit the fields above before creating.</p>
          }
          @if (previewError()) { <div class="err">{{ previewError() }}</div> }
          <div class="actions">
            <button class="btn secondary" (click)="prev()">← Back</button>
            <button class="btn" (click)="step.set(7)" [disabled]="!preview()">Continue →</button>
          </div>
        </div>
      }

      <!-- Step 7 Create -->
      @if (step()===7) {
        <div class="card">
          <h2>Ready to create</h2>
          <p class="muted">Agent <strong>{{ editName || preview()?.name }}</strong> will be created in <strong>{{ orgName() }}</strong>.</p>
          @if (createError()) { <div class="err">{{ createError() }}</div> }
          <div class="actions">
            <button class="btn secondary" (click)="prev()">← Back</button>
            <button class="btn" (click)="createAgent()" [disabled]="creating()">{{ creating() ? 'Creating…' : 'Create agent' }}</button>
          </div>
        </div>
      }
    </div>
  `,
  styles: [`
    .builder { max-width:900px; margin:0 auto; padding:24px; }
    .header h1 { margin:0; }
    .muted { color:#6b7280; font-size:13px; }
    .muted.small { font-size:11px; }
    .steps { display:flex; gap:6px; margin-top:12px; flex-wrap:wrap; }
    .step { display:flex; align-items:center; gap:6px; font-size:11px; padding:6px 10px; border-radius:999px; background:#e8eaee; color:#6b7280; }
    .step.active { background:#111827; color:#fff; }
    .step.done { background:#d1fae5; color:#065f46; }
    .num { width:18px; height:18px; border-radius:50%; background:rgba(0,0,0,.1); display:grid; place-items:center; font-weight:700; font-size:10px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; margin-top:16px; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; font-family:inherit; box-sizing:border-box; }
    textarea.input { resize:vertical; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn:disabled { opacity:.5; }
    .actions { display:flex; gap:8px; margin-top:12px; justify-content:flex-end; }
    .err { color:#dc2626; font-size:12px; margin-top:6px; }
    .chips { display:flex; flex-wrap:wrap; gap:6px; margin:8px 0; }
    .chip { border:1px solid #e8eaee; background:#fff; border-radius:999px; padding:6px 10px; font-size:12px; cursor:pointer; }
    .chip.selected { background:#111827; color:#fff; border-color:#111827; }
    .add-row { display:flex; gap:6px; margin-top:8px; }
    .list { list-style:none; padding:0; margin:8px 0; }
    .list li { background:#f7f8f9; padding:6px 8px; border-radius:6px; margin:4px 0; display:flex; justify-content:space-between; font-size:13px; }
    .x { background:transparent; border:0; cursor:pointer; color:#9aa0b2; }
    .autonomy-grid { display:grid; grid-template-columns:repeat(2,1fr); gap:8px; margin-top:8px; }
    .autonomy-card { border:1px solid #e8eaee; border-radius:10px; padding:10px; cursor:pointer; display:flex; flex-direction:column; gap:4px; }
    .autonomy-card.selected { border-color:#111827; background:#f7f8f9; }
    .warn { font-size:10px; color:#d97706; }
    .warn-box { background:#fef3c7; border:1px solid #fde68a; padding:8px; border-radius:8px; font-size:12px; margin-top:8px; }
    .preview-grid { display:grid; grid-template-columns:1fr 1fr; gap:12px; margin-top:12px; }
    .p-card { background:#f7f8f9; border:1px solid #e8eaee; border-radius:8px; padding:10px; }
    .p-card.full { grid-column:1 / -1; }
    .p-card label { font-size:10px; text-transform:uppercase; letter-spacing:.06em; color:#6b7280; display:block; margin-bottom:4px; }
    .pill { background:#fff; border:1px solid #e8eaee; border-radius:999px; padding:4px 8px; font-size:11px; display:inline-block; margin:2px; }
    .pill.danger { background:#fef2f2; border-color:#fecaca; color:#dc2626; }
    .badge { background:#111827; color:#fff; padding:4px 8px; border-radius:999px; font-size:11px; }
    .code { background:#fff; padding:6px; border-radius:6px; font-size:11px; white-space:pre-wrap; }
    @media(max-width:700px){ .autonomy-grid, .preview-grid{ grid-template-columns:1fr } }
  `],
})
export class BuilderComponent implements OnInit {
  private agentService = inject(AgentService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  step = signal(1);
  triedNext = signal(false);
  stepLabels = ['Intent', 'Responsibilities', 'Knowledge', 'Actions', 'Autonomy', 'Preview', 'Create'];

  description = '';
  responsibilities = signal<string[]>([]);
  newResp = '';
  suggestedResponsibilities = ['Create content', 'Analyze performance', 'Schedule posts', 'Coordinate with team', 'Research topics', 'Engage audience'];

  knowledge = signal<string[]>([]);
  newKnowledge = '';
  suggestedKnowledge = ['Social media guidelines', 'Brand assets', 'Analytics data', 'Project docs', 'Customer feedback', 'Competitor research'];

  actions = signal<string[]>([]);
  newAction = '';
  suggestedActions = ['Publish to social media', 'Send emails', 'Query knowledge base', 'Schedule tasks', 'Generate report', 'Moderate comments'];

  autonomy = signal('task_executor');
  get autonomyModel() { return this.autonomy(); }
  set autonomyModel(v: string) { this.autonomy.set(v); }

  autonomyLevels = [
    { id: 'assistant', label: 'Assistant', desc: 'Waits for explicit human confirmation', warning: '' },
    { id: 'task_executor', label: 'Task Executor', desc: 'Executes tasks, asks for approval on sensitive actions', warning: '' },
    { id: 'collaborative', label: 'Collaborative', desc: 'Works with humans, handles routine autonomously', warning: '' },
    { id: 'autonomous', label: 'Autonomous', desc: 'Acts independently', warning: 'Dangerous actions will be downgraded' },
  ];

  preview = this.agentService.preview;
  loadingPreview = signal(false);
  previewError = signal('');
  editName = '';
  editPurpose = '';
  editRole = '';
  editObjective = '';
  editModel = 'minimax/minimax-m3:free';
  editRespAdd = '';
  orgName = signal('current workspace');
  private settings = inject(SettingsService);
  fallbackModels = [
    'minimax/minimax-m3:free',
    'openai/gpt-4o-mini',
    'openai/gpt-4o',
    'anthropic/claude-3.5-sonnet',
    'google/gemini-1.5-flash',
    'meta-llama/llama-3.1-70b',
  ];
  get availableModels(): string[] {
    return this.settings.config()?.availableModels?.length ? this.settings.config()!.availableModels : this.fallbackModels;
  }

  createError = signal('');
  creating = signal(false);

  ngOnInit() {
    const intent = this.route.snapshot.queryParamMap.get('intent');
    if (intent) this.description = intent;
    this.settings.llm().subscribe();
  }

  // helpers
  toggleResp(s: string) { this.responsibilities.update(arr => arr.includes(s) ? arr.filter(x => x !== s) : [...arr, s]); }
  addResp() { if (this.newResp.trim()) { this.responsibilities.update(arr => [...arr, this.newResp.trim()]); this.newResp=''; } }
  removeResp(r: string) { this.responsibilities.update(arr => arr.filter(x=>x!==r)); }

  toggleKnowledge(s: string) { this.knowledge.update(arr => arr.includes(s) ? arr.filter(x=>x!==s):[...arr,s]); }
  addKnowledge() { if (this.newKnowledge.trim()) { this.knowledge.update(arr=>[...arr,this.newKnowledge.trim()]); this.newKnowledge=''; } }
  removeKnowledge(k:string){ this.knowledge.update(arr=>arr.filter(x=>x!==k)); }

  toggleAction(s:string){ this.actions.update(arr=>arr.includes(s)?arr.filter(x=>x!==s):[...arr,s]); }
  addAction(){ if(this.newAction.trim()){ this.actions.update(arr=>[...arr,this.newAction.trim()]); this.newAction=''; } }
  removeAction(a:string){ this.actions.update(arr=>arr.filter(x=>x!==a)); }

  hasDangerousPreview() {
    const p = this.preview();
    return !!p?.capabilities?.find(c=> ['publish_content','send_email','external_api'].includes(c.name));
  }
  isDangerous(name:string){ return ['publish_content','send_email','external_api','delete_data','financial'].includes(name); }

  next() {
    if (this.step()===1 && !this.description.trim()){ this.triedNext.set(true); return; }
    this.step.update(v=> Math.min(7, v+1));
  }
  prev(){ this.step.update(v=> Math.max(1, v-1)); }

  generatePreview() {
    this.loadingPreview.set(true);
    this.previewError.set('');
    const input: BuilderInput = {
      description: this.description.trim(),
      responsibilities: this.responsibilities(),
      informationSources: this.knowledge(),
      actions: this.actions(),
      autonomyPreference: this.autonomy(),
    };
    this.agentService.builderPreview(input).subscribe({
      next: res => {
        this.loadingPreview.set(false);
        const p = res.data;
        this.editName = p.name;
        this.editPurpose = p.purpose;
        this.editRole = p.role;
        this.editObjective = p.objective;
        this.editModel = (p as any).modelConfiguration?.model || p.modelConfiguration?.model || 'openai/gpt-4o-mini';
        this.step.set(6);
      },
      error: e => { this.loadingPreview.set(false); this.previewError.set(e.error?.error?.message || 'Failed to generate preview'); }
    });
  }

  addPreviewResp(){ if(this.editRespAdd.trim()){ this.preview.update(p=> p ? {...p, responsibilities:[...p.responsibilities, this.editRespAdd.trim()]}:p); this.editRespAdd=''; } }
  stringify(o:any){ return JSON.stringify(o, null, 2); }

  createAgent(){
    this.creating.set(true);
    this.createError.set('');
    const base = this.preview()!;
    const preview = {
      ...base,
      name: this.editName.trim() || base.name,
      purpose: this.editPurpose.trim() || base.purpose,
      role: this.editRole.trim() || base.role,
      objective: this.editObjective.trim() || base.objective,
      modelConfiguration: { ...(base as any).modelConfiguration, model: this.editModel, temperature: (base as any).modelConfiguration?.temperature || 0.7 },
    };
    this.agentService.create(preview as any, this.description).subscribe({
      next: res => { this.creating.set(false); this.router.navigate(['/agents', res.data.id]); },
      error: e => { this.creating.set(false); this.createError.set(e.error?.error?.message || 'Create failed'); }
    });
  }
}
