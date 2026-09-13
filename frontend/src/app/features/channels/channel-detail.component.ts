import { Component, inject, signal, OnInit, OnDestroy, ViewChild, ElementRef, AfterViewChecked } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { ChannelService } from '../../core/channel.service';
import { MessageService, Message } from '../../core/message.service';
import { AuthService } from '../../core/auth.service';
import { WsService } from '../../core/ws.service';

@Component({
  selector: 'app-channel-detail',
  standalone: true,
  imports: [FormsModule, DatePipe],
  template: `
    <div class="channel-detail" [class.with-thread]="threadId()">
      <div class="main-pane">
        <div class="ch-header">
          <div>
            <div class="ch-name"># {{ channel()?.name || 'loading…' }}</div>
            @if (channel()?.topic) { <div class="ch-topic">{{ channel()?.topic }}</div> }
            @else { <div class="ch-topic muted">{{ channel()?.description || 'No topic' }}</div> }
          </div>
          <div class="ch-actions">
            <button class="icon-btn" (click)="renaming.set(!renaming())" title="Rename">✎</button>
            <button class="icon-btn" (click)="archiving()" title="Archive">🗑</button>
          </div>
        </div>

        @if (renaming()) {
          <div class="rename-bar">
            <input [(ngModel)]="newName" placeholder="new-channel-name" class="input" />
            <button class="btn small" (click)="doRename()">Rename</button>
          </div>
        }

        <div class="messages" #scrollEl (scroll)="onScroll($event)">
          @if (messageService.loading() && !messageService.messages().length) {
            <div class="loading">Loading messages…</div>
          }
          @if (messageService.hasMore()) {
            <button class="load-more" (click)="loadMore()">Load earlier messages</button>
          }
          @for (m of messageService.messages(); track m.id) {
            <div class="msg" [class.own]="isOwn(m)" [class.deleted]="!!m.deletedAt">
              <div class="avatar">{{ avatarFor(m) }}</div>
              <div class="msg-body">
                <div class="msg-head">
                  <span class="author">{{ authorFor(m) }}</span>
                  <span class="time">{{ m.createdAt | date:'short' }}</span>
                  @if (m.editedAt) { <span class="edited">(edited)</span> }
                  @if (m.threadCount) { <button class="thread-badge" (click)="openThread(m.id)">{{ m.threadCount }} replies</button> }
                </div>
                @if (editingId()===m.id) {
                  <textarea [(ngModel)]="editBody" class="edit-input" rows="2"></textarea>
                  <div class="edit-actions">
                    <button class="btn small" (click)="saveEdit(m.id)">Save</button>
                    <button class="btn small secondary" (click)="editingId.set(null)">Cancel</button>
                  </div>
                } @else {
                  <div class="body" [innerHTML]="renderBody(m.body)"></div>
                }
                @if (!m.deletedAt) {
                  <div class="msg-actions">
                    <button class="mini" (click)="openThread(m.id)" title="Reply">↳ Reply</button>
                    @if (isOwn(m)) {
                      <button class="mini" (click)="startEdit(m)" title="Edit">Edit</button>
                      <button class="mini danger" (click)="deleteMsg(m.id)" title="Delete">Delete</button>
                    }
                    <span class="reactions">
                      @for (emoji of quickEmojis; track emoji) {
                        <button class="emoji" [class.active]="hasReacted(m, emoji)" (click)="toggleReaction(m, emoji)">{{ emoji }}</button>
                      }
                    </span>
                    @if (m.reactions?.length) {
                      <span class="reaction-list">
                        @for (r of groupedReactions(m); track r.emoji) {
                          <span class="reaction-chip" (click)="toggleReaction(m, r.emoji)">{{ r.emoji }} {{ r.count }}</span>
                        }
                      </span>
                    }
                  </div>
                }
              </div>
            </div>
          }
          @if (!messageService.messages().length && !messageService.loading()) {
            <div class="empty">No messages yet. Start the conversation — @mentions are highlighted, threads keep discussions tidy.</div>
          }
        </div>

        <div class="composer">
          <textarea
            [(ngModel)]="composerBody"
            placeholder="Message #{{ channel()?.name }} — use @name to mention, Shift+Enter for new line"
            rows="3"
            (keydown)="onComposerKey($event)"
            class="composer-input"
          ></textarea>
          <div class="composer-bar">
            <span class="hint">Enter to send • Shift+Enter for new line</span>
            <button class="btn" (click)="send()" [disabled]="!composerBody.trim()">Send</button>
          </div>
        </div>
      </div>

      @if (threadId()) {
        <div class="thread-pane">
          <div class="thread-head">
            <strong>Thread</strong>
            <button class="icon-btn" (click)="threadId.set(null)">✕</button>
          </div>
          <div class="thread-messages">
            @for (m of threadMessages(); track m.id) {
              <div class="msg small">
                <div class="avatar">{{ avatarFor(m) }}</div>
                <div class="msg-body">
                  <div class="msg-head"><span class="author">{{ authorFor(m) }}</span><span class="time">{{ m.createdAt | date:'shortTime' }}</span></div>
                  <div class="body">{{ m.body }}</div>
                </div>
              </div>
            }
            @if (!threadMessages().length) { <div class="empty">No replies yet.</div> }
          </div>
          <div class="composer small">
            <textarea [(ngModel)]="threadBody" placeholder="Reply…" rows="2" class="composer-input" (keydown)="onThreadKey($event)"></textarea>
            <button class="btn small" (click)="sendThread()" [disabled]="!threadBody.trim()">Reply</button>
          </div>
        </div>
      }
    </div>
  `,
  styles: [`
    .channel-detail { display:flex; height:calc(100vh - 56px); background:#fff; }
    .channel-detail.with-thread .main-pane { flex:1.2; }
    .main-pane { flex:1; display:flex; flex-direction:column; min-width:0; }
    .ch-header { display:flex; align-items:center; justify-content:space-between; padding:12px 16px; border-bottom:1px solid #e8eaee; }
    .ch-name { font-weight:700; font-size:15px; }
    .ch-topic { font-size:12px; color:#6b7280; }
    .muted { color:#9aa0b2; }
    .ch-actions { display:flex; gap:6px; }
    .icon-btn { background:transparent; border:1px solid #e8eaee; border-radius:6px; padding:4px 8px; cursor:pointer; font-size:12px; }
    .rename-bar { display:flex; gap:8px; padding:8px 16px; background:#f7f8f9; border-bottom:1px solid #e8eaee; }
    .input { flex:1; padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .btn { background:#111827; color:#fff; border:0; padding:8px 14px; border-radius:8px; cursor:pointer; font-size:13px; }
    .btn.small { padding:6px 10px; font-size:12px; }
    .btn.secondary { background:#e8eaee; color:#111827; }
    .btn:disabled { opacity:0.5; cursor:not-allowed; }
    .messages { flex:1; overflow:auto; padding:12px 16px; display:flex; flex-direction:column; gap:10px; }
    .loading, .empty { text-align:center; color:#6b7280; font-size:13px; padding:20px; }
    .load-more { align-self:center; background:#f7f8f9; border:1px solid #e8eaee; border-radius:999px; padding:6px 12px; font-size:11px; cursor:pointer; }
    .msg { display:flex; gap:10px; padding:8px; border-radius:8px; }
    .msg:hover { background:#f7f8f9; }
    .msg.own { background:#f0f7ff; }
    .msg.deleted { opacity:0.5; }
    .avatar { width:36px; height:36px; border-radius:50%; background:#e8eaee; display:grid; place-items:center; font-weight:700; font-size:12px; flex-shrink:0; }
    .msg-body { flex:1; min-width:0; }
    .msg-head { display:flex; align-items:center; gap:8px; font-size:12px; }
    .author { font-weight:600; }
    .time { color:#9aa0b2; font-size:11px; }
    .edited { color:#9aa0b2; font-size:11px; }
    .thread-badge { background:#e0e7ff; color:#3730a3; border:0; border-radius:999px; padding:2px 8px; font-size:11px; cursor:pointer; }
    .body { font-size:14px; line-height:1.4; white-space:pre-wrap; word-break:break-word; }
    .body :global(.mention) { background:#e0e7ff; color:#3730a3; padding:0 4px; border-radius:4px; font-weight:600; }
    .msg-actions { display:flex; align-items:center; gap:6px; margin-top:4px; flex-wrap:wrap; }
    .mini { background:transparent; border:1px solid #e8eaee; border-radius:6px; padding:2px 6px; font-size:11px; cursor:pointer; }
    .mini.danger { color:#dc2626; border-color:#fecaca; }
    .reactions { display:flex; gap:4px; }
    .emoji { background:#fff; border:1px solid #e8eaee; border-radius:999px; padding:2px 6px; font-size:12px; cursor:pointer; }
    .emoji.active { background:#111827; color:#fff; border-color:#111827; }
    .reaction-list { display:flex; gap:4px; flex-wrap:wrap; }
    .reaction-chip { background:#f7f8f9; border:1px solid #e8eaee; border-radius:999px; padding:2px 6px; font-size:11px; cursor:pointer; }
    .edit-input { width:100%; padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:13px; }
    .edit-actions { display:flex; gap:6px; margin-top:6px; }
    .composer { border-top:1px solid #e8eaee; padding:10px 16px; background:#fff; }
    .composer.small { padding:8px; }
    .composer-input { width:100%; padding:8px 10px; border:1px solid #e8eaee; border-radius:10px; font-size:13px; resize:vertical; font-family:inherit; }
    .composer-bar { display:flex; align-items:center; justify-content:space-between; margin-top:6px; }
    .hint { font-size:11px; color:#9aa0b2; }
    .thread-pane { width:380px; border-left:1px solid #e8eaee; display:flex; flex-direction:column; background:#f7f8f9; }
    .thread-head { display:flex; align-items:center; justify-content:space-between; padding:10px 12px; background:#fff; border-bottom:1px solid #e8eaee; }
    .thread-messages { flex:1; overflow:auto; padding:10px; display:flex; flex-direction:column; gap:8px; }
    .msg.small .avatar { width:28px; height:28px; font-size:10px; }
    @media (max-width: 900px) {
      .channel-detail { flex-direction:column; }
      .thread-pane { width:100%; height:50%; }
    }
  `],
})
export class ChannelDetailComponent implements OnInit, OnDestroy, AfterViewChecked {
  private route = inject(ActivatedRoute);
  channelService = inject(ChannelService);
  messageService = inject(MessageService);
  private auth = inject(AuthService);
  private ws = inject(WsService);

