import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { AuthService } from '../../core/auth.service';

@Component({
  selector: 'app-register',
  standalone: true,
  imports: [FormsModule, RouterLink],
  template: `
    <div class="wrap">
      <div class="card">
        <h1>Create workspace</h1>
        <p class="muted">Start collaborating with humans + AI agents</p>
        <input [(ngModel)]="displayName" placeholder="Full name" class="input" />
        <input [(ngModel)]="email" placeholder="Email" class="input" />
        <input [(ngModel)]="password" type="password" placeholder="Password (min 8 chars)" class="input" />
        <input [(ngModel)]="orgName" placeholder="Workspace name (optional)" class="input" />
        <button class="btn" (click)="register()" [disabled]="loading()">{{ loading() ? 'Creating…' : 'Create workspace' }}</button>
        @if (error()) { <div class="err">{{ error() }}</div> }
        <p class="muted" style="margin-top:12px">Already have an account? <a routerLink="/login">Sign in</a></p>
      </div>
    </div>
  `,
  styles: [`
    .wrap{display:grid;place-items:center;min-height:100vh;background:#f7f8f9}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:16px;padding:24px;width:380px}
    h1{margin:0;font-size:20px} .muted{color:#6b7280;font-size:13px}
    .input{width:100%;padding:10px 12px;border:1px solid #e8eaee;border-radius:10px;margin-top:10px;box-sizing:border-box}
    .btn{width:100%;margin-top:14px;background:#111827;color:#fff;border:0;padding:10px;border-radius:10px;cursor:pointer}
    .btn:disabled{opacity:.6}
    .err{color:#dc2626;font-size:13px;margin-top:8px}
    a{color:#111827}
  `],
})
export class RegisterComponent {
  private auth = inject(AuthService);
  private router = inject(Router);
  email = '';
  displayName = '';
  password = '';
  orgName = '';
  loading = signal(false);
  error = signal('');

  register() {
    this.loading.set(true);
    this.error.set('');
    this.auth.register(this.email, this.displayName, this.password, this.orgName).subscribe({
      next: () => { this.loading.set(false); this.router.navigate(['/']); },
      error: (e) => { this.loading.set(false); this.error.set(e.error?.error?.message || 'Registration failed'); },
    });
  }
}
