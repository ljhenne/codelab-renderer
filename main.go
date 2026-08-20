package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

type Metadata struct {
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	Summary       string `yaml:"summary"`
	Authors       string `yaml:"authors"`
	Duration      int    `yaml:"duration"`
	Layout        string `yaml:"layout"`
	AwardBehavior string `yaml:"award_behavior"`
	Keywords      string `yaml:"keywords"`
	LastUpdated   string `yaml:"last_updated"`
}

type Step struct {
	Title    string
	Duration int
	HTML     template.HTML
}

type Codelab struct {
	ID          string
	Title       string
	Authors     string
	Duration    int
	LastUpdated string
	WatchMode   bool
	Steps       []Step
}

const codelabTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Source+Code+Pro:400,700;Roboto:400,400italic,500,500italic,700,700italic">
  <link rel="stylesheet" href="https://fonts.googleapis.com/icon?family=Material+Icons">
  <link rel="stylesheet" href="https://unpkg.com/codelab-elements/codelab-elements.css">
  <style>
    .success {
      color: #1e8e3e;
    }
    code {
      font-family: 'Source Code Pro', monospace;
    }
    /* codelab-elements.css only styles .note/.tip/.warning/.special, but the
       claat markdown source uses .positive/.negative infoboxes. The long
       selector mirrors that stylesheet so these win over its generic
       "aside { border-left: 4px solid }" rule. */
    aside.positive,
    google-codelab:not([theme="minimal"]) google-codelab-step .instructions aside.positive {
      border: none;
      background: #E6F4EA;
      color: #212124;
    }
    aside.negative,
    google-codelab:not([theme="minimal"]) google-codelab-step .instructions aside.negative {
      border: none;
      background: #FEF7E0;
      color: #212124;
    }
    google-codelab google-codelab-step .instructions aside > :first-child {
      margin-top: 0;
    }
    google-codelab google-codelab-step .instructions aside > :last-child {
      margin-bottom: 0;
    }
  </style>
</head>
<body class="color-scheme--light">
  <google-codelab id="{{.ID}}"
                  title="{{.Title}}"
                  authors="{{.Authors}}"
                  duration="{{.Duration}}"
                  environment="web"
                  feedback-link="">
    {{range $idx, $step := .Steps}}
    <google-codelab-step label="{{$step.Title}}" duration="{{$step.Duration}}">
      {{if eq $idx 0}}
      <google-codelab-about codelab-title="{{$.Title}}"
                            authors="{{$.Authors}}"
                            last-updated="{{$.LastUpdated}}">
      </google-codelab-about>
      {{end}}
      {{$step.HTML}}
    </google-codelab-step>
    {{end}}
  </google-codelab>
  <script src="https://unpkg.com/@webcomponents/webcomponentsjs/custom-elements-es5-adapter.js"></script>
  <script src="https://unpkg.com/codelab-elements/codelab-elements.js"></script>
  {{if .WatchMode}}
  <script>
    // Restore scroll position
    const savedScrollTop = sessionStorage.getItem('codelab-scroll-top');
    if (savedScrollTop) {
      const restore = () => {
        const activeStep = document.querySelector('google-codelab-step[selected]');
        if (activeStep) {
          activeStep.scrollTop = parseInt(savedScrollTop, 10);
          sessionStorage.removeItem('codelab-scroll-top');
        } else {
          requestAnimationFrame(restore);
        }
      };
      requestAnimationFrame(restore);
    }

    const reloadEvents = new EventSource('/reload-events');
    reloadEvents.onmessage = (event) => {
      if (event.data === 'reload') {
        console.log('Detected file change. Saving scroll state and reloading...');
        const activeStep = document.querySelector('google-codelab-step[selected]');
        if (activeStep) {
          sessionStorage.setItem('codelab-scroll-top', activeStep.scrollTop);
        }
        window.location.reload();
      }
    };
  </script>
  {{end}}
