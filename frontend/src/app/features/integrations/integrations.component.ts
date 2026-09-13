import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { IntegrationService } from '../../core/integration.service';

@Component({
  selector: 'app-integrations',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <h1>Integrations</h1>
      <p class="muted">Provider abstraction — never store raw secrets in agent config. Credentials are encrypted at rest.</p>

      <div class="card">
        <h3>Connect integration</h3>
        <div class="form">
          <input [(ngModel)]="name" placeholder="Name (e.g., My LinkedIn)" class="input" />
          <select [(ngModel)]="provider" class="input">
            <option value="">Select provider</option>
            <option value="linkedin">LinkedIn</option>
            <option value="x">X</option>
            <option value="instagram">Instagram</option>
            <option value="facebook">Facebook</option>
            <option value="email">Email</option>
            <option value="google_drive">Google Drive</option>
            <option value="slack">Slack</option>
            <option value="calendar">Calendar</option>
            <option value="http_api">Generic HTTP API</option>
            <option value="custom">Custom</option>
          </select>
          <textarea [(ngModel)]="credentialsJson" rows="3" placeholder='Credentials JSON e.g. {"apiKey":"...","token":"..."} — will be encrypted' class="input"></textarea>
          <button class="btn" (click)="create()" [disabled]="creating()">{{ creating() ? 'Connecting…' : 'Connect' }}</button>
          @if (error()) { <div class="err">{{ error() }}</div> }
        </div>
      </div>

      <div class="list">
        @for (integ of integrations(); track integ.id) {
          <div class="card">
            <div class="head">
              <strong>{{ integ.name }}</strong>
              <span class="badge">{{ integ.provider }}</span>
              <span class="badge" [class]="integ.status">{{ integ.status }}</span>
            </div>
            <p class="muted">ID {{ integ.id.slice(0,8) }} • {{ integ.createdAt }}</p>
            <button class="btn small danger" (click)="remove(integ.id)">Disconnect</button>
          </div>
        }
        @if (!integrations().length) { <p class="muted">No integrations — connect LinkedIn, X, etc. Credentials are isolated per organization.</p> }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:900px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; margin:12px 0; }
    .form { display:flex; flex-direction:column; gap:8px; margin-top:8px; }
    .input { padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn.danger { background:#fee2e2; color:#dc2626; }
    .head { display:flex; gap:6px; align-items:center; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .badge.connected { background:#d1fae5; color:#065f46; }
    .badge.disconnected { background:#e8eaee; }
    .badge.error { background:#fee2e2; color:#dc2626; }
    .err { color:#dc2626; font-size:12px; }
    .list { margin-top:12px; }
  `],
})
export class IntegrationsComponent implements OnInit {
  private svc = inject(IntegrationService);
  integrations = this.svc.integrations;
  name = '';
  provider = '';
  credentialsJson = '';
  creating = signal(false);
  error = signal('');

  ngOnInit(){ this.svc.list().subscribe(); }

  create(){
    if (!this.name || !this.provider) { this.error.set('Name and provider required'); return; }
    let creds:any = {};
    if (this.credentialsJson.trim()) {
      try { creds = JSON.parse(this.credentialsJson); } catch { this.error.set('Credentials must be valid JSON'); return; }
    }
    this.creating.set(true);
    this.error.set('');
    this.svc.create(this.name, this.provider, {}, creds).subscribe({
      next: ()=> { this.creating.set(false); this.name=''; this.provider=''; this.credentialsJson=''; },
      error: e=> { this.creating.set(false); this.error.set(e.error?.error?.message || 'Failed'); }
    });
  }

  remove(id:string){
    if (!confirm('Disconnect integration?')) return;
    this.svc.remove(id).subscribe();
  }
}
