import { Component, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ToolService } from '../../core/tool.service';
import { AgentService } from '../../core/agent.service';

@Component({
  selector: 'app-tools',
  standalone: true,
  imports: [FormsModule],
  template: `
    <div class="wrap">
      <h1>Tools</h1>
      <p class="muted">Generic tool abstraction — agents request tools via structured arguments, server validates schemas and enforces permissions.</p>

      <div class="grid">
        @for (tool of tools(); track tool.name) {
          <div class="card" [class.disabled]="!tool.enabled">
            <div class="head">
              <strong>{{ tool.name }}</strong>
              <span class="risk" [class]="tool.riskLevel">{{ tool.riskLevel }}</span>
            </div>
            <p class="muted">{{ tool.description }}</p>
            <div class="meta">
              <span class="badge">{{ tool.enabled ? 'enabled' : 'disabled' }}</span>
            </div>
            <div class="assign">
              <select [(ngModel)]="assignAgent[tool.name]" class="input small">
                <option value="">Assign to agent…</option>
                @for (a of agents(); track a.id) { <option [value]="a.id">🤖 {{ a.name }}</option> }
              </select>
              <button class="btn small" (click)="assign(tool.name)">Assign</button>
            </div>
            <details class="exec">
              <summary>Execute (test)</summary>
              <textarea [(ngModel)]="execInput[tool.name]" placeholder='{"query":"test"}' rows="2" class="input"></textarea>
              <button class="btn small" (click)="execute(tool.name)">Execute</button>
              @if (execResult[tool.name]) { <pre class="code">{{ execResult[tool.name] }}</pre> }
            </details>
          </div>
        }
      </div>

      <div class="card" style="margin-top:16px">
        <h3>Recent Executions</h3>
        <button class="btn small" (click)="reloadExec()">Refresh</button>
        @for (ex of executions(); track ex.id) {
          <div class="row"><span>{{ ex.toolName }} — {{ ex.status }}</span><span class="muted">{{ ex.id.slice(0,8) }} • {{ ex.createdAt }}</span></div>
        }
        @if (!executions().length) { <p class="muted">No executions yet. Use Execute above or trigger via agent task.</p> }
      </div>
    </div>
  `,
  styles: [`
    .wrap { padding:24px; max-width:1100px; margin:0 auto; }
    .muted { color:#6b7280; font-size:12px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:12px; margin-top:16px; }
    .card { background:#fff; border:1px solid #e8eaee; border-radius:10px; padding:14px; }
    .card.disabled { opacity:.6; }
    .head { display:flex; justify-content:space-between; align-items:center; }
    .risk { padding:2px 6px; border-radius:999px; font-size:10px; text-transform:capitalize; }
    .risk.low { background:#d1fae5; color:#065f46; }
    .risk.medium { background:#fef3c7; color:#92400e; }
    .risk.high { background:#fee2e2; color:#dc2626; }
    .risk.critical { background:#dc2626; color:#fff; }
    .meta { margin:6px 0; }
    .badge { background:#e8eaee; padding:2px 6px; border-radius:999px; font-size:10px; }
    .assign { display:flex; gap:6px; margin-top:8px; }
    .input.small { flex:1; padding:6px 8px; border:1px solid #e8eaee; border-radius:6px; font-size:12px; }
    .btn.small { background:#111827; color:#fff; border:0; padding:6px 10px; border-radius:6px; font-size:12px; cursor:pointer; }
    .exec { margin-top:8px; }
    .code { background:#f7f8f9; padding:6px; border-radius:6px; font-size:11px; white-space:pre-wrap; margin-top:6px; }
    .row { display:flex; justify-content:space-between; font-size:12px; padding:4px 0; border-bottom:1px solid #f7f8f9; }
  `],
})
export class ToolsComponent implements OnInit {
  private toolService = inject(ToolService);
  private agentService = inject(AgentService);
  tools = this.toolService.tools;
  executions = this.toolService.executions;
  agents = this.agentService.agents;
  assignAgent: Record<string,string> = {};
  execInput: Record<string,string> = {};
  execResult: Record<string,string> = {};

  ngOnInit(){
    this.toolService.list().subscribe();
    this.agentService.list().subscribe();
    this.reloadExec();
  }
  reloadExec(){ this.toolService.listExecutions().subscribe(); }
  assign(name:string){
    const agentId = this.assignAgent[name];
    if (!agentId) return;
    this.toolService.assignToAgent(agentId, name).subscribe(()=> alert(`Assigned ${name} to agent`));
  }
  execute(name:string){
    let input:any = {};
    try { input = this.execInput[name] ? JSON.parse(this.execInput[name]) : {}; } catch { input = {}; }
    // pick first agent for demo
    const agentId = this.agents()[0]?.id;
    this.toolService.execute(name, input, agentId).subscribe({
      next: res => this.execResult[name] = JSON.stringify(res.data, null, 2),
      error: e => this.execResult[name] = JSON.stringify(e.error?.error || e, null, 2)
    });
  }
}
