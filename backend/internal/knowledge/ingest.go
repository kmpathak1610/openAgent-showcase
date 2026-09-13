package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/llm"
	"openagent/internal/repository"
	"openagent/internal/security"
)

// Chunking: simple token-based (approx 4 chars per token), 500 tokens ~2000 chars, 50 overlap

func ChunkText(content string, chunkSizeTokens int, overlapTokens int) []string {
	if chunkSizeTokens <=0 { chunkSizeTokens = 500 }
	if overlapTokens <0 { overlapTokens = 50 }
	charsPerToken := 4
	chunkChars := chunkSizeTokens * charsPerToken
	overlapChars := overlapTokens * charsPerToken
	content = NormalizeContent(content)
	if len(content) <= chunkChars {
		return []string{content}
	}
	var chunks []string
	for start := 0; start < len(content); {
		end := start + chunkChars
		if end > len(content) { end = len(content) }
		// try to break at paragraph or sentence
		if end < len(content) {
			// look back for newline or period
			cut := end
			for j := end; j > start+chunkChars/2 && j > start; j-- {
				if content[j] == '\n' || content[j] == '.' {
					cut = j+1
					break
				}
			}
			end = cut
		}
		chunks = append(chunks, strings.TrimSpace(content[start:end]))
		if end >= len(content) { break }
		start = end - overlapChars
		if start <0 { start =0 }
	}
	return chunks
}

func NormalizeContent(s string) string {
	s = strings.TrimSpace(s)
	// collapse multiple whitespace, normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// remove excessive blank lines
	for strings.Contains(s, "\n\n\n") { s = strings.ReplaceAll(s, "\n\n\n", "\n\n") }
	return s
}

func EstimateTokens(s string) int {
	return len(s) / 4
}

// Service handles async ingestion via worker

type Service struct {
	db       *repository.DB
	embedder llm.Embedder
}

func NewService(db *repository.DB, embedder llm.Embedder) *Service {
	return &Service{db: db, embedder: embedder}
}

// IngestDocument is called by worker job `ingest_document` with payload {document_id, organization_id, content}
func (s *Service) IngestDocument(ctx context.Context, orgID, docID uuid.UUID, content string) error {
	if err := s.db.UpdateDocumentStatus(orgID, docID, "processing"); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		doc, err := s.db.GetDocument(orgID, docID)
		if err != nil { return err }
		content = doc.Content
		if content == "" {
			content = doc.Title
		}
	}
	// RAG security: sanitize malicious document content before chunking/embedding
	content = security.SanitizeDocumentContent(content)
	// Page-aware chunking: split by [Page N] markers from PDFParser
	type pageChunk struct {
		page    int
		text    string
		section string
	}
	var pageChunks []pageChunk
	pageRe := regexp.MustCompile(`\[Page (\d+)\]\n?`)
	if pageRe.MatchString(content) {
		matches := pageRe.FindAllStringSubmatchIndex(content, -1)
		for idx, m := range matches {
			pageNum, _ := strconv.Atoi(content[m[2]:m[3]])
			start := m[1]
			end := len(content)
			if idx+1 < len(matches) {
				end = matches[idx+1][0]
			}
			pageText := strings.TrimSpace(content[start:end])
			if pageText != "" {
				// Extract section as first heading or first line
				section := ""
				for _, line := range strings.Split(pageText, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "#") {
						section = strings.TrimSpace(strings.TrimLeft(line, "#"))
						break
					}
				}
				pageChunks = append(pageChunks, pageChunk{page: pageNum, text: pageText, section: section})
			}
		}
	} else {
		pageChunks = append(pageChunks, pageChunk{page: 1, text: content, section: ""})
	}
	// Chunk each page and collect with metadata
	var chunksText []string
	var chunkMetas []map[string]any
	for _, pc := range pageChunks {
		texts := ChunkText(pc.text, 500, 50)
		for _, t := range texts {
			chunksText = append(chunksText, t)
			meta := map[string]any{
				"page":     pc.page,
				"section":  pc.section,
				"source":   "upload",
				"parser":   "pdf",
				"documentId": docID.String(),
			}
			if pc.section != "" {
				meta["section"] = pc.section
			}
			chunkMetas = append(chunkMetas, meta)
		}
	}
	// Fallback if no pageChunks (should not happen)
	if len(chunksText) == 0 {
		chunksText = ChunkText(content, 500, 50)
		for range chunksText {
			chunkMetas = append(chunkMetas, map[string]any{"page": 1, "source": "upload", "documentId": docID.String()})
		}
	}
	// embed
	var chunks []domain.DocumentChunk
	if s.embedder == nil {
		_ = s.db.UpdateDocumentStatus(orgID, docID, "failed")
		return fmt.Errorf("embedding provider not configured: set OPENROUTER_API_KEY or EMBEDDING_PROVIDER (refusing to use stub in production)")
	}
	// batch embed via embedder
	embReq := llm.EmbedRequest{Input: chunksText, Model: "text-embedding-3-small"}
	embRes, err := s.embedder.Embed(ctx, embReq)
	if err != nil {
		_ = s.db.UpdateDocumentStatus(orgID, docID, "failed")
		return fmt.Errorf("embed: %w", err)
	}
	if len(embRes.Embeddings) != len(chunksText) {
		_ = s.db.UpdateDocumentStatus(orgID, docID, "failed")
		return fmt.Errorf("embedding count mismatch")
	}
	for i, txt := range chunksText {
		meta := chunkMetas[i]
		// Ensure documentId is set
		if _, ok := meta["documentId"]; !ok {
			meta["documentId"] = docID.String()
		}
		ch := domain.DocumentChunk{
			DocumentID:     docID,
			OrganizationID: orgID,
			ChunkIndex:     i,
			Content:        txt,
			TokenCount:     EstimateTokens(txt),
			Embedding:      embRes.Embeddings[i],
			Metadata:       meta,
			CreatedAt:      time.Now(),
		}
		chunks = append(chunks, ch)
	}
	// store
	if err := s.db.CreateDocumentChunks(orgID, docID, chunks); err != nil {
		_ = s.db.UpdateDocumentStatus(orgID, docID, "failed")
		return err
	}
	return nil
}

// ExtractWebsiteContent fetches URL and returns plain text (simple html strip)
func ExtractWebsiteContent(url string) (string, error) {
	client := &http.Client{Timeout: 10*time.Second}
	resp, err := client.Get(url)
	if err != nil { return "", err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return "", fmt.Errorf("fetch failed %d", resp.StatusCode) }
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB limit
	text := stripHTML(string(body))
	return NormalizeContent(text), nil
}

func stripHTML(s string) string {
	// naive strip: remove tags
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true; continue }
		if r == '>' { inTag = false; continue }
		if !inTag { out.WriteRune(r) }
	}
	return out.String()
}

// ExtractFileContent reads uploaded file bytes via provider abstraction.
// For PDF/DOCX, uses dedicated parsers; if a real parser dependency is missing, returns a clear marker instead of pretending bytes are text.
func ExtractFileContent(data []byte, mime string) string {
	return ExtractFileContentWithFilename(data, mime, "")
}

func ExtractFileContentWithFilename(data []byte, mime, filename string) string {
	parser := GetParser(mime, filename)
	text, err := parser.Parse(data, filename)
	if err != nil {
		// Return marker with abstraction, not raw bytes
		return fmt.Sprintf("[extraction via %s parser failed: %v — size %d bytes, mime %s, file %s — install real parser for production]", parser.Name(), err, len(data), mime, filename)
	}
	return text
}
