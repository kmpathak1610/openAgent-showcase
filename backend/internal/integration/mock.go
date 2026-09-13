package integration

import (
	"context"
	"fmt"
)

// Mock providers for Phase 5 verification (no external calls)

type MockProvider struct {
	name string
}

func NewMock(name string) *MockProvider { return &MockProvider{name: name} }
func (m *MockProvider) Name() string { return m.name }
func (m *MockProvider) ValidateCredentials(creds map[string]any) error { return nil }
func (m *MockProvider) Execute(ctx context.Context, toolName string, input map[string]any, credentials map[string]any) (map[string]any, error) {
	switch toolName {
	case "web_search":
		q, _ := input["query"].(string)
		return map[string]any{"results": []map[string]any{{"title": "Mock result for " + q, "url": "https://example.com", "snippet": "Mock content"}}}, nil
	case "http_request":
		url, _ := input["url"].(string)
		return map[string]any{"status": 200, "body": map[string]any{"mock": true, "url": url}}, nil
	case "send_email":
		to, _ := input["to"].(string)
		return map[string]any{"messageId": "mock-" + to}, nil
	case "create_calendar_event":
		title, _ := input["title"].(string)
		return map[string]any{"eventId": "evt-" + title}, nil
	case "publish_social_post":
		platform, _ := input["platform"].(string)
		content, _ := input["content"].(string)
		if len(content) > 20 { content = content[:20] }
		return map[string]any{"postId": "post-" + platform + "-" + content, "url": fmt.Sprintf("https://%s.com/post/mock", platform)}, nil
	case "read_social_analytics":
		return map[string]any{"metrics": map[string]any{"impressions": 1234, "likes": 56}}, nil
	case "upload_file":
		fn, _ := input["filename"].(string)
		return map[string]any{"fileId": "file-" + fn}, nil
	case "create_task":
		title, _ := input["title"].(string)
		return map[string]any{"taskId": "task-" + title}, nil
	case "send_channel_message":
		return map[string]any{"messageId": "msg-mock"}, nil
	default:
		return map[string]any{"mock": true, "tool": toolName, "input": input}, nil
	}
}