</body>
</html>
`

func main() {
	inPath := flag.String("in", "instructions.lab.md", "path to the input markdown file")
	outPath := flag.String("out", "preview.html", "path to the output HTML file")
	openFlag := flag.Bool("open", true, "automatically open the generated HTML file in the default browser")
	watchFlag := flag.Bool("watch", false, "start a local HTTP server and automatically refresh on markdown updates")
	flag.Parse()

	log.Printf("Starting Codelab renderer compilation...")
	log.Printf("Input: %s", *inPath)
	log.Printf("Output: %s", *outPath)
	log.Printf("Watch Mode: %v", *watchFlag)

	codelab, err := parseInstructions(*inPath)
	if err != nil {
		log.Fatalf("Compilation failed: %v", err)
	}
	codelab.WatchMode = *watchFlag

	if err := renderCodelab(codelab, *outPath); err != nil {
		log.Fatalf("Rendering failed: %v", err)
	}

	log.Printf("Successfully compiled Codelab renderer output to %s!", *outPath)

	if *watchFlag {
		rs := &ReloadServer{
			clients: make(map[chan bool]bool),
		}

		go watchFiles(*inPath, *outPath, codelab, rs)

		addr := "localhost:8080"
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Port 8080 is in use. Finding a free alternative port...")
			listener, err = net.Listen("tcp", "localhost:0")
			if err != nil {
				log.Fatalf("Failed to bind port listener: %v", err)
			}
			addr = listener.Addr().String()
		}

		port := listener.Addr().(*net.TCPAddr).Port
		url := fmt.Sprintf("http://localhost:%d/", port)

		http.HandleFunc("/reload-events", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Access-Control-Allow-Origin", "*")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
				return
			}

			ch := make(chan bool, 1)
			rs.Lock()
			rs.clients[ch] = true
			rs.Unlock()

			defer func() {
				rs.Lock()
				delete(rs.clients, ch)
				rs.Unlock()
				close(ch)
			}()

			fmt.Fprintf(w, "data: connected\n\n")
			flusher.Flush()

			ctx := r.Context()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ch:
					fmt.Fprintf(w, "data: reload\n\n")
					flusher.Flush()
				}
			}
		})

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.FileServer(http.Dir(filepath.Dir(*inPath))).ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, *outPath)
		})

		log.Printf("Serving preview server at %s", url)
		if *openFlag {
			log.Printf("Opening preview in browser...")
			if err := openBrowser(url); err != nil {
				log.Printf("Warning: failed to open browser: %v", err)
			}
		}

		if err := http.Serve(listener, nil); err != nil {
			log.Fatalf("Preview server failed: %v", err)
		}
	} else {
		if *openFlag {
			absOutPath, err := filepath.Abs(*outPath)
			if err != nil {
				log.Printf("Warning: failed to get absolute path for output: %v", err)
				absOutPath = *outPath
			}
			log.Printf("Opening preview in browser...")
			if err := openBrowser(absOutPath); err != nil {
				log.Printf("Warning: failed to open browser: %v", err)
			}
		}
	}
}

func parseInstructions(filePath string) (*Codelab, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	raw := string(content)

	// Separate YAML Front Matter
	var yamlStr, mdStr string
	if strings.HasPrefix(raw, "---") {
		parts := strings.SplitN(raw, "---", 3)
		if len(parts) >= 3 {
			yamlStr = parts[1]
			mdStr = parts[2]
		} else {
			mdStr = raw
		}
	} else {
		mdStr = raw
	}

	var meta Metadata
	if yamlStr != "" {
		if err := yaml.Unmarshal([]byte(yamlStr), &meta); err != nil {
			return nil, fmt.Errorf("failed to parse YAML front matter: %w", err)
		}
	}

	dir := filepath.Dir(filePath)

	steps, err := parseSteps(mdStr, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse steps: %w", err)
	}

	// Calculate total duration if not specified
	totalDuration := meta.Duration
	if totalDuration == 0 {
		for _, step := range steps {
			totalDuration += step.Duration
		}
	}

	// Sanitize Codelab ID
	id := meta.ID
	if id == "" {
		id = strings.ToLower(strings.ReplaceAll(meta.Title, " ", "-"))
		id = regexp.MustCompile(`[^a-z0-9\-_]`).ReplaceAllString(id, "")
	}

	lastUpdated := meta.LastUpdated
	if lastUpdated == "" {
		lastUpdated = time.Now().Format("2006-01-02T15:04:05Z")
	}

	codelab := &Codelab{
		ID:          id,
		Title:       meta.Title,
		Authors:     meta.Authors,
		Duration:    totalDuration,
		LastUpdated: lastUpdated,
		Steps:       steps,
	}

	if codelab.Title == "" {
		// Fallback to top-level heading # if present
		re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
		if matches := re.FindStringSubmatch(mdStr); len(matches) > 1 {
			codelab.Title = strings.TrimSpace(matches[1])
		} else {
			codelab.Title = "Codelab Preview"
		}
	}

	return codelab, nil
}

func parseSteps(mdStr string, baseDir string) ([]Step, error) {
	lines := strings.Split(mdStr, "\n")
	var steps []Step
	var currentStep *Step
	var rawLines []string

	durationRe := regexp.MustCompile(`(?i)duration:\s*(\d+)`)
	linkRe := regexp.MustCompile(`\[[^\]]*\]\(([^)]+\.md)\)`)
	attributeRe := regexp.MustCompile(`\{:.*\}`)

	saveCurrentStep := func() error {
		if currentStep == nil {
			return nil
		}

		var stepMD string
		var referencedFile string
		for _, line := range rawLines {
			if matches := linkRe.FindStringSubmatch(line); len(matches) > 1 {
				referencedFile = strings.TrimSpace(matches[1])
				break
			}
		}

		if referencedFile != "" {
			absPath := filepath.Join(baseDir, referencedFile)
			content, err := os.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("failed to read step file %s: %w", referencedFile, err)
			}
			stepMD = string(content)
		} else {
			stepMD = strings.Join(rawLines, "\n")
		}

		var buf bytes.Buffer
		md := goldmark.New(
			goldmark.WithExtensions(
				extension.GFM,
			),
			goldmark.WithRendererOptions(
				// Codelab sources embed raw HTML (e.g. <aside class="positive">).
				// Without this goldmark replaces it with "<!-- raw HTML omitted -->".
				html.WithUnsafe(),
			),
		)
		if err := md.Convert([]byte(normalizeAsides(stepMD)), &buf); err != nil {
			return fmt.Errorf("failed to convert markdown to html: %w", err)
		}

		currentStep.HTML = template.HTML(buf.String())
		steps = append(steps, *currentStep)
		return nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if err := saveCurrentStep(); err != nil {
				return nil, err
			}

			title := strings.TrimPrefix(trimmed, "## ")
			title = attributeRe.ReplaceAllString(title, "")
			title = strings.TrimSpace(title)

			currentStep = &Step{
				Title:    title,
				Duration: 0,
			}
			rawLines = []string{}
			continue
		}

		if currentStep != nil {
			if matches := durationRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				var dur int
				if _, err := fmt.Sscanf(matches[1], "%d", &dur); err == nil {
					currentStep.Duration = dur
				}
				continue
			}
			rawLines = append(rawLines, line)
		}
	}

	if err := saveCurrentStep(); err != nil {
		return nil, err
	}

	return steps, nil
}

var (
	asideBlockRe = regexp.MustCompile(`(?ism)^[ \t]*<aside\b[^>]*>.*?</aside\s*>`)
	asideOpenRe  = regexp.MustCompile(`(?is)^[ \t]*<aside\b[^>]*>`)
	asideCloseRe = regexp.MustCompile(`(?is)</aside\s*>$`)
	fenceRe      = regexp.MustCompile("^\\s{0,3}(?:```|~~~)")
)

