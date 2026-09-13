import { Component, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { ChannelService } from '../../core/channel.service';

@Component({
  selector: 'app-channels',
  standalone: true,
  imports: [RouterLink],
  template: `
    <div class="welcome">
      <h2>Channels</h2>
      <p class="muted">Create a channel to start collaborating. Channels can be organization-wide or tied to a project. Private channels enforce membership.</p>
      @if (channels().length) {
        <div class="grid">
          @for (c of channels(); track c.id) {
            <a class="card" [routerLink]="['/channels', c.id]">
              <div class="name"># {{ c.name }}</div>
              <div class="desc">{{ c.description || 'No description' }}</div>
              <div class="meta">{{ c.channelType }} • {{ c.projectId ? 'project' : 'org' }}</div>
            </a>
          }
        </div>
      } @else {
        <p class="muted">No channels yet. Use the + button in the sidebar to create one.</p>
      }
    </div>
  `,
  styles: [`
    .welcome { padding:24px; }
    h2 { margin:0 0 8px; }
    .muted { color:#6b7280; font-size:13px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(240px,1fr)); gap:12px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; text-decoration:none; color:inherit; display:block; }
    .card:hover { border-color:#111827; }
    .name { font-weight:600; }
    .desc { font-size:12px; color:#6b7280; margin-top:4px; }
    .meta { font-size:11px; color:#9aa0b2; margin-top:6px; }
  `],
})
export class ChannelsComponent implements OnInit {
  private channelService = inject(ChannelService);
  channels = this.channelService.channels;
  ngOnInit() {
    this.channelService.list().subscribe();
  }
}
