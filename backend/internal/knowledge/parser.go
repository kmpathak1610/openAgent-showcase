package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Parser extracts text from raw bytes based on mime/extension.
type Parser interface {
	Parse(data []byte, filename string) (string, error)
	Name() string
}

// Registry holds parsers by mime/extension
var parserRegistry = map[string]Parser{
	"text/plain":       &TextParser{},
	"text/markdown":    &MarkdownParser{},
	"text/html":        &HTMLParser{},
	"application/pdf":  &PDFParser{},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": &DOCXParser{},
}

func GetParser(mime, filename string) Parser {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if p, ok := parserRegistry[mime]; ok {
		return p
	}
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".md"):
		return &MarkdownParser{}
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return &HTMLParser{}
	case strings.HasSuffix(lower, ".pdf"):
		return &PDFParser{}
	case strings.HasSuffix(lower, ".docx"):
		return &DOCXParser{}
	case strings.HasSuffix(lower, ".txt"):
		return &TextParser{}
	default:
		return &TextParser{}
	}
}

// TextParser — plain text, markdown
type TextParser struct{}
func (p *TextParser) Name() string { return "text" }
func (p *TextParser) Parse(data []byte, _ string) (string, error) {
	return NormalizeContent(string(data)), nil
}

type MarkdownParser struct{}
func (p *MarkdownParser) Name() string { return "markdown" }
func (p *MarkdownParser) Parse(data []byte, _ string) (string, error) {
	return NormalizeContent(string(data)), nil
}

// HTMLParser — strips tags, decodes entities
type HTMLParser struct{}
func (p *HTMLParser) Name() string { return "html" }
func (p *HTMLParser) Parse(data []byte, _ string) (string, error) {
	text := stripHTML(string(data))
	replacements := map[string]string{
		"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": "\"", "&#39;": "'",
		"&nbsp;": " ", "&apos;": "'",
	}
	for k, v := range replacements {
		text = strings.ReplaceAll(text, k, v)
	}
	return NormalizeContent(text), nil
}

// PDFParser — production grade using ledongthuc/pdf with heuristic fallback
type PDFParser struct{}
func (p *PDFParser) Name() string { return "pdf" }
func (p *PDFParser) Parse(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty pdf")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return "", fmt.Errorf("not a pdf: missing header")
	}
	if text, err := extractPDFNative(data); err == nil && strings.TrimSpace(text) != "" {
		return NormalizeContent(text), nil
	} else if err != nil {
		heuristic := extractPDFHeuristic(data)
		if strings.TrimSpace(heuristic) != "" {
			return NormalizeContent(heuristic), nil
		}
		return "", fmt.Errorf("pdf extraction failed (native: %v) and heuristic found no text: file %s size %d bytes", err, filename, len(data))
	}
	heuristic := extractPDFHeuristic(data)
	if strings.TrimSpace(heuristic) != "" {
		return NormalizeContent(heuristic), nil
	}
	return "", fmt.Errorf("pdf extraction found no extractable text: file %s size %d bytes — ensure PDF contains selectable text (not scanned image)", filename, len(data))
}

func extractPDFNative(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	numPages := pdfReader.NumPage()
	if numPages == 0 {
		return "", fmt.Errorf("pdf has 0 pages")
	}
	var buf strings.Builder
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if i > 1 {
			buf.WriteString("\n\n")
		}
		buf.WriteString("[Page " + strconv.Itoa(i) + "]\n")
		buf.WriteString(text)
	}
	return buf.String(), nil
}

func extractPDFHeuristic(data []byte) string {
	s := string(data)
	var parts []string
	reParen := regexp.MustCompile(`\(([^)]+)\)`)
	for _, m := range reParen.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			t := strings.TrimSpace(unescapePDFString(m[1]))
			if len(t) >= 3 && isPrintable(t) && !isBinaryGarbage(t) {
				parts = append(parts, t)
			}
		}
	}
	reHex := regexp.MustCompile(`<([0-9A-Fa-f\s]+)>`)
	for _, m := range reHex.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			hexStr := strings.ReplaceAll(m[1], " ", "")
			hexStr = strings.ReplaceAll(hexStr, "\n", "")
			hexStr = strings.ReplaceAll(hexStr, "\r", "")
			if len(hexStr) >= 6 && len(hexStr)%2 == 0 {
				if decoded := decodeHexString(hexStr); decoded != "" && len(strings.TrimSpace(decoded)) >= 3 && isPrintable(decoded) {
					parts = append(parts, strings.TrimSpace(decoded))
				}
			}
		}
	}
	reTJ := regexp.MustCompile(`\[([^\]]+)\]\s*TJ`)
	for _, m := range reTJ.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			inner := reParen.FindAllStringSubmatch(m[1], -1)
			for _, im := range inner {
				if len(im) > 1 {
					t := strings.TrimSpace(unescapePDFString(im[1]))
					if len(t) >= 3 && isPrintable(t) {
						parts = append(parts, t)
					}
				}
			}
		}
	}
	seen := map[string]bool{}
	var dedup []string
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			dedup = append(dedup, p)
		}
	}
	return strings.Join(dedup, " ")
}

