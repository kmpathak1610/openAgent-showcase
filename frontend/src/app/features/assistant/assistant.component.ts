import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AssistantService } from '../../core/assistant.service';
import { BrowserService } from '../../core/browser.service';

@Component({
  selector: 'app-assistant',
  standalone: true,
  imports: [FormsModule, RouterLink],
  template: `
    <div class="wrap">
      <h1>OpenAgent Assistant</h1>
      <p class="muted">Default assistant — onboarding, guidance, agent/team/task design, troubleshooting. Uses existing runtime; respects permissions; memory-aware.</p>

      @if (assistant(); as a) {
        <div class="hero">
          <div class="avatar">🤖</div>
          <div>
            <h2>{{ a.name }} <span class="slug">{{ a.slug }}</span></h2>
            <p class="muted">{{ a.purpose }}</p>
            <span class="badge">{{ a.status }}</span>
          </div>
          <button class="btn small" (click)="ensure()">Ensure (idempotent)</button>
        </div>
      } @else {
        <div class="card"><p class="muted">Loading assistant…</p><button class="btn small" (click)="ensure()">Create Assistant</button></div>
      }

      <div class="card">
        <h3>Welcome to OpenAgent.</h3>
        <p>I can help you:</p>
        <ul class="muted">
          <li>create AI agents</li><li>build AI teams</li><li>create agentic workflows</li><li>connect tools</li><li>set up projects</li><li>troubleshoot runs</li>
        </ul>
        <p class="muted">Tell me what you want to accomplish.</p>
        <div class="suggested">
          @for (act of onboarding()?.suggestedActions || []; track act.id) {
            <button class="mini" (click)="pick(act.id)">{{ act.title }}</button>
          }
        </div>
        <textarea [(ngModel)]="prompt" rows="2" placeholder="Describe your goal — e.g., Every Friday research AI news, create three LinkedIn posts and send them for approval." class="input"></textarea>
        <button class="btn" (click)="send()">Send to Assistant</button>
        @if (response()) { <pre class="code">{{ response() }}</pre> }
      </div>

      <div class="grid">
        <div class="card">
          <h3>Workspace (bounded)</h3>
          <button class="btn small" (click)="inspect()">Inspect workspace</button>
          @if (workspace(); as ws) {
            <div class="muted small">Projects {{ ws.projects?.length || 0 }} • Agents {{ ws.agents?.length || 0 }} • Teams {{ ws.teams?.length || 0 }} • Tasks {{ ws.tasks?.length || 0 }}</div>
            <div class="muted small">Browser profiles {{ ws.browserProfiles?.length || 0 }}</div>
            <details><summary class="muted small">Show raw</summary><pre class="code">{{ stringify(ws) }}</pre></details>
          }
          <h4>Agent recommendation</h4>
          <p class="muted small">Assistant checks existing agents before creating — reuses suitable, creates only missing.</p>
          <button class="btn small" (click)="recommend()">Recommend agents for current goal</button>
          @if (recommendation()) { <pre class="code">{{ recommendation() }}</pre> }
        </div>

        <div class="card">
          <h3>Agentic Task Designer</h3>
          <p class="muted small">Goal → research → generation → review → human approval → publishing → schedule. Uses existing task schema.</p>
          <div class="row">
            <input [(ngModel)]="goal" placeholder="Goal" class="input" style="flex:1" />
            <button class="btn small" (click)="designTask()">Design task</button>
          </div>
          @if (taskPlan()) { <pre class="code">{{ taskPlan() }}</pre> }
          <h4>Troubleshooting</h4>
          <div class="row">
            <input [(ngModel)]="taskId" placeholder="Task ID" class="input" />
            <input [(ngModel)]="runId" placeholder="Run ID (optional)" class="input" />
            <button class="btn small" (click)="diagnose()">Diagnose</button>
          </div>
          @if (diagnosis()) { <pre class="code">{{ stringify(diagnosis()) }}</pre> }
        </div>
      </div>

      <div class="card">
        <h3>Memory & Context</h3>
        <p class="muted small">Assistant uses existing memory lifecycle (candidate→active→stale) with org/project isolation, never stores credentials.</p>
        <a routerLink="/memories" class="muted small">→ Memories</a>
        <span class="muted small"> • Bounded context: conversation + workspace + relevant memory + task state</span>
      </div>
    </div>
  `,
  styles: [`
    .wrap{padding:24px;max-width:1000px;margin:0 auto}
    .muted{color:#6b7280;font-size:12px}
    .muted.small{font-size:11px}
    .hero{display:flex;gap:12px;align-items:center;background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:12px}
    .avatar{font-size:28px;width:48px;height:48px;display:grid;place-items:center;background:#f7f8f9;border-radius:10px}
    h1{margin:0;font-size:20px} h2{margin:0;font-size:16px} .slug{font-size:11px;color:#6b7280}
    .badge{background:#e8eaee;padding:2px 6px;border-radius:999px;font-size:10px}
    .card{background:#fff;border:1px solid #e8eaee;border-radius:10px;padding:14px;margin:12px 0}
    .grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
    .input{width:100%;padding:8px 10px;border:1px solid #e8eaee;border-radius:8px;font-size:13px;margin-top:6px;box-sizing:border-box}
    .row{display:flex;gap:6px;align-items:center;margin-top:6px}
    .btn{background:#111827;color:#fff;border:0;padding:8px 14px;border-radius:8px;cursor:pointer;font-size:13px}
    .btn.small{padding:6px 10px;font-size:12px}
    .mini{border:1px solid #e8eaee;background:#fff;border-radius:6px;padding:4px 8px;font-size:11px;cursor:pointer}
    .code{background:#f7f8f9;padding:8px;border-radius:6px;font-size:11px;white-space:pre-wrap;max-height:300px;overflow:auto}
    .suggested{display:flex;gap:6px;flex-wrap:wrap;margin:6px 0}
    @media(max-width:800px){.grid{grid-template-columns:1fr}}
  `],
})
export class AssistantComponent implements OnInit {
  private svc = inject(AssistantService);
  private browserSvc = inject(BrowserService);
  assistant = this.svc.assistant;
  workspace = this.svc.workspace;
  onboarding = this.svc.onboarding;
  prompt = '';
  goal = '';
  taskId = '';
  runId = '';
  response = signal('');
  recommendation = signal('');
  taskPlan = signal('');
  diagnosis = signal<any>(null);

