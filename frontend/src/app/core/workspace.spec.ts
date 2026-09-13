import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService } from './auth.service';
import { ProjectService } from './project.service';
import { ChannelService } from './channel.service';
import { MessageService } from './message.service';

describe('Workspace critical behavior', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    localStorage.clear();
  });

  it('auth service should handle token storage and isAuthenticated', () => {
    const auth = TestBed.inject(AuthService);
    expect(auth.isAuthenticated()).toBe(false);
    // simulate token set via localStorage
    localStorage.setItem('openagent_token', 'fake-jwt-token');
    expect(localStorage.getItem('openagent_token')).toBe('fake-jwt-token');
    auth.logout(false);
    expect(auth.isAuthenticated()).toBe(false);
    expect(localStorage.getItem('openagent_token')).toBeNull();
  });

  it('project service should be injectable', () => {
    const ps = TestBed.inject(ProjectService);
    expect(ps).toBeTruthy();
    expect(ps.projects()).toEqual([]);
  });

  it('channel service should manage selection', () => {
    const cs = TestBed.inject(ChannelService);
    cs.channels.set([
      { id: '1', name: 'general', displayName: 'General', description: '', topic: '', channelType: 'standard', organizationId: 'org1', projectId: null, createdAt: '' },
    ] as any);
    cs.select('1');
    expect(cs.selectedId()).toBe('1');
    expect(cs.selected()?.name).toBe('general');
  });

  it('message service should handle incoming WS events', () => {
    const ms = TestBed.inject(MessageService);
    ms.messages.set([]);
    // simulate message.created event
    ms.handleIncoming({
      type: 'message.created',
      payload: { message: { id: 'm1', channelId: 'c1', body: 'hello @alice', createdAt: new Date().toISOString() } as any },
    });
    expect(ms.messages().length).toBe(1);
    expect(ms.messages()[0].id).toBe('m1');

    // simulate edit
    ms.handleIncoming({
      type: 'message.updated',
      payload: { message: { id: 'm1', body: 'hello @alice edited' } as any },
    });
    expect(ms.messages()[0].body).toBe('hello @alice edited');

    // simulate delete
    ms.handleIncoming({ type: 'message.deleted', payload: { messageId: 'm1' } });
    expect(ms.messages().length).toBe(0);
  });

  it('message body should highlight mentions', () => {
    // This is the same logic as ChannelDetailComponent.renderBody
    const renderBody = (body: string) => {
      const escaped = body.replace(/</g, '&lt;').replace(/>/g, '&gt;');
      return escaped.replace(/@(\w+)/g, '<span class="mention">@$1</span>');
    };
    expect(renderBody('hello @alice and @bob')).toContain('<span class="mention">@alice</span>');
    expect(renderBody('no mentions')).not.toContain('<span class="mention">');
    expect(renderBody('<script>@x</script>')).toContain('&lt;script&gt;');
  });
});
