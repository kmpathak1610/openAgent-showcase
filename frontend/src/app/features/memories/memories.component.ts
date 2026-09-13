import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MemoryService, Memory } from '../../core/memory.service';
import { AgentService } from '../../core/agent.service';
import { ProjectService } from '../../core/project.service';

@Component({
  selector: 'app-memories',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <h1>Memory</h1>
      <p class="muted">Lifecycle: candidate → validating → active → merged/updated → stale → archived. Consolidated, scope-safe, traceable.</p>

      <div class="card">
        <h3>Create Memory (consolidation-aware)</h3>
        <div class="grid">
          <select [(ngModel)]="memoryType" class="input">
            <option value="working">Working Memory (task/session)</option>
            <option value="project">Project Memory</option>
            <option value="agent">Agent Memory</option>
            <option value="conversation">Conversation Memory</option>
            <option value="task">Task Memory</option>
          </select>
          <select [(ngModel)]="scope" class="input">
            <option value="task">task</option>
            <option value="session">session</option>
            <option value="project">project</option>
            <option value="agent">agent</option>
            <option value="conversation">conversation</option>
            <option value="organization">organization</option>
          </select>
        </div>
        <select [(ngModel)]="projectId" class="input" *ngIf="scope==='project' || memoryType==='project'">
          <option value="">Select project (for project scope)</option>
          @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
        </select>
        <select [(ngModel)]="agentId" class="input" *ngIf="scope==='agent' || memoryType==='agent'">
          <option value="">Select agent (for agent scope)</option>
          @for (a of agents(); track a.id) { <option [value]="a.id">{{ a.name }}</option> }
        </select>
        <textarea [(ngModel)]="content" rows="3" placeholder="Content — e.g., Customer prefers CSV reports. (low-value like 'hello' will be rejected)" class="input"></textarea>
        <div class="row">
          <label>Importance: {{ importance }}</label>
          <input type="range" min="0" max="1" step="0.1" [(ngModel)]="importance" />
          <span class="muted">{{ importance }}</span>
          <label style="margin-left:12px">Confidence: {{ confidence }}</label>
          <input type="range" min="0" max="1" step="0.05" [(ngModel)]="confidence" />
          <span class="muted">{{ confidence }}</span>
        </div>
        <div class="row">
          <select [(ngModel)]="source" class="input small">
            <option value="manual">manual</option>
            <option value="EXPLICIT_USER_PREFERENCE">EXPLICIT_USER_PREFERENCE (0.95)</option>
            <option value="USER_MESSAGE">USER_MESSAGE (0.8)</option>
            <option value="AGENT_OBSERVATION">AGENT_OBSERVATION (0.5)</option>
            <option value="TASK_RESULT">TASK_RESULT</option>
            <option value="DOCUMENT">DOCUMENT</option>
          </select>
          <span class="muted small">Explicit preference → high confidence, survives consolidation.</span>
        </div>
        <p class="muted small">Conversation &lt;0.3 rejected • Working 1h TTL • Conversation 7d • Duplicates merged • Contradictions preserved • Secrets rejected</p>
        <button class="btn" (click)="create()" [disabled]="creating()">{{ creating() ? 'Saving…' : 'Save Memory (via Consolidator)' }}</button>
        @if (lastResult()) { <div class="result">Result: <b>{{ lastResult() }}</b> — {{ lastReason() }}</div> }
        @if (error()) { <div class="err">{{ error() }}</div> }
      </div>

      <div class="filters">
        <select [(ngModel)]="filterType" (change)="reload()" class="input small">
          <option value="">All types</option>
          <option value="working">working</option>
          <option value="project">project</option>
          <option value="agent">agent</option>
          <option value="conversation">conversation</option>
          <option value="task">task</option>
        </select>
        <select [(ngModel)]="filterScope" (change)="reload()" class="input small">
          <option value="">All scopes</option>
          <option value="task">task</option>
          <option value="project">project</option>
          <option value="agent">agent</option>
          <option value="conversation">conversation</option>
          <option value="organization">organization</option>
        </select>
        <select [(ngModel)]="filterStatus" (change)="reload()" class="input small">
          <option value="">Active only</option>
          <option value="active">active</option>
          <option value="conflict">conflict</option>
          <option value="stale">stale</option>
          <option value="archived">archived</option>
          <option value="superseded">superseded</option>
          <option value="candidate">candidate</option>
        </select>
        <label class="muted small"><input type="checkbox" [(ngModel)]="showAll" (change)="reload()"> show all (incl. archived)</label>
        <button class="btn small" (click)="reload()">Reload</button>
        <button class="btn small secondary" (click)="markStale()">Mark stale (admin)</button>
      </div>

      <div class="list">
        @for (m of memories(); track m.id) {
          <div class="mem" [class.archived]="m.status==='archived'" [class.conflict]="m.status==='conflict'" [class.stale]="m.status==='stale'">
            <div class="head">
              <span class="badge">{{ m.memoryType }}</span>
              <span class="badge">{{ m.scope }}</span>
              <span class="badge" [class.active]="m.status==='active'" [class.warn]="m.status==='conflict'">{{ m.status }}</span>
              <span class="importance">★{{ m.importance }}</span>
              <span class="confidence">conf {{ (m.confidence*100).toFixed(0) }}%</span>
              <span class="version">v{{ m.version }}</span>
            </div>
            <div class="body">{{ m.content }}</div>
            <div class="meta muted small">
              <span>source: {{ m.source }}</span>
              <span>• created {{ m.createdAt }}</span>
              <span>• confirmed {{ m.lastConfirmedAt || m.updatedAt }}</span>
              <span>• expires {{ m.expiresAt || 'never' }}</span>
              @if (m.conversationId) { <span>• conv {{ m.conversationId!.slice(0,8) }}</span> }
              @if (m.taskId) { <span>• task {{ m.taskId!.slice(0,8) }}</span> }
            </div>
            <div class="actions">
              <button class="mini" (click)="viewDetail(m)">Inspect</button>
              @if (m.status !== 'archived') { <button class="mini danger" (click)="archive(m.id)">Archive</button> }
              @else { <button class="mini" (click)="restore(m.id)">Restore</button> }
              <button class="mini danger" (click)="remove(m.id)">Hard Delete</button>
            </div>
            @if (selectedId() === m.id && selectedDetail()) {
              <div class="detail">
                <div class="detail-grid">
                  <div><b>ID:</b> {{ m.id }}</div>
                  <div><b>ContentHash:</b> {{ m.contentHash ? m.contentHash.substring(0,8) : '—' }}</div>
                  <div><b>Status:</b> {{ m.status }}</div>
                  <div><b>Confidence:</b> {{ m.confidence }}</div>
                  <div><b>Importance:</b> {{ m.importance }}</div>
                  <div><b>Version:</b> {{ m.version }}</div>
                  <div><b>Scope:</b> {{ m.scope }} / {{ m.memoryType }}</div>
                  <div><b>Source:</b> {{ m.source }} @ {{ m.sourceEventId || '—' }}</div>
                </div>
                @if (m.metadata) { <pre class="code">{{ stringify(m.metadata) }}</pre> }
                @if (versions().length) {
                  <h4>Version history ({{ versions().length }})</h4>
                  @for (v of versions(); track v.id) {
                    <div class="ver"><span>v{{ v.version }} • {{ v.status }} • {{ v.createdAt.substring(0,19) }} — {{ v.reason }}</span><div class="muted small">{{ v.content }}</div></div>
                  }
                }
                <button class="mini" (click)="selectedId.set('')">Close</button>
              </div>
            }
          </div>
        }
        @if (!memories().length) { <p class="muted">No memories. Try creating one: "Customer prefers CSV reports." → then repeat with same meaning to see MERGE, then change to PDF to see CONFLICT.</p> }
      </div>

      <div class="card">
        <h3>Search Memories (relevance, ACTIVE only)</h3>
        <input [(ngModel)]="query" placeholder="Search query — e.g., What format should I use for customer reports?" class="input" />
        <button class="btn small" (click)="search()">Search</button>
        @for (m of searchResults(); track m.id) {
          <div class="mem"><div class="body">{{ m.content }}</div><div class="muted small">§ {{ m.memoryType }}/{{ m.scope }} • conf {{ (m.confidence*100).toFixed(0) }}% • ★{{ m.importance }} • {{ m.status }}</div></div>
        }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:960px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .muted.small { font-size:11px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; margin:12px 0; }
    .grid { display:grid; grid-template-columns:1fr 1fr; gap:8px; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; margin-top:6px; box-sizing:border-box; }
    .input.small { width:auto; }
    .row { display:flex; gap:8px; align-items:center; margin-top:6px; flex-wrap:wrap; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn:disabled { opacity:.5; }
    .err { color:#dc2626; font-size:12px; margin-top:6px; }
    .result { background:#d1fae5; border:1px solid #6ee7b7; padding:6px 8px; border-radius:6px; font-size:12px; margin-top:6px; }
    .filters { display:flex; gap:6px; margin-top:12px; flex-wrap:wrap; align-items:center; }
    .mem { background:#fff; border:1px solid #e8eaee; border-radius:8px; padding:10px; margin:6px 0; }
    .mem.archived { opacity:0.6; background:#f9fafb; }
    .mem.conflict { border-color:#fbbf24; background:#fffbeb; }
    .mem.stale { border-color:#9ca3af; background:#f3f4f6; }
    .head { display:flex; gap:6px; align-items:center; flex-wrap:wrap; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .badge.active { background:#d1fae5; color:#065f46; }
    .badge.warn { background:#fef3c7; color:#92400e; }
    .importance { color:#d97706; font-size:11px; }
    .confidence { color:#2563eb; font-size:11px; }
    .version { color:#6b7280; font-size:11px; }
    .body { font-size:13px; margin:6px 0; white-space:pre-wrap; }
    .meta { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
    .actions { display:flex; gap:6px; margin-top:6px; }
    .mini { border:1px solid #e8eaee; background:#fff; border-radius:6px; padding:4px 8px; font-size:11px; cursor:pointer; }
    .mini.danger { color:#dc2626; border-color:#fecaca; }
    .detail { background:#f9fafb; border:1px solid #e8eaee; border-radius:6px; padding:8px; margin-top:8px; }
    .detail-grid { display:grid; grid-template-columns:1fr 1fr; gap:4px; font-size:11px; }
    .code { background:#111827; color:#e5e7eb; padding:8px; border-radius:6px; font-size:11px; white-space:pre-wrap; overflow:auto; }
    .ver { background:#fff; border:1px solid #e8eaee; border-radius:6px; padding:6px; margin:4px 0; font-size:11px; }
  `],
})
export class MemoriesComponent implements OnInit {
  private memSvc = inject(MemoryService);
  private agentSvc = inject(AgentService);
  private projectSvc = inject(ProjectService);
  memories = this.memSvc.memories;
  agents = this.agentSvc.agents;
  projects = this.projectSvc.projects;
  memoryType: string = 'agent';
  scope: string = 'agent';
  projectId = '';
  agentId = '';
  content = '';
  importance = 0.5;
  confidence = 0.5;
  source = 'manual';
  creating = signal(false);
  error = signal('');
  lastResult = signal('');
  lastReason = signal('');
  filterType = '';
  filterScope = '';
  filterStatus = '';
  showAll = false;
  query = '';
  searchResults = signal<any[]>([]);
  selectedId = signal('');
  selectedDetail = signal<any>(null);
  versions = signal<any[]>([]);

  ngOnInit(){
    this.agentSvc.list().subscribe();
    this.projectSvc.list().subscribe();
    this.reload();
  }
  reload(){
    const filters: any = {};
    if (this.filterType) filters.memoryType = this.filterType;
    if (this.filterScope) filters.scope = this.filterScope;
    if (this.filterStatus) filters.status = this.filterStatus;
    if (this.showAll) filters.status = this.filterStatus || undefined;
    // if showAll, we actually want to list all including archived; backend uses all=true
    if (this.showAll) {
      this.memSvc.listAll(filters).subscribe();
    } else {
      this.memSvc.list(filters).subscribe();
    }
  }
  create(){
    if (!this.content.trim()) { this.error.set('Content required'); return; }
    this.creating.set(true);
    this.error.set('');
    this.lastResult.set('');
    this.memSvc.create({
      memoryType: this.memoryType,
      scope: this.scope,
      content: this.content,
      importance: this.importance,
      confidence: this.confidence,
      source: this.source,
      agentId: this.agentId || undefined,
      projectId: this.projectId || undefined,
    }).subscribe({
      next: (res:any)=> {
        this.creating.set(false);
        const d = res.data || res;
        this.lastResult.set(d.result || 'CREATED');
        this.lastReason.set(d.reason || 'ok');
        this.content='';
        this.reload();
      },
      error: e=> { this.creating.set(false); this.error.set(e.error?.error?.message || e.error?.message || 'Failed: ' + JSON.stringify(e.error)); }
    });
  }
  remove(id:string){ this.memSvc.remove(id).subscribe(()=> this.reload()); }
  archive(id:string){ this.memSvc.archive(id).subscribe(()=> this.reload()); }
  restore(id:string){ this.memSvc.restore(id).subscribe(()=> this.reload()); }
  markStale(){ /* admin placeholder */ this.reload(); }
  viewDetail(m: Memory){
    if (this.selectedId() === m.id) { this.selectedId.set(''); return; }
    this.selectedId.set(m.id);
    this.memSvc.get(m.id).subscribe({
      next: (r:any)=> {
        const data = r.data || r;
        this.selectedDetail.set(data.memory);
        this.versions.set(data.versions || []);
      },
      error: ()=> this.versions.set([])
    });
  }
  search(){
    if (!this.query.trim()) return;
    this.memSvc.search(this.query).subscribe(r=> this.searchResults.set(r.data || []));
  }
  stringify(v:any){ return JSON.stringify(v, null, 2); }
}
