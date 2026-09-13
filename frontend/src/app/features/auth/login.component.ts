import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { AuthService } from '../../core/auth.service';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [FormsModule, RouterLink],
  template: `
    <div class="wrap">
      <div class="card">
        <h1>OpenAgent</h1>
        <p class="muted">Sign in to your workspace</p>
        <input [(ngModel)]="email" placeholder="Email" class="input" />
        <input [(ngModel)]="password" type="password" placeholder="Password" class="input" />
        <button class="btn" (click)="login()" [disabled]="loading()"> {{ loading() ? 'Signing in...' : 'Sign in' }}</button>
        @if (error()) { <div class="err">{{ error() }}</div> }
        <p class="muted" style="margin-top:12px">No account? <a routerLink="/register">Create workspace</a></p>
      </div>
    </div>
  `,
  styles: [`
    .wrap{display:grid;place-items:center;min-height:100vh;background:#f7f8f9;font-family:Inter,system-ui}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:16px;padding:24px;width:360px}
    h1{margin:0;font-size:20px} .muted{color:#6b7280;font-size:13px}
    .input{width:100%;padding:10px 12px;border:1px solid #e8eaee;border-radius:10px;margin-top:10px;box-sizing:border-box}
    .btn{width:100%;margin-top:14px;background:#111827;color:#fff;border:0;padding:10px;border-radius:10px;cursor:pointer}
    .err{color:#dc2626;font-size:13px;margin-top:8px}
  `],
})
export class LoginComponent {
  private auth = inject(AuthService);
  private router = inject(Router);
  email = '';
  password = '';
  loading = signal(false);
  error = signal('');

  login() {
    this.loading.set(true);
    this.error.set('');
    this.auth.login(this.email, this.password).subscribe({
      next: () => {
        this.loading.set(false);
        this.router.navigate(['/']);
      },
      error: (e) => {
        this.loading.set(false);
        this.error.set(e.error?.error?.message || 'Login failed');
      },
    });
  }
}
