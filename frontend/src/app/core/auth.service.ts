import { Injectable, computed, signal, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap, map, catchError, of } from 'rxjs';
import { environment } from '../../environments/environment';
import { Router } from '@angular/router';

export interface User {
  id: string;
  email: string;
  displayName: string;
  avatarUrl?: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  avatarUrl?: string;
}

const TOKEN_KEY = 'openagent_token';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);
  private router = inject(Router);
  private _token = signal<string | null>(localStorage.getItem(TOKEN_KEY));
  private _user = signal<User | null>(null);
  private _org = signal<Organization | null>(null);
  private _orgs = signal<Organization[]>([]);
  private _loading = signal(false);

  token = this._token.asReadonly();
  user = this._user.asReadonly();
  org = this._org.asReadonly();
  orgs = this._orgs.asReadonly();
  loading = this._loading.asReadonly();
  isAuthenticated = computed(() => !!this._token());

  constructor() {
    if (this._token()) {
      this.fetchMe().subscribe();
    }
  }

  login(email: string, password: string) {
    this._loading.set(true);
    return this.http
      .post<{ data: { token: string; user: User; organization: Organization; organizations: Organization[] } }>(
        `${environment.apiBase}/auth/login`,
        { email, password },
      )
      .pipe(
        tap(res => {
          this.setToken(res.data.token);
          this._user.set(res.data.user);
          if (res.data.organization) this._org.set(res.data.organization);
          if (res.data.organizations) this._orgs.set(res.data.organizations);
          this._loading.set(false);
        }),
        catchError(err => { this._loading.set(false); throw err; }),
      );
  }

  register(email: string, displayName: string, password: string, orgName?: string) {
    this._loading.set(true);
    return this.http
      .post<{ data: { token: string; user: User; organization: Organization } }>(
        `${environment.apiBase}/auth/register`,
        { email, displayName, password, orgName },
      )
      .pipe(
        tap(res => {
          this.setToken(res.data.token);
          this._user.set(res.data.user);
          if (res.data.organization) {
            this._org.set(res.data.organization);
            this._orgs.set([res.data.organization]);
          }
          this._loading.set(false);
        }),
        catchError(err => { this._loading.set(false); throw err; }),
      );
  }

  fetchMe() {
    return this.http.get<{ data: { user: User; organization: Organization; organizations: Organization[] } }>(`${environment.apiBase}/auth/me`).pipe(
      tap(res => {
        this._user.set(res.data.user);
        if (res.data.organization) this._org.set(res.data.organization);
        if (res.data.organizations) this._orgs.set(res.data.organizations);
      }),
      map(() => true),
      catchError(() => {
        // token invalid -> logout
        if (this._token()) this.logout(false);
        return of(false);
      }),
    );
  }

  switchOrg(orgId: string) {
    return this.http.post<{ data: { token: string } }>(`${environment.apiBase}/auth/switch`, { organizationId: orgId }).pipe(
      tap(res => {
        this.setToken(res.data.token);
        // refetch me to update org
        this.fetchMe().subscribe();
      }),
    );
  }

  logout(navigate = true) {
    localStorage.removeItem(TOKEN_KEY);
    this._token.set(null);
    this._user.set(null);
    this._org.set(null);
    this._orgs.set([]);
    if (navigate) this.router.navigate(['/login']);
  }

  private setToken(token: string) {
    localStorage.setItem(TOKEN_KEY, token);
    this._token.set(token);
  }

  getToken(): string | null {
    return this._token();
  }
}
