import { Component, inject, OnInit, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { AgentService } from '../../core/agent.service';
import { SettingsService } from '../../core/settings.service';
import { KnowledgeService } from '../../core/knowledge.service';
import { TaskService } from '../../core/task.service';
import { MemoryService } from '../../core/memory.service';

@Component({
  selector: 'app-agent-detail',
  standalone: true,
  imports: [RouterLink, FormsModule],
  template: `
    <div class="detail">
      @if (agent()) {
        <div class="hero">
          <div class="avatar">{{ agent()!.avatar || '🤖' }}</div>
          <div class="meta">
            <h1>{{ agent()!.name }} <span class="slug">{{ agent()!.slug }}</span></h1>
            <p class="purpose">{{ agent()!.purpose || agent()!.description }}</p>
            <div class="badges">
              <span class="badge">{{ agent()!.status }}</span>
              <span class="badge dark">{{ agent()!.autonomyLevel }}</span>
              <span class="badge">v{{ agent()!.currentVersion }}</span>
            </div>
          </div>
          <a routerLink="/agents" class="btn secondary">← Back</a>
        </div>

        <div class="grid">
          <div class="card">
            <h3>Identity</h3>
            <div class="row"><span>Purpose</span><span>{{ agent()!.purpose || '—' }}</span></div>
            <div class="row"><span>Owner</span><span>{{ agent()!.ownerId || '—' }}</span></div>
            <div class="row"><span>Created</span><span>{{ agent()!.createdAt }}</span></div>
            <div class="row"><span>Version</span><span>v{{ agent()!.currentVersion }}</span></div>
            @if (agent()!.version) {
              <div class="sub">
                <h4>Version {{ agent()!.version!.version }} — {{ agent()!.version!.role }}</h4>
                <p class="muted">Objective: {{ agent()!.version!.objective }}</p>
                <p class="muted">Instructions: {{ agent()!.version!.instructions }}</p>
              </div>
            }
          </div>

          <div class="card">
            <h3>Capabilities</h3>
            @if (agent()!.capabilityList?.length) {
              @for (c of agent()!.capabilityList; track c.name) {
                <div class="row"><span>{{ c.name }}</span><span class="pill" [class.on]="c.enabled">{{ c.enabled ? 'enabled' : 'disabled' }}</span></div>
                <div class="muted small">{{ c.description }}</div>
              }
            } @else if (agent()!.version?.capabilities?.length) {
              @for (c of agent()!.version!.capabilities; track c.name) {
                <div class="row"><span>{{ c.name }}</span><span class="pill on">{{ c.enabled ? 'enabled' : 'disabled' }}</span></div>
              }
            } @else { <p class="muted">No capabilities</p> }
            <h3 style="margin-top:12px;">Knowledge</h3>
            @if (agentKnowledge().length) {
              @for (k of agentKnowledge(); track k.id) {
                <div class="row">
                  <span>📄 {{ k.document?.title || k.collection?.name || k.documentId || k.collectionId }}</span>
                  <button class="mini" (click)="detachKnowledge(k.id)">Detach</button>
                </div>
              }
            } @else {
              <p class="muted">No knowledge attached. Use below to attach.</p>
            }
            <div class="attach-row">
              <select [(ngModel)]="attachDocId" class="input">
                <option value="">Select document</option>
                @for (d of availableDocs(); track d.id) { <option [value]="d.id">{{ d.title }} • {{ d.scope }}</option> }
              </select>
              <button class="btn small" (click)="attachSelected()">Attach</button>
            </div>
            <p class="muted small">{{ knowledgeText() }}</p>
            <h3>Tools</h3>
            <p class="muted">{{ toolsText() }}</p>
          </div>

          <div class="card">
            <h3>Permissions</h3>
            @if (agent()!.permissions?.length) {
              @for (p of agent()!.permissions; track p.id) {
                <div class="row"><span>{{ p.resourceType }}:{{ p.permission }}</span><span class="muted">{{ p.resourceId || 'wildcard' }}</span></div>
              }
            } @else { <p class="muted">No extra permissions (least privilege)</p> }
            <h3 style="margin-top:12px;">Projects</h3>
            <p class="muted">Projects will appear here when agent is assigned. Use project detail to assign.</p>
          </div>

          <div class="card">
            <h3>Active Tasks</h3>
            @if (agentTasks().length) {
              @for (t of agentTasks(); track t.id) {
                <a class="row" [routerLink]="['/tasks', t.id]" style="text-decoration:none;color:inherit">
                  <span>{{ t.title }}</span><span class="badge">{{ t.status }}</span>
                </a>
              }
            } @else { <p class="muted">No active tasks — assign a task to see runtime.</p> }
            <h3 style="margin-top:12px;">Run History</h3>
            @if (agentRuns().length) {
              @for (r of agentRuns(); track r.id) {
                <div class="row"><span>{{ r.id.slice(0,8) }} • {{ r.status }}</span><span class="muted">{{ r.createdAt }}</span></div>
              }
            } @else { <p class="muted">No runs yet.</p> }
          </div>

          <div class="card">
            <h3>Approval & Behavioral Rules</h3>
            @if (agent()!.version?.approvalPolicy) {
              <pre class="code">{{ stringify(agent()!.version!.approvalPolicy) }}</pre>
            }
            @if (agent()!.version?.behavioralRules?.length) {
              <ul>@for (r of agent()!.version!.behavioralRules; track r) { <li>{{ r }}</li> }</ul>
            }
            <h3 style="margin-top:12px;">Model Configuration</h3>
            <pre class="code">{{ stringify(agent()!.version?.modelConfiguration) }}</pre>
          </div>

          <div class="card">
            <h3>Version History</h3>
            @if (versions().length) {
              @for (v of versions(); track v.version) {
                <div class="row"><span>v{{ v.version }} — {{ v.role }}</span><span class="muted">{{ v.createdAt }}</span></div>
                <div class="muted small">{{ v.changeSummary }}</div>
              }
            } @else { <p class="muted">Loading…</p> }
          </div>

          <div class="card">
            <h3>Memory (lifecycle)</h3>
            <div class="row muted small"><span>Active</span><span>{{ activeMemories().length }}</span></div>
            <div class="row muted small"><span>Stale / Archived</span><span>{{ staleMemories().length }}</span></div>
            @if (agentMemories().length) {
              @for (m of agentMemories(); track m.id) {
                <div class="mem-mini" [class.conflict]="m.status==='conflict'">
                  <div class="row"><span class="badge">{{ m.status }}</span><span class="badge">{{ m.memoryType }}</span><span class="muted small">{{ (m.confidence*100).toFixed(0) }}% • ★{{ m.importance }}</span></div>
                  <div class="small">{{ m.content }}</div>
                  <div class="muted small">{{ m.createdAt.substring(0,10) }} • {{ m.source }} • v{{ m.version }}
                    <button class="mini" (click)="inspectMemory(m.id)">Inspect</button>
                    @if (m.status!=='archived') { <button class="mini danger" (click)="archiveMemory(m.id)">Archive</button> } @else { <button class="mini" (click)="restoreMemory(m.id)">Restore</button> }
                  </div>
                </div>
              }
            } @else { <p class="muted">No memories for this agent. Create via Memories page or agent task.</p> }
            @if (selectedMemory()) {
              <div class="detail">
                <pre class="code">{{ stringify(selectedMemory()) }}</pre>
                @if (memoryVersions().length) {
                  <h4>History ({{ memoryVersions().length }})</h4>
                  @for (v of memoryVersions(); track v.id) { <div class="muted small">v{{ v.version }} {{ v.status }} — {{ v.reason }}: {{ v.content }}</div> }
                }
                <button class="mini" (click)="selectedMemory.set(null)">Close</button>
              </div>
            }
            <a routerLink="/memories" class="muted small">→ Full Memory console</a>
          </div>

          <div class="card">
            <h3>Activity</h3>
            <p class="muted">Agent runs are versioned and never mutate history. Runtime execution coming later — current version is reproducible.</p>
          </div>
        </div>

        <div class="edit">
          <h3>Update agent (creates new version)</h3>
          <input [(ngModel)]="editName" placeholder="Name" class="input" />
          <select [(ngModel)]="editAutonomy" class="input">
            <option value="assistant">assistant</option>
            <option value="task_executor">task_executor</option>
            <option value="collaborative">collaborative</option>
            <option value="autonomous">autonomous</option>
          </select>
          <select [(ngModel)]="editModel" class="input">
            @for (m of availableModels; track m) { <option [value]="m">{{ m }}</option> }
          </select>
          <p class="muted small">Model from OPENROUTER_MODEL or pick here. Saved as new version.</p>
          <button class="btn small" (click)="save()">Save (new version)</button>
          @if (saveError()) { <div class="err">{{ saveError() }}</div> }
        </div>
      } @else {
        <p class="muted">Loading agent…</p>
      }
    </div>
  `,
  styles: [`
    .detail { padding:24px; max-width:1100px; margin:0 auto; }
    .hero { display:flex; gap:16px; align-items:center; background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; }
    .avatar { font-size:32px; width:56px; height:56px; display:grid; place-items:center; background:#f7f8f9; border-radius:12px; }
    h1 { margin:0; font-size:20px; }
    .slug { font-size:12px; color:#6b7280; font-weight:400; }
    .purpose { color:#6b7280; font-size:13px; margin:4px 0; }
    .badges { display:flex; gap:6px; }
    .badge { background:#e8eaee; padding:4px 8px; border-radius:999px; font-size:11px; }
    .badge.dark { background:#111827; color:#fff; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; text-decoration:none; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .grid { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .row { display:flex; justify-content:space-between; font-size:13px; padding:4px 0; }
    .muted { color:#6b7280; font-size:12px; }
    .muted.small { font-size:11px; }
    .pill { padding:2px 6px; border-radius:999px; font-size:11px; background:#e8eaee; }
    .pill.on { background:#d1fae5; color:#065f46; }
    .code { background:#f7f8f9; padding:8px; border-radius:6px; font-size:11px; white-space:pre-wrap; }
    .attach-row { display:flex; gap:6px; margin-top:8px; }
    .mini { border:1px solid #e8eaee; background:#fff; border-radius:6px; padding:4px 8px; font-size:11px; cursor:pointer; }
    .mini.danger { color:#dc2626; border-color:#fecaca; }
    .edit { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; margin-top:16px; display:flex; gap:8px; flex-direction:column; }
    .input { padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .err { color:#dc2626; font-size:12px; }
    .mem-mini { background:#f9fafb; border:1px solid #e8eaee; border-radius:6px; padding:6px; margin:4px 0; }
    .mem-mini.conflict { border-color:#fbbf24; background:#fffbeb; }
    .detail { background:#f9fafb; border:1px solid #e8eaee; border-radius:6px; padding:8px; margin-top:6px; }
    @media(max-width:800px){ .grid{ grid-template-columns:1fr } .hero{ flex-direction:column; align-items:flex-start } }
  `],
})
export class AgentDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);
  private agentService = inject(AgentService);
  private knowledgeService = inject(KnowledgeService);
  private taskService = inject(TaskService);
  private memoryService = inject(MemoryService);
  private settings = inject(SettingsService);
  agent = this.agentService.selected;
  versions = signal<any[]>([]);
  agentKnowledge = signal<any[]>([]);
  availableDocs = signal<any[]>([]);
  agentTasks = signal<any[]>([]);
  agentRuns = signal<any[]>([]);
  agentMemories = signal<any[]>([]);
  selectedMemory = signal<any>(null);
  memoryVersions = signal<any[]>([]);
  attachDocId = '';
  editName = '';
  editAutonomy = 'assistant';
  editModel = 'minimax/minimax-m3:free';
  fallbackModels = ['minimax/minimax-m3:free','openai/gpt-4o-mini','openai/gpt-4o','anthropic/claude-3.5-sonnet','google/gemini-pro','google/gemini-1.5-flash','meta-llama/llama-3.1-70b'];
  get availableModels(): string[] {
    return this.settings.config()?.availableModels?.length ? this.settings.config()!.availableModels : this.fallbackModels;
  }
  saveError = signal('');

  ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id')!;
    this.settings.llm().subscribe();
    this.agentService.get(id).subscribe(res => {
      this.editName = res.data.name;
      this.editAutonomy = res.data.autonomyLevel;
      this.editModel = res.data.version?.modelConfiguration?.model || this.settings.config()?.defaultModel || 'minimax/minimax-m3:free';
    });
    this.agentService.versions(id).subscribe(res => this.versions.set(res.data || []));
    this.loadKnowledge(id);
    this.loadMemories(id);
    this.knowledgeService.listDocuments().subscribe(r=> this.availableDocs.set(r.data || []));
    this.taskService.list().subscribe(r=> {
      const all = (r as any).data || this.taskService.tasks();
      this.agentTasks.set(all.filter((t:any)=> t.assignedToAgent===id));
    });
    this.taskService.listRunsGlobal(id).subscribe(r=> this.agentRuns.set(r.data || []));
  }

  loadMemories(agentId:string){
    this.memoryService.list({ agentId }).subscribe(r=> this.agentMemories.set((r as any).data || []));
  }
  activeMemories = () => this.agentMemories().filter((m:any)=> m.status==='active');
  staleMemories = () => this.agentMemories().filter((m:any)=> ['stale','archived','superseded','conflict'].includes(m.status));
  inspectMemory(id:string){
    this.memoryService.get(id).subscribe((r:any)=>{
      const d = r.data || r;
      this.selectedMemory.set(d.memory);
      this.memoryVersions.set(d.versions || []);
    });
  }
  archiveMemory(id:string){
    this.memoryService.archive(id).subscribe(()=> this.loadMemories(this.route.snapshot.paramMap.get('id')!));
  }
  restoreMemory(id:string){
    this.memoryService.restore(id).subscribe(()=> this.loadMemories(this.route.snapshot.paramMap.get('id')!));
  }

  loadKnowledge(agentId:string){
    this.knowledgeService.listAgentKnowledge(agentId).subscribe(r=> this.agentKnowledge.set(r.data || []));
  }
  attachSelected(){
    const id = this.route.snapshot.paramMap.get('id')!;
    if (!this.attachDocId) return;
    this.knowledgeService.attachToAgent(id, this.attachDocId).subscribe(()=> { this.loadKnowledge(id); this.attachDocId=''; });
  }
  detachKnowledge(kid:string){
    const id = this.route.snapshot.paramMap.get('id')!;
    this.knowledgeService.detachAgentKnowledge(id, kid).subscribe(()=> this.loadKnowledge(id));
  }

  knowledgeText() {
    const v: any = this.agent()?.version;
    return v?.memoryPolicy?.sources?.join(', ') || JSON.stringify(v?.memoryPolicy || {}) || '—';
  }
  toolsText() {
    const v: any = this.agent()?.version;
    return v?.toolPolicy?.allowed_tools?.join(', ') || this.agent()?.version?.capabilities?.map((c:any)=>c.name).join(', ') || '—';
  }
  stringify(o:any){ return JSON.stringify(o, null, 2); }

  save(){
    const id = this.route.snapshot.paramMap.get('id')!;
    const version: any = { modelConfiguration: { model: this.editModel, temperature: 0.7 } };
    this.agentService.update(id, { name: this.editName, autonomyLevel: this.editAutonomy, version }).subscribe({
      next: ()=> this.agentService.get(id).subscribe(),
      error: e => this.saveError.set(e.error?.error?.message || 'Update failed')
    });
  }
}
