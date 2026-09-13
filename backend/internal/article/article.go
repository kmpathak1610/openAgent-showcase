package article

import (
	"fmt"
	"strings"
)

type Brief struct {
	Topic                  string   `json:"topic"`
	Length                 string   `json:"length"`
	Tone                   string   `json:"tone"`
	Format                 string   `json:"format"`
	ProjectID              string   `json:"projectId"`
	SourceURLs             []string `json:"sourceUrls"`
	DocumentIDs            []string `json:"documentIds"`
	KnowledgeCollectionIDs []string `json:"knowledgeCollectionIds"`
}

func (b Brief) Validate() error {
	if strings.TrimSpace(b.Topic) == "" {
		return fmt.Errorf("topic required")
	}
	switch b.Length {
	case "short", "long", "flexible":
	default:
		return fmt.Errorf("length must be short|long|flexible")
	}
	if strings.TrimSpace(b.Tone) == "" {
		return fmt.Errorf("tone required")
	}
	switch b.Format {
	case "blog", "seo", "docs":
	default:
		return fmt.Errorf("format must be blog|seo|docs")
	}
	return nil
}

func (b Brief) TaskTitle() string {
	return "Article: " + strings.TrimSpace(b.Topic)
}

func (b Brief) TaskDescription() string {
	return "topic=" + b.Topic + "; length=" + b.Length + "; tone=" + b.Tone + "; format=" + b.Format
}