// normalizeAsides surrounds the contents of an <aside> with blank lines. An
// <aside> starts a markdown HTML block that only ends at a blank line, so
// without this the body of an infobox is emitted as opaque raw HTML and its
// markdown (bold, lists, links) is never converted. Fenced code blocks are
// left untouched so codelabs can show <aside> markup as example source.
func normalizeAsides(md string) string {
	var result []string
	var text []string
	inFence := false

	flush := func() {
		if len(text) == 0 {
			return
		}
		result = append(result, strings.Split(rewriteAsides(strings.Join(text, "\n")), "\n")...)
		text = nil
	}

	for _, line := range strings.Split(md, "\n") {
		switch {
		case fenceRe.MatchString(line):
			flush()
			inFence = !inFence
			result = append(result, line)
		case inFence:
			result = append(result, line)
		default:
			text = append(text, line)
		}
	}
	flush()

	return strings.Join(result, "\n")
}

func rewriteAsides(s string) string {
	return asideBlockRe.ReplaceAllStringFunc(s, func(block string) string {
		open := asideOpenRe.FindString(block)
		closeLoc := asideCloseRe.FindStringIndex(block)
		if open == "" || closeLoc == nil || closeLoc[0] < len(open) {
			return block
		}

		// An <aside> nested in a list item stays part of that item only while
		// its lines keep the item's indentation, so realign the body with the
		// opening tag instead of flattening it to column zero.
		indent := open[:len(open)-len(strings.TrimLeft(open, " \t"))]
		inner := indentLines(dedent(strings.Trim(block[len(open):closeLoc[0]], "\n")), indent)
		closeTag := indent + block[closeLoc[0]:]
		if strings.TrimSpace(inner) == "" {
			return open + closeTag
		}

		return open + "\n\n" + inner + "\n\n" + closeTag
	})
}