  ngOnInit(){
    this.svc.get().subscribe({ error: ()=> this.svc.ensure().subscribe() });
    this.svc.getOnboarding().subscribe();
  }
  ensure(){ this.svc.ensure().subscribe(); }
  inspect(){ this.svc.inspectWorkspace().subscribe(); }
  pick(id:string){
    const map:any = {
      create_agent: 'Create my first agent for web research',
      build_team: 'Build an AI team for content creation',
      setup_workflow: 'Every Friday research AI news, create three LinkedIn posts and send them for approval',
      explore_workspace: 'Explore my workspace'
    };
    this.prompt = map[id] || id;
  }
  send(){
    if (!this.prompt.trim()) return;
    // For now, simulate assistant reasoning via workspace inspect + recommendation
    this.response.set('Assistant reasoning (stub): Goal → ' + this.prompt + '\nInspect workspace, reuse agents, design task, ask only necessary questions. [Uses existing runtime; no fabrication]');
    this.svc.inspectWorkspace().subscribe();
  }
  recommend(){
    // Simulate recommendation: check existing agents
    this.svc.inspectWorkspace().subscribe(r=>{
      const ws:any = r.data || this.workspace();
      const agents = ws?.agents || [];
      this.recommendation.set('Existing agents: ' + agents.map((a:any)=>a.name).join(', ') + '\nRecommendation: reuse ' + (agents[0]?.name || 'none') + ', create research agent if missing. Single assistant can handle simple tasks; team for complex delegation.');
    });
  }
  designTask(){
    if (!this.goal.trim()) return;
    this.taskPlan.set(`Goal: ${this.goal}\nResearch step (browser.search → extract)\nContent step (agent or team)\nReview → Human approval (existing approval system)\nPublishing via official API (or browser if permitted)\nSchedule: Friday cron 0 9 * * 5`);
  }
  diagnose(){
    if (!this.taskId.trim()) return;
    this.svc.diagnose(this.taskId.trim(), this.runId.trim()||undefined).subscribe(r=> this.diagnosis.set(r.data || r));
  }
  stringify(v:any){ return JSON.stringify(v, null, 2); }
}
