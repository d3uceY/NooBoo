package services

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"nooboo/models"
)

// logNames are the error logs the game drops beside its own folder, newest wins.
var logNames = []string{"lastException.txt", "lastUIException.txt", "lastCrash.txt"}

// ExceptionService turns the game's raw error log into something readable.
type ExceptionService struct {
	cfg *ConfigService
}

func NewExceptionService(cfg *ConfigService) *ExceptionService { return &ExceptionService{cfg: cfg} }

// frame is one line of a traceback. Path, Line and Fn stay empty on lines that
// are not frames, so the summary and the error text survive too.
type frame struct {
	path string
	fn   string
	line int
	text string
}

// Latest reports the newest error log. No folder or no log reads as "none",
// because a clean game is the normal case, not a failure.
func (s *ExceptionService) Latest() (models.Exception, error) {
	empty := models.Exception{Trace: []string{}, Suspects: []models.Suspect{}}
	root := s.cfg.Get().GameRoot
	if root == "" {
		return empty, nil
	}

	file, modTime := newestLog(root)
	if file == "" {
		return empty, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return empty, err
	}

	out, frames := parseLog(string(raw))
	out.Found = true
	out.File = file
	if out.When == "" {
		out.When = modTime.Format(time.RFC3339)
	}
	modsRoot := filepath.Join(root, "Mods")
	out.Suspects = findSuspects(frames, modsRoot)
	attachMods(out.Suspects, modsRoot)
	return out, nil
}

// attachMods resolves a suspect by folder name when its path does not sit under
// Mods. That happens when the log was written before the folder was moved, or
// when the game is reached through a OneDrive redirect.
func attachMods(suspects []models.Suspect, modsRoot string) {
	entries, err := os.ReadDir(modsRoot)
	if err != nil {
		return
	}
	for i := range suspects {
		if suspects[i].ID != "" {
			continue
		}
		lower := strings.ToLower(suspects[i].Path)
		for _, e := range entries {
			name := e.Name()
			if len(name) < 3 || !strings.Contains(lower, strings.ToLower(name)) {
				continue
			}
			suspects[i].ID = name
			suspects[i].Label = name
			break
		}
	}
}

// Open reveals the log so the user can read every line themselves.
func (s *ExceptionService) Open() error {
	file, _ := newestLog(s.cfg.Get().GameRoot)
	if file == "" {
		return errors.New("no error log to open")
	}
	return openInExplorer(file)
}

func newestLog(root string) (string, time.Time) {
	var best string
	var bestTime time.Time
	for _, name := range logNames {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || info.IsDir() {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best = filepath.Join(root, name)
			bestTime = info.ModTime()
		}
	}
	return best, bestTime
}

// tracebackLine is a Python `File "...", line N, in f` frame.
var tracebackLine = regexp.MustCompile(`File "([^"]+)", line (\d+), in (.+)`)

// parseLog pulls the summary and the frames out of the game's log. The file is
// XML-ish and the interesting part is one escaped blob, so this reads it as
// text rather than trusting a strict decoder with the whole document.
func parseLog(raw string) (models.Exception, []frame) {
	out := models.Exception{Trace: []string{}, Suspects: []models.Suspect{}}
	report := newestReport(raw)
	out.Build = oneLine(textOf(report, "buildsignature"))
	if stamp, err := time.Parse("2006-01-02 15:04:05", oneLine(textOf(report, "createtime"))); err == nil {
		out.When = stamp.Format(time.RFC3339)
	}

	blob := textOf(report, "desyncdata")
	if blob == "" {
		blob = textOf(report, "descr")
	}
	if blob == "" {
		// Crash dumps are plain text with no report wrapper at all.
		blob = report
	}

	frames := []frame{}
	for _, line := range splitLines(blob) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		frames = append(frames, toFrame(line))
	}

	shown := frames
	if len(shown) > 24 {
		shown = shown[len(shown)-24:]
	}
	for _, f := range shown {
		out.Trace = append(out.Trace, f.text)
	}

	out.Message = engineSummary(out.Trace)
	if out.Message == "" {
		out.Message = lastLine(out.Trace)
	}
	if out.Message == "" {
		out.Message = oneLine(textOf(report, "categoryid"))
	}
	if out.Message == "" {
		out.Message = "Script error"
	}
	return out, frames
}

