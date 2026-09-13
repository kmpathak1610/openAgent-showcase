import { Routes } from '@angular/router';
import { ShellComponent } from './layout/shell.component';
import { authGuard } from './core/auth.guard';

export const routes: Routes = [
  {
    path: '',
    component: ShellComponent,
    canActivate: [authGuard],
    children: [
      { path: '', loadComponent: () => import('./features/dashboard/dashboard.component').then(m => m.DashboardComponent) },
      { path: 'projects', loadComponent: () => import('./features/projects/projects.component').then(m => m.ProjectsComponent) },
      { path: 'projects/:id', loadComponent: () => import('./features/projects/project-detail.component').then(m => m.ProjectDetailComponent) },
      { path: 'channels', loadComponent: () => import('./features/channels/channels.component').then(m => m.ChannelsComponent) },
      { path: 'channels/:id', loadComponent: () => import('./features/channels/channel-detail.component').then(m => m.ChannelDetailComponent) },
      { path: 'agents/builder', loadComponent: () => import('./features/agents/builder/builder.component').then(m => m.BuilderComponent) },
      { path: 'agents/:id', loadComponent: () => import('./features/agents/agent-detail.component').then(m => m.AgentDetailComponent) },
      { path: 'agents', loadComponent: () => import('./features/agents/agents.component').then(m => m.AgentsComponent) },
      { path: 'knowledge', loadComponent: () => import('./features/knowledge/knowledge.component').then(m => m.KnowledgeComponent) },
      { path: 'tasks/:id', loadComponent: () => import('./features/tasks/task-detail.component').then(m => m.TaskDetailComponent) },
      { path: 'tasks', loadComponent: () => import('./features/tasks/tasks.component').then(m => m.TasksComponent) },
      { path: 'articles/:id', loadComponent: () => import('./features/articles/article-detail.component').then(m => m.ArticleDetailComponent) },
      { path: 'articles', loadComponent: () => import('./features/articles/articles.component').then(m => m.ArticlesComponent) },
      { path: 'tools', loadComponent: () => import('./features/tools/tools.component').then(m => m.ToolsComponent) },
      { path: 'integrations', loadComponent: () => import('./features/integrations/integrations.component').then(m => m.IntegrationsComponent) },
      { path: 'approvals', loadComponent: () => import('./features/approvals/approvals.component').then(m => m.ApprovalsComponent) },
      { path: 'teams/builder', loadComponent: () => import('./features/teams/builder/builder.component').then(m => m.BuilderComponent) },
      { path: 'teams/:id', loadComponent: () => import('./features/teams/dashboard/dashboard.component').then(m => m.DashboardComponent) },
      { path: 'teams', loadComponent: () => import('./features/teams/teams.component').then(m => m.TeamsComponent) },
      { path: 'runs', loadComponent: () => import('./features/runs/runs.component').then(m => m.RunsComponent) },
      { path: 'runs/:id', loadComponent: () => import('./features/runs/run-detail.component').then(m => m.RunDetailComponent) },
      { path: 'memories', loadComponent: () => import('./features/memories/memories.component').then(m => m.MemoriesComponent) },
      { path: 'browser', loadComponent: () => import('./features/browser/browser.component').then(m => m.BrowserComponent) },
      { path: 'assistant', loadComponent: () => import('./features/assistant/assistant.component').then(m => m.AssistantComponent) },
      { path: 'schedules', loadComponent: () => import('./features/scheduler/scheduler.component').then(m => m.SchedulerComponent) },
      { path: 'settings', loadComponent: () => import('./features/settings/settings.component').then(m => m.SettingsComponent) },
      { path: 'status', loadComponent: () => import('./features/status/status.component').then(m => m.StatusComponent) },
    ],
  },
  {
    path: 'login',
    loadComponent: () => import('./features/auth/login.component').then(m => m.LoginComponent),
  },
  {
    path: 'register',
    loadComponent: () => import('./features/auth/register.component').then(m => m.RegisterComponent),
  },
  { path: '**', redirectTo: '' },
];