func indentLines(s, indent string) string {
	if indent == "" {
		return s
	}

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// dedent removes the common leading whitespace from every line, preserving the
// relative indentation that nested lists rely on. Without it, an infobox body
// indented more deeply than its tag would be parsed as a code block.
func dedent(s string) string {
	lines := strings.Split(s, "\n")

	indent := -1
	for _, line := range lines {
		stripped := strings.TrimLeft(line, " \t")
		if stripped == "" {
			continue
		}
		if width := len(line) - len(stripped); indent < 0 || width < indent {
			indent = width
		}
	}
	if indent <= 0 {
		return s
	}

	for i, line := range lines {
		if len(line) >= indent {
			lines[i] = line[indent:]
		} else {
			lines[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

func renderCodelab(codelab *Codelab, outputPath string) error {
	tmpl, err := template.New("codelab").Parse(codelabTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, codelab); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // "linux", "freebsd", etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

type ReloadServer struct {
	sync.Mutex
	clients map[chan bool]bool
}

func (rs *ReloadServer) Broadcast() {
	rs.Lock()
	defer rs.Unlock()
	for ch := range rs.clients {
		select {
		case ch <- true:
		default:
		}
	}
}

func watchFiles(inPath, outPath string, codelab *Codelab, rs *ReloadServer) {
	dir := filepath.Dir(inPath)
	modTimes := make(map[string]time.Time)

	scan := func() bool {
		changed := false
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
				lastTime, exists := modTimes[path]
				if !exists {
					modTimes[path] = info.ModTime()
				} else if info.ModTime().After(lastTime) {
					modTimes[path] = info.ModTime()
					changed = true
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("Error scanning files: %v", err)
		}
		return changed
	}

	// Initial scan to populate times
	scan()

	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		if scan() {
			log.Printf("Detected file changes. Recompiling Codelab...")
			newCodelab, err := parseInstructions(inPath)
			if err != nil {
				log.Printf("Recompilation error: %v", err)
				continue
			}
			newCodelab.WatchMode = true
			if err := renderCodelab(newCodelab, outPath); err != nil {
				log.Printf("Rendering error: %v", err)
				continue
			}
			log.Printf("Recompilation successful. Reloading browser...")
			rs.Broadcast()
		}
	}
}
