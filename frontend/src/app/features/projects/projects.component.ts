import { Component, inject, signal, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { ProjectService } from '../../core/project.service';

@Component({
  selector: 'app-projects',
  standalone: true,
  imports: [RouterLink, FormsModule],
  template: `
    <div class="wrap">
      <div class="head">
        <h2>Projects</h2>
        <button class="btn" (click)="showCreate.set(!showCreate())">New project</button>
      </div>

      @if (showCreate()) {
        <div class="create">
          <input [(ngModel)]="name" placeholder="Project name" class="input" />
          <input [(ngModel)]="description" placeholder="Description" class="input" />
          <input [(ngModel)]="objective" placeholder="Objective — what will this project achieve?" class="input" />
          <div class="row">
            <button class="btn" (click)="create()">Create</button>
            <button class="btn secondary" (click)="showCreate.set(false)">Cancel</button>
          </div>
        </div>
      }

      @if (projectService.loading()) { <p class="muted">Loading…</p> }

      <div class="grid">
        @for (p of projectService.projects(); track p.id) {
          <a class="card" [routerLink]="['/projects', p.id]">
            <div class="card-head">
              <span class="icon">{{ p.icon }}</span>
              <span class="name">{{ p.name }}</span>
              <span class="status">{{ p.status }}</span>
            </div>
            <div class="desc">{{ p.description || 'No description' }}</div>
            @if (p.objective) { <div class="obj"><strong>Objective:</strong> {{ p.objective }}</div> }
            <div class="meta">Slug: {{ p.slug }}</div>
          </a>
        }
      </div>

      @if (!projectService.projects().length && !projectService.loading()) {
        <p class="muted">No projects yet. Create one to group channels, tasks, and knowledge.</p>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; }
    .head { display:flex; align-items:center; justify-content:space-between; }
    h2 { margin:0; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .create { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:12px; display:flex; flex-direction:column; gap:8px; margin:12px 0; }
    .input { padding:8px 10px; border:1px solid #e8eaee; border-radius:8px; font-size:13px; }
    .row { display:flex; gap:8px; }
    .muted { color:#6b7280; font-size:13px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:12px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; text-decoration:none; color:inherit; display:block; }
    .card:hover { border-color:#111827; }
    .card-head { display:flex; align-items:center; gap:8px; }
    .icon { font-size:18px; }
    .name { font-weight:700; flex:1; }
    .status { font-size:10px; background:#e8eaee; padding:2px 6px; border-radius:999px; }
    .desc { font-size:12px; color:#6b7280; margin-top:6px; }
    .obj { font-size:12px; margin-top:6px; background:#f7f8f9; padding:6px; border-radius:6px; }
    .meta { font-size:11px; color:#9aa0b2; margin-top:8px; }
  `],
})
export class ProjectsComponent implements OnInit {
  projectService = inject(ProjectService);
  showCreate = signal(false);
  name = '';
  description = '';
  objective = '';

  ngOnInit() { this.projectService.list().subscribe(); }

  create() {
    if (!this.name.trim()) return;
    this.projectService.create({ name: this.name.trim(), description: this.description.trim(), objective: this.objective.trim() }).subscribe(() => {
      this.name = ''; this.description = ''; this.objective = ''; this.showCreate.set(false);
    });
  }
}