  @ViewChild('scrollEl') scrollEl?: ElementRef<HTMLDivElement>;

  channel = this.channelService.selected;
  renaming = signal(false);
  newName = '';
  editingId = signal<string | null>(null);
  editBody = '';
  composerBody = '';
  threadId = signal<string | null>(null);
  threadMessages = signal<Message[]>([]);
  threadBody = '';
  quickEmojis = ['👍', '❤️', '😂', '🎉', '👀'];
  private channelId: string | null = null;
  private shouldScroll = false;

  ngOnInit() {
    this.route.paramMap.subscribe(params => {
      const id = params.get('id');
      if (!id) return;
      // Leave previous channel if any
      if (this.channelId) this.ws.leaveChannel(this.channelId);
      this.channelId = id;
      this.channelService.select(id);
      this.messageService.clear();
      this.messageService.list(id).subscribe();
      this.ws.joinChannel(id);
    });
  }

  ngOnDestroy() {
    if (this.channelId) this.ws.leaveChannel(this.channelId);
    this.messageService.clear();
  }

  ngAfterViewChecked() {
    if (this.shouldScroll && this.scrollEl) {
      this.scrollEl.nativeElement.scrollTop = this.scrollEl.nativeElement.scrollHeight;
      this.shouldScroll = false;
    }
  }

