package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

func TestTransformAsides(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)

	tests := []struct {
		name        string
		input       string
		contains    []string
		notContains []string
	}{
		{
			name: "Positive aside callout",
			input: `> aside positive
> **Note on billing:** Any account with billing enabled works, including the
> [free trial](https://cloud.google.com/free).`,
			contains: []string{
				`<aside class="special">`,
				`</aside>`,
				`<strong>Note on billing:</strong>`,
				`<a href="https://cloud.google.com/free">free trial</a>`,
			},
			notContains: []string{
				`aside positive`,
				`<blockquote>`,
			},
		},
		{
			name: "Negative aside callout",
			input: `> aside negative
> **` + "`No open billing account`" + `?** The script stops here and prints instructions.`,
			contains: []string{
				`<aside class="warning">`,
				`</aside>`,
				`<code>No open billing account</code>`,
			},
			notContains: []string{
				`aside negative`,
				`<blockquote>`,
			},
		},
		{
			name: "Regular blockquote preserved",
			input: `> This is a normal quote.
> It should stay as blockquote.`,
			contains: []string{
				`<blockquote>`,
				`This is a normal quote.`,
				`</blockquote>`,
			},
			notContains: []string{
				`<aside`,
			},
		},
		{
			name: "Multi-paragraph aside",
			input: `> aside positive
> First paragraph.
>
> Second paragraph with [link](https://example.com).`,
			contains: []string{
				`<aside class="special">`,
				`<p>First paragraph.</p>`,
				`<p>Second paragraph with <a href="https://example.com">link</a>.</p>`,
				`</aside>`,
			},
		},
		{
			name: "Aside with code block",
			input: `> aside positive
> Check this command:
> ` + "```bash" + `
> gcloud auth list
> ` + "```",
			contains: []string{
				`<aside class="special">`,
				`<pre><code class="language-bash">gcloud auth list`,
				`</aside>`,
			},
		},
		{
			name: "Multiple asides in single step",
			input: `Some introduction text.

> aside positive
> This is a tip.

Middle paragraph.

> aside negative
> This is a warning.

Closing paragraph.`,
			contains: []string{
				`<p>Some introduction text.</p>`,
				`<aside class="special">`,
				`<p>This is a tip.</p>`,
				`</aside>`,
				`<p>Middle paragraph.</p>`,
				`<aside class="warning">`,
				`<p>This is a warning.</p>`,
				`</aside>`,
				`<p>Closing paragraph.</p>`,
			},
		},
		{
			name: "Case insensitive and formatting variations",
			input: `> Aside Positive:
> Title with colon.

> **aside negative**
> Bold marker with text.`,
			contains: []string{
				`<aside class="special">`,
				`<p>Title with colon.</p>`,
				`</aside>`,
				`<aside class="warning">`,
				`<p>Bold marker with text.</p>`,
				`</aside>`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transformed := transformAsides(tc.input)
			var buf bytes.Buffer
			if err := md.Convert([]byte(transformed), &buf); err != nil {
				t.Fatalf("Failed to convert markdown: %v", err)
			}
			out := buf.String()
			for _, exp := range tc.contains {
				if !strings.Contains(out, exp) {
					t.Errorf("Expected output to contain %q, but got:\n%s", exp, out)
				}
			}
			for _, notExp := range tc.notContains {
				if strings.Contains(out, notExp) {
					t.Errorf("Expected output NOT to contain %q, but got:\n%s", notExp, out)
				}
			}
		})
	}
}

func TestParseStepsWithAside(t *testing.T) {
	tempDir := t.TempDir()
	stepFile := filepath.Join(tempDir, "_01-setup.md")
	stepContent := `### Setup Environment

> aside positive
> **Note:** Any account with billing enabled works.

> aside negative
> **Warning:** Check your credentials before proceeding.
`
	if err := os.WriteFile(stepFile, []byte(stepContent), 0644); err != nil {
		t.Fatalf("Failed to write step file: %v", err)
	}

	masterMD := `
## Setup
Duration: 5

<<_01-setup.md>>
`

	steps, err := parseSteps(masterMD, tempDir)
	if err != nil {
		t.Fatalf("parseSteps failed: %v", err)
	}

	if len(steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(steps))
	}

	htmlStr := string(steps[0].HTML)
	if !strings.Contains(htmlStr, `<aside class="special">`) {
		t.Errorf("Expected positive aside in step HTML, got: %s", htmlStr)
	}
	if !strings.Contains(htmlStr, `<aside class="warning">`) {
		t.Errorf("Expected negative aside in step HTML, got: %s", htmlStr)
	}
}

func TestRenderCodelabWithAside(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "preview.html")

	codelab := &Codelab{
		ID:          "test-codelab",
		Title:       "Test Codelab",
		Authors:     "Test Author",
		Duration:    10,
		LastUpdated: "2026-08-28T12:00:00Z",
		Steps: []Step{
			{
				Title:    "Setup",
				Duration: 5,
				HTML:     `<aside class="special"><p><strong>Note:</strong> Welcome!</p></aside>`,
			},
		},
	}

	if err := renderCodelab(codelab, outPath); err != nil {
		t.Fatalf("renderCodelab failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	htmlOutput := string(content)
	if !strings.Contains(htmlOutput, "aside.special, aside.positive") {
		t.Errorf("Expected aside CSS in HTML output")
	}
	if !strings.Contains(htmlOutput, `<aside class="special"><p><strong>Note:</strong> Welcome!</p></aside>`) {
		t.Errorf("Expected step HTML in output: %s", htmlOutput)
	}
}
