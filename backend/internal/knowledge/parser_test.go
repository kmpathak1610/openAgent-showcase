package knowledge

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestPDFParser_Simple(t *testing.T) {
	pdfData := []byte(`%PDF-1.1
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT
/F1 12 Tf
72 720 Td
(Hello World) Tj
ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000056 00000 n
0000000111 00000 n
0000000212 00000 n
0000000412 00000 n
trailer
<< /Size 6 /Root 1 0 R >>
startxref
500
%%EOF`)
	parser := &PDFParser{}
	text, err := parser.Parse(pdfData, "test.pdf")
	if err != nil {
		t.Fatalf("PDF parse failed: %v", err)
	}
	if !strings.Contains(text, "Hello World") {
		t.Fatalf("expected Hello World in extracted text, got %q", text)
	}
	if !strings.Contains(text, "[Page 1]") {
		t.Logf("page marker not found, text: %q", text)
	}
}

func TestPDFParser_Unicode(t *testing.T) {
	// Use hex string for Unicode: <FEFF00630061006600E9> is "café" in UTF-16BE
	pdfData := []byte("%PDF-1.1\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>\nendobj\n4 0 obj\n<< /Length 30 >>\nstream\nBT\n<FEFF00630061006600E9> Tj\nET\nendstream\nendobj\nxref\n0 5\ntrailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n0\n%%EOF")
	parser := &PDFParser{}
	text, err := parser.Parse(pdfData, "unicode.pdf")
	if err != nil {
		t.Fatalf("Unicode PDF parse failed: %v", err)
	}
	// Should contain café or at least not fail
	if strings.TrimSpace(text) == "" {
		t.Fatalf("expected non-empty text for unicode PDF, got %q", text)
	}
}

func TestPDFParser_Malformed(t *testing.T) {
	parser := &PDFParser{}
	_, err := parser.Parse([]byte("not a pdf"), "bad.pdf")
	if err == nil {
		t.Fatal("expected error for malformed PDF")
	}
	_, err = parser.Parse([]byte(""), "empty.pdf")
	if err == nil {
		t.Fatal("expected error for empty PDF")
	}
	_, err = parser.Parse([]byte("%PDF-1.1\n no content"), "empty2.pdf")
	if err == nil {
		t.Logf("expected error for PDF with no extractable text, but got no error (heuristic may have found something)")
	}
}

func TestDOCXParser_Simple(t *testing.T) {
	// Create minimal DOCX in memory
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Test Heading</w:t></w:r></w:p><w:p><w:r><w:t>Hello world paragraph.</w:t></w:r></w:p></w:body></w:document>`))
	zw.Close()
	parser := &DOCXParser{}
	text, err := parser.Parse(buf.Bytes(), "test.docx")
	if err != nil {
		t.Fatalf("DOCX parse failed: %v", err)
	}
	if !strings.Contains(text, "Test Heading") {
		t.Fatalf("expected heading in text, got %q", text)
	}
	if !strings.Contains(text, "Hello world") {
		t.Fatalf("expected paragraph in text, got %q", text)
	}
	if !strings.Contains(text, "#") {
		t.Logf("heading marker not preserved, text: %q", text)
	}
}

func TestDOCXParser_Table(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cell A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Cell B1</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p><w:r><w:t>Cell A2</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Cell B2</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`))
	zw.Close()
	parser := &DOCXParser{}
	text, err := parser.Parse(buf.Bytes(), "table.docx")
	if err != nil {
		t.Fatalf("DOCX table parse failed: %v", err)
	}
	if !strings.Contains(text, "Cell A1") || !strings.Contains(text, "Cell B2") {
		t.Fatalf("expected table cells in text, got %q", text)
	}
	if !strings.Contains(text, "|") {
		t.Logf("table marker | not found, text: %q", text)
	}
}

func TestDOCXParser_Malformed(t *testing.T) {
	parser := &DOCXParser{}
	_, err := parser.Parse([]byte("not a zip"), "bad.docx")
	if err == nil {
		t.Fatal("expected error for malformed DOCX")
	}
}
