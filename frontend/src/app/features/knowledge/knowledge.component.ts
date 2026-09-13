import { Component, inject, OnInit, signal, computed } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { KnowledgeService } from '../../core/knowledge.service';
import { ProjectService } from '../../core/project.service';
import { AgentService } from '../../core/agent.service';

@Component({
  selector: 'app-knowledge',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <div class="head">
        <div>
          <h1>Knowledge</h1>
          <p class="muted">Upload files, websites, or text. The system handles chunking, embeddings, and retrieval. No RAG expertise needed.</p>
        </div>
        <button class="btn" (click)="showAdd.set(!showAdd())">+ Add Knowledge</button>
      </div>

      @if (showAdd()) {
        <div class="add-panel">
          <div class="tabs">
            <button [class.active]="addTab()==='upload'" (click)="addTab.set('upload')">Upload files</button>
            <button [class.active]="addTab()==='website'" (click)="addTab.set('website')">Website</button>
            <button [class.active]="addTab()==='text'" (click)="addTab.set('text')">Text</button>
            <button [class.active]="addTab()==='integration'" (click)="addTab.set('integration')">Connect source</button>
          </div>

          @if (addTab()==='upload') {
            <input type="file" (change)="onFile($event)" multiple class="input" />
            <input [(ngModel)]="uploadTitle" placeholder="Title (optional, defaults to filename)" class="input" />
            <select [(ngModel)]="uploadScope" class="input">
              <option value="organization">Organization Knowledge</option>
              <option value="project">Project Knowledge</option>
            </select>
            @if (uploadScope==='project') {
              <select [(ngModel)]="uploadProjectId" class="input">
                <option value="">Select project</option>
                @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
              </select>
            }
            <button class="btn" (click)="doUpload()" [disabled]="uploading()">{{ uploading() ? 'Uploading…' : 'Upload' }}</button>
          }

          @if (addTab()==='website') {
            <input [(ngModel)]="websiteUrl" placeholder="https://example.com/page" class="input" />
            <input [(ngModel)]="websiteTitle" placeholder="Title" class="input" />
            <select [(ngModel)]="websiteScope" class="input">
              <option value="organization">Organization</option>
              <option value="project">Project</option>
            </select>
            @if (websiteScope==='project') {
              <select [(ngModel)]="websiteProjectId" class="input">
                <option value="">Select project</option>
                @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
              </select>
            }
            <button class="btn" (click)="doWebsite()" [disabled]="uploading()">{{ uploading() ? 'Fetching…' : 'Add website' }}</button>
          }

          @if (addTab()==='text') {
            <input [(ngModel)]="textTitle" placeholder="Title" class="input" />
            <textarea [(ngModel)]="textContent" rows="5" placeholder="Paste or write knowledge… Brand guidelines, product docs, campaign history, etc." class="input"></textarea>
            <select [(ngModel)]="textScope" class="input">
              <option value="organization">Organization</option>
              <option value="project">Project</option>
            </select>
            @if (textScope==='project') {
              <select [(ngModel)]="textProjectId" class="input">
                <option value="">Select project</option>
                @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
              </select>
            }
            <button class="btn" (click)="doText()" [disabled]="uploading()">{{ uploading() ? 'Saving…' : 'Add text' }}</button>
          }

          @if (addTab()==='integration') {
            <p class="muted">Connect Google Drive, Notion, Slack, GitHub — coming soon. For now use Upload/Website/Text.</p>
            <div class="coming">
              <span class="pill">Google Drive</span><span class="pill">Notion</span><span class="pill">Slack</span><span class="pill">GitHub</span>
            </div>
          }

          @if (addError()) { <div class="err">{{ addError() }}</div> }
          @if (addSuccess()) { <div class="ok">{{ addSuccess() }}</div> }
        </div>
      }

      <div class="filters">
        <button [class.active]="filterScope()===''" (click)="filterScope.set('')">All</button>
        <button [class.active]="filterScope()==='organization'" (click)="filterScope.set('organization')">Organization</button>
        <button [class.active]="filterScope()==='project'" (click)="filterScope.set('project')">Project</button>
        <button [class.active]="filterScope()==='agent'" (click)="filterScope.set('agent')">Agent</button>
        <select [(ngModel)]="filterProjectId" (change)="reload()" class="input small">
          <option value="">All projects</option>
          @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
        </select>
      </div>

      <div class="grid">
        <div class="card">
          <h3>Documents @if (knowledgeService.loading()) { <span class="muted">loading…</span> }</h3>
          @if (!docs().length) { <p class="muted">No documents yet. Add knowledge above.</p> }
          @for (d of docs(); track d.id) {
            <div class="doc-row" [class.processing]="d.status!=='ready'">
              <div class="doc-main">
                <strong>{{ d.title }}</strong>
                <span class="badge" [class.ready]="d.status==='ready'" [class.pending]="d.status!=='ready'">{{ d.status }}</span>
                <span class="muted small">{{ d.sourceType }} • {{ d.scope }} • {{ d.chunkCount || 0 }} chunks</span>
                <div class="muted small">{{ d.createdAt }}</div>
              </div>
              <div class="doc-actions">
                <button class="mini" (click)="viewDoc(d)">View</button>
                <button class="mini" (click)="attachDoc(d)">Attach</button>
                <button class="mini danger" (click)="deleteDoc(d.id)">Delete</button>
              </div>
            </div>
          }
        </div>

        <div class="col">
          <div class="card">
            <h3>Collections</h3>
            <div class="add-row">
              <input [(ngModel)]="newCollectionName" placeholder="Collection name" class="input" />
              <button class="btn small" (click)="createCollection()">Create</button>
            </div>
            @for (c of collections(); track c.id) { <div class="row"><span>{{ c.name }}</span><span class="muted">{{ c.slug }}</span></div> }
            @if (!collections().length) { <p class="muted">No collections. Group docs for agent/project.</p> }
          </div>

          <div class="card">
            <h3>Sources</h3>
            @for (s of sources(); track s.id) { <div class="row"><span>{{ s.name }}</span><span class="badge">{{ s.sourceType }}</span></div> }
            @if (!sources().length) { <p class="muted">No sources. Uploads auto-track.</p> }
          </div>

          <div class="card">
            <h3>Search preview</h3>
            <input [(ngModel)]="searchQuery" placeholder="Semantic search…" class="input" (keydown.enter)="doSearch()" />
            <button class="btn small" (click)="doSearch()" style="margin-top:6px">Search</button>
            @for (r of searchResults(); track r.chunk.id) {
              <div class="search-hit">
                <div class="muted small">{{ r.chunk.documentTitle || r.chunk.documentId }} — score {{ r.score }}</div>
                <div class="hit-body">{{ r.chunk.content.slice(0,180) }}…</div>
              </div>
            }
          </div>
        </div>
      </div>

      @if (selectedDoc()) {
        <div class="modal" (click)="selectedDoc.set(null)">
          <div class="modal-card" (click)="$event.stopPropagation()">
            <h3>{{ selectedDoc()!.title }}</h3>
            <p class="muted">{{ selectedDoc()!.sourceType }} • {{ selectedDoc()!.status }} • {{ selectedDoc()!.scope }}</p>
            <div class="chunks">
              @for (ch of selectedChunks(); track ch.id) {
                <div class="chunk"><div class="muted small">Chunk {{ ch.chunkIndex }} — {{ ch.tokenCount }} tokens</div><div class="body">{{ ch.content }}</div></div>
              }
              @if (!selectedChunks().length) { <p class="muted">No chunks yet — processing…</p> }
            </div>
            <button class="btn secondary" (click)="selectedDoc.set(null)">Close</button>
          </div>
        </div>
      }

      @if (attachDocTarget()) {
        <div class="modal" (click)="attachDocTarget.set(null)">
          <div class="modal-card" (click)="$event.stopPropagation()">
            <h3>Attach “{{ attachDocTarget()!.title }}”</h3>
            <div class="attach-grid">
              <div>
                <h4>To Project</h4>
                <select [(ngModel)]="attachProjectId" class="input">
                  <option value="">Select project</option>
                  @for (p of projects(); track p.id) { <option [value]="p.id">{{ p.name }}</option> }
                </select>
                <button class="btn small" (click)="doAttachProject()" style="margin-top:6px">Attach to project</button>
              </div>
              <div>
                <h4>To Agent</h4>
                <select [(ngModel)]="attachAgentId" class="input">
                  <option value="">Select agent</option>
                  @for (a of agents(); track a.id) { <option [value]="a.id">{{ a.name }}</option> }
                </select>
                <button class="btn small" (click)="doAttachAgent()" style="margin-top:6px">Attach to agent</button>
              </div>
            </div>
            <button class="btn secondary" (click)="attachDocTarget.set(null)" style="margin-top:10px">Done</button>
          </div>
        </div>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:1200px; margin:0 auto; }
    h1 { margin:0; }
    .muted { color:#6b7280; font-size:12px; }
    .head { display:flex; justify-content:space-between; align-items:flex-start; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn:disabled { opacity:.5; }
    .add-panel { background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; margin-top:12px; }
    .tabs { display:flex; gap:6px; margin-bottom:10px; }
    .tabs button { padding:6px 10px; border:1px solid #e8eaee; background:#f7f8f9; border-radius:999px; font-size:12px; cursor:pointer; }
    .tabs button.active { background:#111827; color:#fff; border-color:#111827; }
    .input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; margin-top:6px; box-sizing:border-box; }
    .input.small { width:auto; }
    .coming { display:flex; gap:6px; margin-top:8px; }
    .pill { background:#e8eaee; padding:4px 8px; border-radius:999px; font-size:11px; }
    .err { color:#dc2626; font-size:12px; margin-top:6px; }
    .ok { color:#065f46; font-size:12px; margin-top:6px; }
    .filters { display:flex; gap:6px; margin-top:16px; flex-wrap:wrap; }
    .filters button { padding:6px 10px; border:1px solid #e8eaee; background:#fff; border-radius:999px; font-size:12px; cursor:pointer; }
    .filters button.active { background:#111827; color:#fff; }
    .grid { display:grid; grid-template-columns:1.6fr .9fr; gap:16px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .col { display:flex; flex-direction:column; gap:16px; }
    .doc-row { display:flex; justify-content:space-between; padding:8px; border-bottom:1px solid #f7f8f9; }
    .doc-row.processing { opacity:.7; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; margin-left:6px; }
    .badge.ready { background:#d1fae5; color:#065f46; }
    .badge.pending { background:#fef3c7; color:#92400e; }
    .muted.small { font-size:11px; }
    .doc-actions { display:flex; gap:4px; }
    .mini { border:1px solid #e8eaee; background:#fff; border-radius:6px; padding:4px 8px; font-size:11px; cursor:pointer; }
    .mini.danger { color:#dc2626; border-color:#fecaca; }
    .row { display:flex; justify-content:space-between; font-size:13px; padding:6px 0; border-bottom:1px solid #f7f8f9; }
    .search-hit { background:#f7f8f9; padding:8px; border-radius:6px; margin:6px 0; }
    .hit-body { font-size:12px; white-space:pre-wrap; }
    .add-row { display:flex; gap:6px; margin-top:8px; }
    .modal { position:fixed; inset:0; background:rgba(0,0,0,.4); display:grid; place-items:center; z-index:30; }
    .modal-card { background:#fff; border-radius:12px; padding:16px; width: min(800px, 90vw); max-height:80vh; overflow:auto; }
    .chunks { max-height:400px; overflow:auto; }
    .chunk { background:#f7f8f9; padding:8px; border-radius:6px; margin:6px 0; }
    .body { font-size:13px; white-space:pre-wrap; }
    .attach-grid { display:grid; grid-template-columns:1fr 1fr; gap:12px; margin-top:8px; }
    @media(max-width:900px){ .grid{ grid-template-columns:1fr } .attach-grid{ grid-template-columns:1fr } }
  `],
})
export class KnowledgeComponent implements OnInit {
  private ks = inject(KnowledgeService);
  private ps = inject(ProjectService);
  private as = inject(AgentService);

  showAdd = signal(false);
  addTab = signal<'upload'|'website'|'text'|'integration'>('upload');
  uploading = signal(false);
  addError = signal('');
  addSuccess = signal('');

  // upload
  uploadTitle = '';
  uploadScope: string = 'organization';
  uploadProjectId = '';
  fileList: FileList | null = null;

  websiteUrl = '';
  websiteTitle = '';
  websiteScope = 'organization';
  websiteProjectId = '';

  textTitle = '';
  textContent = '';
  textScope = 'organization';
  textProjectId = '';

  newCollectionName = '';

  filterScope = signal('');
  filterProjectId = '';
  docs = this.ks.documents;
  collections = this.ks.collections;
  sources = this.ks.sources;
  projects = this.ps.projects;
  agents = this.as.agents;

  searchQuery = '';
  searchResults = signal<any[]>([]);
  selectedDoc = signal<any|null>(null);
  selectedChunks = signal<any[]>([]);
  attachDocTarget = signal<any|null>(null);
  attachProjectId = '';
  attachAgentId = '';

  knowledgeService = this.ks;

  ngOnInit(){
    this.ps.list().subscribe();
    this.as.list().subscribe();
    this.ks.listSources().subscribe();
    this.ks.listCollections().subscribe();
    this.reload();
    // poll for processing status
    setInterval(()=> { if (this.docs().some(d=> d.status!=='ready' && d.status!=='archived')) this.reload(); }, 3000);
  }

  reload(){
    this.ks.listDocuments(this.filterProjectId || undefined, this.filterScope() || undefined).subscribe();
  }

  onFile(e: Event){ this.fileList = (e.target as HTMLInputElement).files; }

  doUpload(){
    if (!this.fileList || !this.fileList.length) { this.addError.set('Select a file'); return; }
    this.uploading.set(true);
    this.addError.set(''); this.addSuccess.set('');
    const file = this.fileList[0];
    const title = this.uploadTitle || file.name;
    this.ks.createDocument({ title, sourceType:'upload', scope: this.uploadScope, projectId: this.uploadProjectId || undefined, file }).subscribe({
      next: ()=> { this.uploading.set(false); this.addSuccess.set('Uploaded — processing in background'); this.reload(); },
      error: e=> { this.uploading.set(false); this.addError.set(e.error?.error?.message || 'Upload failed'); }
    });
  }

  doWebsite(){
    if (!this.websiteUrl.trim()) { this.addError.set('URL required'); return; }
    this.uploading.set(true);
    this.ks.createDocument({ title: this.websiteTitle || this.websiteUrl, sourceType:'website', sourceUrl: this.websiteUrl, scope: this.websiteScope, projectId: this.websiteProjectId || undefined }).subscribe({
      next: ()=> { this.uploading.set(false); this.addSuccess.set('Website queued — extracting & embedding'); this.reload(); },
      error: e=> { this.uploading.set(false); this.addError.set(e.error?.error?.message || 'Failed'); }
    });
  }

  doText(){
    if (!this.textTitle.trim() || !this.textContent.trim()) { this.addError.set('Title and content required'); return; }
    this.uploading.set(true);
    this.ks.createDocument({ title: this.textTitle, sourceType:'text', content: this.textContent, scope: this.textScope, projectId: this.textProjectId || undefined }).subscribe({
      next: ()=> { this.uploading.set(false); this.addSuccess.set('Text saved — chunking'); this.reload(); this.textTitle=''; this.textContent=''; },
      error: e=> { this.uploading.set(false); this.addError.set(e.error?.error?.message || 'Failed'); }
    });
  }

  createCollection(){
    if (!this.newCollectionName.trim()) return;
    this.ks.createCollection(this.newCollectionName.trim()).subscribe(()=> this.newCollectionName='');
  }

  viewDoc(d:any){
    this.selectedDoc.set(d);
    this.ks.getChunks(d.id).subscribe(r=> this.selectedChunks.set(r.data || []));
  }

  deleteDoc(id:string){
    if (!confirm('Delete document?')) return;
    this.ks.deleteDocument(id).subscribe(()=> this.reload());
  }

  attachDoc(d:any){ this.attachDocTarget.set(d); }

  doAttachProject(){
    const doc = this.attachDocTarget();
    if (!doc || !this.attachProjectId) return;
    this.ks.attachToProject(this.attachProjectId, doc.id).subscribe(()=> { this.attachDocTarget.set(null); this.attachProjectId=''; });
  }
  doAttachAgent(){
    const doc = this.attachDocTarget();
    if (!doc || !this.attachAgentId) return;
    this.ks.attachToAgent(this.attachAgentId, doc.id).subscribe(()=> { this.attachDocTarget.set(null); this.attachAgentId=''; });
  }

  doSearch(){
    if (!this.searchQuery.trim()) return;
    this.ks.search(this.searchQuery.trim(), { limit:5, hybrid: true }).subscribe(r=> this.searchResults.set(r.data || []));
  }
}
