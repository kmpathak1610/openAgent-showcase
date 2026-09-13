import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { ArticleService } from './article.service';

describe('ArticleService', () => {
  let svc: ArticleService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), ArticleService] });
    svc = TestBed.inject(ArticleService);
    http = TestBed.inject(HttpTestingController);
  });

  it('creates parent task with article title', () => {
    svc.create({ projectId: 'p1', topic: 'Vector DBs', length: 'short', tone: 'friendly', format: 'blog' }).subscribe();
    const req = http.expectOne(r => r.url.endsWith('/tasks') && r.method === 'POST');
    expect(req.request.body.title).toContain('Vector DBs');
    req.flush({ data: { id: 't1', title: 'Article: Vector DBs' } });
    expect(svc.articles()[0].id).toBe('t1');
  });
});
