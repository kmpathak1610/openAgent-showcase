import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { BrowserService } from '../../core/browser.service';

@Component({
  selector: 'app-browser',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <h1>Browser Profiles</h1>
      <p class="muted">Persistent browser identities for legitimate web research and permitted workflows. Cookies/storage retained securely; never exposed to LLM. Organization/user isolated.</p>

      <div class="card">
        <h3>Create Profile</h3>
        <div class="row">
          <select [(ngModel)]="provider" class="input">
            <option value="generic">generic</option>
            <option value="linkedin">linkedin</option>
            <option value="github">github</option>
            <option value="notion">notion</option>
            <option value="google">google</option>
          </select>
          <input [(ngModel)]="name" placeholder="Name — e.g. LinkedIn Research" class="input" style="flex:1" />
          <button class="btn" (click)="create()" [disabled]="!name.trim() || creating()">Create</button>
        </div>
        <p class="muted small">Actual login happens in browser session — AI never receives credentials. StorageKey is internal hash, never a path.</p>
        @if (error()) { <div class="err">{{ error() }}</div> }
      </div>

      <div class="grid">
        <div class="card">
          <h3>Profiles ({{ profiles().length }})</h3>
          @for (p of profiles(); track p.id) {
            <div class="item" [class.connected]="p.status==='connected'">
              <div class="head"><span class="badge">{{ p.provider }}</span> <span class="badge" [class.on]="p.status==='connected'">{{ p.status }}</span></div>
              <div class="body">{{ p.name }}</div>
              <div class="muted small">{{ p.id.slice(0,8) }} • {{ p.createdAt.substring(0,10) }} • last {{ p.lastUsedAt?.substring(0,10) || 'never' }}</div>
              <div class="row">
                <button class="btn small" (click)="startSession(p.id)">Start session</button>
                <button class="mini" (click)="setStatus(p.id,'disconnected')">Disconnect</button>
                <button class="mini danger" (click)="remove(p.id)">Delete</button>
              </div>
              <div class="muted small">Safe state: profile_id={{ p.id.slice(0,8) }}, provider={{ p.provider }}, status={{ p.status }} — no cookies/tokens exposed</div>
            </div>
          }
          @if (!profiles().length) { <p class="muted">No profiles. Create one to start authenticated research.</p> }
        </div>

        <div class="card">
          <h3>Active Sessions</h3>
          @for (s of sessions(); track s.id) {
            <div class="item">
              <div class="head"><span class="badge">{{ s.status }}</span> <span class="muted small">{{ s.id.slice(0,8) }}</span></div>
              <div class="muted small">Profile {{ s.browserProfileId.slice(0,8) }} • started {{ s.startedAt.substring(0,19) }} • expires {{ s.expiresAt.substring(0,19) }}</div>
              <button class="mini danger" (click)="close(s.id)">Close</button>
            </div>
          }
          @if (!sessions().length) { <p class="muted">No active sessions. Sessions auto-expire in 30m, orphan reaped in 10m.</p> }
          <h3 style="margin-top:12px">Audit (last 50)</h3>
          <button class="btn small" (click)="loadAudit()">Refresh audit</button>
          @for (a of audit(); track a.id) {
            <div class="muted small">{{ a.createdAt?.substring(0,19) }} • {{ a.toolName }} • {{ a.action }} • {{ a.domain || a.target }} • {{ a.resultStatus }}</div>
          }
        </div>
      </div>

      <div class="card">
        <h3>Web Research Workflow (agent)</h3>
        <p class="muted small">browser.search → result selection → browser.navigate → browser.extract → normalization → agent reasoning → cite sources</p>
        <p class="muted small">Agents use existing tool framework; browser tools are <b>low</b> (search/navigate/extract/screenshot) and <b>medium</b> (click/type/download). Publishing via official API, not browser bypass.</p>
      </div>
    </div>
  `,
  styles: [`
    .wrap{padding:24px;max-width:1000px;margin:0 auto}
    .muted{color:#6b7280;font-size:12px}
    .muted.small{font-size:11px}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:14px;margin:12px 0}
    .grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
    .input{padding:8px 10px;border:1px solid #e8eaee;border-radius:8px;font-size:13px}
    .row{display:flex;gap:8px;align-items:center;margin-top:6px;flex-wrap:wrap}
    .btn{background:#111827;color:#fff;border:0;padding:8px 14px;border-radius:8px;cursor:pointer;font-size:13px}
    .btn.small{padding:6px 10px;font-size:12px}
    .btn:disabled{opacity:.5}
    .mini{border:1px solid #e8eaee;background:#fff;border-radius:6px;padding:4px 8px;font-size:11px;cursor:pointer}
    .mini.danger{color:#dc2626;border-color:#fecaca}
    .badge{background:#e8eaee;padding:2px 6px;border-radius:999px;font-size:10px}
    .badge.on{background:#d1fae5;color:#065f46}
    .item{background:#f9fafb;border:1px solid #e8eaee;border-radius:8px;padding:8px;margin:6px 0}
    .item.connected{border-color:#6ee7b7;background:#ecfdf5}
    .head{display:flex;gap:6px;align-items:center}
    .body{font-size:13px;margin:4px 0}
    .err{color:#dc2626;font-size:12px}
    @media(max-width:800px){.grid{grid-template-columns:1fr}}
  `],
})
export class BrowserComponent implements OnInit {
  private svc = inject(BrowserService);
  profiles = this.svc.profiles;
  sessions = this.svc.sessions;
  provider = 'generic';
  name = '';
  audit = signal<any[]>([]);
  error = signal('');
  creating = signal(false);

  ngOnInit(){ this.reload(); }
  reload(){ this.svc.listProfiles().subscribe(); this.svc.listSessions().subscribe(); }
  create(){
    if (!this.name.trim()) return;
    this.creating.set(true);
    this.error.set('');
    this.svc.createProfile(this.provider, this.name.trim()).subscribe({
      next: ()=>{this.creating.set(false); this.name=''; this.reload();},
      error: e=>{this.creating.set(false); this.error.set(e.error?.error?.message || 'Failed');}
    });
  }
  remove(id:string){ this.svc.deleteProfile(id).subscribe(()=> this.reload()); }
  setStatus(id:string,s:string){ this.svc.updateStatus(id,s).subscribe(()=> this.reload()); }
  startSession(pid:string){ this.svc.createSession(pid).subscribe(()=> this.reload()); }
  close(id:string){ this.svc.closeSession(id).subscribe(()=> this.reload()); }
  loadAudit(){ this.svc.audit().subscribe(r=> this.audit.set(r.data || [])); }
}