  isOwn(m: Message) {
    return m.senderUserId === this.auth.user()?.id;
  }

  avatarFor(m: Message) {
    const name = m.sender?.displayName || m.sender?.email || 'U';
    return name.slice(0, 1).toUpperCase();
  }

  authorFor(m: Message) {
    return m.sender?.displayName || m.sender?.email || 'Unknown';
  }

  renderBody(body: string) {
    // simple mention highlight: @word
    const escaped = body.replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return escaped.replace(/@(\w+)/g, '<span class="mention">@$1</span>');
  }

  hasReacted(m: Message, emoji: string) {
    return !!m.reactions?.find(r => r.emoji === emoji && r.userId === this.auth.user()?.id);
  }

  groupedReactions(m: Message) {
    const map = new Map<string, number>();
    for (const r of m.reactions || []) map.set(r.emoji, (map.get(r.emoji) || 0) + 1);
    return Array.from(map.entries()).map(([emoji, count]) => ({ emoji, count }));
  }

  onScroll(e: Event) {
    const el = e.target as HTMLElement;
    if (el.scrollTop < 40) this.loadMore();
  }

  loadMore() {
    if (this.channelId) this.messageService.loadMore(this.channelId);
  }

  send() {
    const body = this.composerBody.trim();
    if (!body || !this.channelId) return;
    this.messageService.send(this.channelId, body).subscribe(() => {
      this.composerBody = '';
      this.shouldScroll = true;
    });
  }

  onComposerKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      this.send();
    }
  }

  startEdit(m: Message) {
    this.editingId.set(m.id);
    this.editBody = m.body;
  }

  saveEdit(id: string) {
    if (!this.editBody.trim()) return;
    this.messageService.edit(id, this.editBody.trim()).subscribe(() => this.editingId.set(null));
  }

  deleteMsg(id: string) {
    if (!confirm('Delete this message?')) return;
    this.messageService.remove(id).subscribe();
  }

  toggleReaction(m: Message, emoji: string) {
    if (this.hasReacted(m, emoji)) this.messageService.unreact(m.id, emoji).subscribe();
    else this.messageService.react(m.id, emoji).subscribe();
  }

  openThread(id: string) {
    this.threadId.set(id);
    this.messageService.replies(id).subscribe(res => this.threadMessages.set(res.data || []));
  }

  sendThread() {
    const tid = this.threadId();
    if (!tid || !this.channelId || !this.threadBody.trim()) return;
    this.messageService.send(this.channelId, this.threadBody.trim(), tid).subscribe(res => {
      this.threadMessages.update(arr => [...arr, res.data]);
      this.threadBody = '';
    });
  }

  onThreadKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      this.sendThread();
    }
  }

  doRename() {
    const id = this.channelId;
    if (!id || !this.newName.trim()) return;
    this.channelService.rename(id, this.newName.trim()).subscribe(() => {
      this.renaming.set(false);
      this.newName = '';
    });
  }

  archiving() {
    if (!this.channelId || !confirm('Archive this channel?')) return;
    this.channelService.archive(this.channelId).subscribe(() => {
      // navigate away
      location.href = '/channels';
    });
  }
}
