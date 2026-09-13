import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { SettingsService } from './settings.service';

describe('SettingsService', () => {
  let svc: SettingsService;
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), SettingsService] });
    svc = TestBed.inject(SettingsService);
    http = TestBed.inject(HttpTestingController);
  });
  it('loads default model from backend', () => {
    svc.llm().subscribe();
    const req = http.expectOne(r => r.url.endsWith('/settings/llm') && r.method === 'GET');
    req.flush({ data: { defaultModel: 'minimax/minimax-m3:free', availableModels: ['minimax/minimax-m3:free'], providers: [{ name: 'openrouter', real: true }] } });
    expect(svc.config()?.defaultModel).toBe('minimax/minimax-m3:free');
  });
});