// newestReport picks the latest report block. One file can hold several, and
// the engine appends them, so equal timestamps put the last one in front.
func newestReport(raw string) string {
	parts := strings.Split(raw, "<report>")
	if len(parts) < 2 {
		return raw
	}
	best := parts[1]
	bestStamp := oneLine(textOf(best, "createtime"))
	for _, p := range parts[2:] {
		stamp := oneLine(textOf(p, "createtime"))
		if stamp >= bestStamp {
			best, bestStamp = p, stamp
		}
	}
	return best
}

// engineSummary is the j[mal|g] line the engine writes before the traceback. It
// says what failed in one sentence, which beats the exception type at the end.
func engineSummary(trace []string) string {
	if len(trace) == 0 {
		return ""
	}
	first := strings.TrimSpace(trace[0])
	if strings.HasPrefix(first, "j[mal|g]") {
		return first
	}
	return ""
}

func toFrame(raw string) frame {
	line := strings.TrimSpace(raw)
	m := tracebackLine.FindStringSubmatch(line)
	if m == nil {
		return frame{text: line}
	}
	return frame{
		path: normalPath(m[1]),
		line: atoi(m[2]),
		fn:   strings.TrimSpace(m[3]),
		text: shortenFrame(m[1], m[2], m[3]),
	}
}

// findSuspects maps traceback frames back to Mods. A frame under the user's
// Mods folder is a direct hit; a frame from an unknown script is a mention.
func findSuspects(frames []frame, modsRoot string) []models.Suspect {
	out := []models.Suspect{}
	seen := map[string]bool{}
	mods := normalPath(modsRoot)

	for _, f := range frames {
		if f.path == "" {
			continue
		}
		// Frames like File "<string>" are generated code, not a file.
		if !strings.Contains(f.path, `\`) && !strings.HasSuffix(strings.ToLower(f.path), ".py") {
			continue
		}

		if mods != "" && underRoot(f.path, mods) {
			id := firstSegment(f.path, mods)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, models.Suspect{ID: id, Label: id, Path: f.path, Line: f.line, Why: "in the stack"})
			continue
		}

		label := baseName(f.path)
		if isGamePath(f.path) || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, models.Suspect{Label: label, Path: f.path, Line: f.line, Why: "named in the stack"})
	}
	return out
}

// isGamePath is the engine's own code, which is never the answer.
func isGamePath(path string) bool {
	lower := strings.ToLower(path)
	for _, marker := range []string{`\ingame\`, `\game\bin\`, `\game\bin_le\`, `\game\bin64\`} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// normalPath evens out the separators and the doubled slashes some logs carry.
func normalPath(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	return p
}

func underRoot(path, root string) bool {
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(root)+`\`)
}

func firstSegment(path, root string) string {
	rest := strings.TrimPrefix(path, root)
	rest = strings.TrimPrefix(rest, `\`)
	if i := strings.Index(rest, `\`); i >= 0 {
		return rest[:i]
	}
	return rest
}

func baseName(path string) string {
	if i := strings.LastIndex(path, `\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// shortenFrame drops the install path so one frame fits one line.
func shortenFrame(path, line, fn string) string {
	p := normalPath(path)
	lower := strings.ToLower(p)
	for _, marker := range []string{`\scripts\`, `\mods\`} {
		if i := strings.Index(lower, marker); i >= 0 {
			p = p[i+len(marker):]
			break
		}
	}
	return `File "` + p + `", line ` + line + `, in ` + strings.TrimSpace(fn)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func lastLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// splitLines handles the &#13;&#10; pairs the engine uses inside one tag.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// textOf returns the contents of the first tag by that name, unescaped.
func textOf(raw, tag string) string {
	re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>(.*?)</` + tag + `>`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	return unescape(m[1])
}

// oneLine is textOf flattened onto a single line.
func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", ""))
}

func unescape(s string) string {
	return strings.NewReplacer(
		"&#13;", "\n",
		"&#10;", "\n",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
		"&#39;", "'",
		"&amp;", "&",
	).Replace(s)
}