func unescapePDFString(s string) string {
	r := strings.ReplaceAll(s, "\\(", "(")
	r = strings.ReplaceAll(r, "\\)", ")")
	r = strings.ReplaceAll(r, "\\\\", "\\")
	r = strings.ReplaceAll(r, "\\n", "\n")
	r = strings.ReplaceAll(r, "\\r", "\r")
	r = strings.ReplaceAll(r, "\\t", "\t")
	return r
}

func decodeHexString(hexStr string) string {
	var out strings.Builder
	for i := 0; i+1 < len(hexStr); i += 2 {
		var b byte
		_, _ = fmt.Sscanf(hexStr[i:i+2], "%02x", &b)
		if b >= 32 && b <= 126 {
			out.WriteByte(b)
		} else if b == 10 || b == 13 {
			out.WriteByte(' ')
		}
	}
	s := out.String()
	if strings.HasPrefix(hexStr, "FEFF") || strings.HasPrefix(hexStr, "feff") {
		var uni strings.Builder
		for i := 4; i+3 < len(hexStr); i += 4 {
			var hi, lo byte
			_, _ = fmt.Sscanf(hexStr[i:i+2], "%02x", &hi)
			_, _ = fmt.Sscanf(hexStr[i+2:i+4], "%02x", &lo)
			if hi == 0 && lo >= 32 && lo <= 126 {
				uni.WriteByte(lo)
			}
		}
		if uni.Len() > 3 {
			return uni.String()
		}
	}
	return s
}

func isBinaryGarbage(s string) bool {
	alpha := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '.' || r == ',' || r == '-' {
			alpha++
		}
	}
	if len(s) == 0 {
		return true
	}
	return float64(alpha)/float64(len(s)) < 0.6
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			if r != '\n' && r != '\r' && r != '\t' {
				return false
			}
		}
	}
	return true
}

// DOCXParser — production grade with headings, lists, tables
type DOCXParser struct{}
func (p *DOCXParser) Name() string { return "docx" }
func (p *DOCXParser) Parse(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty docx")
	}
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("not a valid docx zip: %w", err)
	}
	var docXML []byte
	for _, f := range zipReader.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			docXML, _ = io.ReadAll(rc)
			rc.Close()
			break
		}
	}
	if len(docXML) == 0 {
		return "", fmt.Errorf("docx missing word/document.xml: file %s", filename)
	}
	decoder := xml.NewDecoder(bytes.NewReader(docXML))
	var buf strings.Builder
	var inTable bool
	var listLevel int
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "p":
				if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "\n") {
					buf.WriteString("\n")
				}
			case "pStyle":
				for _, attr := range elem.Attr {
					if attr.Name.Local == "val" {
						val := attr.Value
						if strings.HasPrefix(val, "Heading") {
							level := strings.TrimPrefix(val, "Heading")
							if n, err := strconv.Atoi(level); err == nil && n >= 1 && n <= 6 {
								buf.WriteString(strings.Repeat("#", n) + " ")
							} else {
								buf.WriteString("# ")
							}
						}
					}
				}
			case "numPr":
				listLevel++
				buf.WriteString("- ")
			case "tbl":
				inTable = true
				buf.WriteString("\n")
			case "tr":
				if inTable {
					if !strings.HasSuffix(buf.String(), "\n") {
						buf.WriteString("\n")
					}
					buf.WriteString("| ")
				}
			case "tc":
				// cell start
			case "tab":
				buf.WriteString("\t")
			case "br":
				buf.WriteString("\n")
			}
		case xml.EndElement:
			switch elem.Name.Local {
			case "p":
				buf.WriteString("\n")
				listLevel = 0
			case "tbl":
				inTable = false
				buf.WriteString("\n")
			case "tr":
				if inTable {
					buf.WriteString("\n")
				}
			case "tc":
				if inTable {
					buf.WriteString(" | ")
				}
			}
		case xml.CharData:
			text := strings.TrimSpace(string(elem))
			if text != "" {
				buf.WriteString(text + " ")
			}
		}
		_ = listLevel
	}
	result := strings.TrimSpace(buf.String())
	if result == "" {
		re := regexp.MustCompile(`<w:t[^>]*>([^<]+)</w:t>`)
		matches := re.FindAllStringSubmatch(string(docXML), -1)
		var parts []string
		for _, m := range matches {
			if len(m) > 1 {
				parts = append(parts, m[1])
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("docx extraction found no text: file %s", filename)
		}
		result = strings.Join(parts, " ")
	}
	result = NormalizeContent(result)
	result = strings.ReplaceAll(result, "#  ", "# ")
	return result, nil
}
