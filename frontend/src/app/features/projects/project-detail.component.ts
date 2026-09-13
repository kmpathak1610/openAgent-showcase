import { Component, inject, signal, OnInit } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { ProjectService } from '../../core/project.service';
import { ChannelService } from '../../core/channel.service';
import { ApiService } from '../../core/api.service';

@Component({
  selector: 'app-project-detail',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      @if (project()) {
        <div class="hero">
          <div class="icon">{{ project()!.icon }}</div>
          <div>
            <h1>{{ project()!.name }}</h1>
            <p class="muted">{{ project()!.description || 'No description' }}</p>
            @if (project()!.objective) { <p><strong>Objective:</strong> {{ project()!.objective }}</p> }
          </div>
          <button class="btn danger" (click)="archive()">Archive</button>
        </div>

        <div class="tabs">
          <div class="tab">
            <h3>Channels</h3>
            @if (channels().length) {
              @for (c of channels(); track c.id) { <div class="row"># {{ c.name }} — {{ c.description }}</div> }
            } @else { <p class="muted">No channels in this project. Create one from the sidebar.</p> }
          </div>
          <div class="tab">
            <h3>Members</h3>
            <div class="member-list">
              @for (m of members(); track m.id) {
                <div class="row">{{ m.user?.displayName || m.user?.email || m.userId }} — {{ m.role }}</div>
              }
            </div>
            <div class="add-member">
              <input [(ngModel)]="newMemberEmail" placeholder="user email" class="input" />
              <button class="btn small" (click)="addMember()">Add</button>
            </div>
            <h3 style="margin-top:16px;">Knowledge <span class="badge">Soon</span></h3>
            <p class="muted">Documents & AI knowledge will be attached to projects.</p>
            <h3>Activity</h3>
            <p class="muted">Project activity feed coming next.</p>
          </div>
        </div>

        <div class="edit">
          <h3>Edit project</h3>
          <input [(ngModel)]="editName" placeholder="Name" class="input" />
          <input [(ngModel)]="editDescription" placeholder="Description" class="input" />
          <input [(ngModel)]="editObjective" placeholder="Objective" class="input" />
          <button class="btn small" (click)="save()">Save</button>
        </div>
      } @else {
        <p class="muted">Loading…</p>
      }
    </div>
  `,
  styles: [`
    .wrap { padding:24px; }
    .hero { display:flex; gap:16px; align-items:flex-start; background:#fff; border:1px solid #e8eaee; border-radius:12px; padding:16px; }
    .icon { font-size:32px; }
    h1 { margin:0; font-size:20px; }
    .muted { color:#6b7280; font-size:13px; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn.danger { background:#dc2626; margin-left:auto; }
    .tabs { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-top:16px; }
    .tab { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .row { padding:6px 0; font-size:13px; border-bottom:1px solid #f7f8f9; }
    .add-member { display:flex; gap:8px; margin-top:8px; }
    .input { flex:1; padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .edit { margin-top:16px; background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; display:flex; flex-direction:column; gap:8px; }
    .badge { background:#e8eaee; font-size:10px; padding:2px 6px; border-radius:999px; }
    @media(max-width:800px){ .tabs{grid-template-columns:1fr} }
  `],
})
export class ProjectDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);
  private projectService = inject(ProjectService);
  private channelService = inject(ChannelService);
  private api = inject(ApiService);

  project = this.projectService.selected;
  channels = signal<any[]>([]);
  members = signal<any[]>([]);
  editName = '';
  editDescription = '';
  editObjective = '';
  newMemberEmail = '';

  ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id')!;
    this.projectService.get(id).subscribe(res => {
      this.editName = res.data.name;
      this.editDescription = res.data.description;
      this.editObjective = res.data.objective;
    });
    this.channelService.list(id).subscribe(res => this.channels.set(res.data || []));
    this.projectService.members(id).subscribe(res => this.members.set(res.data || []));
  }

  save() {
    const id = this.route.snapshot.paramMap.get('id')!;
    this.projectService.update(id, { name: this.editName, description: this.editDescription, objective: this.editObjective }).subscribe();
  }

  archive() {
    if (!confirm('Archive this project?')) return;
    const id = this.route.snapshot.paramMap.get('id')!;
    this.projectService.archive(id).subscribe(() => history.back());
  }

  addMember() {
    const id = this.route.snapshot.paramMap.get('id')!;
    if (!this.newMemberEmail.trim()) return;
    this.projectService.addMember(id, this.newMemberEmail.trim()).subscribe(() => {
      this.newMemberEmail = '';
      this.projectService.members(id).subscribe(res => this.members.set(res.data || []));
    });
  }
}
