package sync

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
)

var bannerTmpl = template.Must(template.New("banner").Parse(`
{{ .Header }}
{{ range .Paths }}  {{ . }}
{{ end }}{{ if .MoreCount }}  ...and {{ .MoreCount }} more
{{ end }}{{ .Footer }}
`))

type bannerData struct {
	Header    string
	Paths     []string
	MoreCount int
	Footer    string
}

// bannerMaxPaths is the maximum number of conflict paths shown in the banner.
const bannerMaxPaths = 3

// RenderBanner writes a sync conflict warning banner to the given writer.
// Uses lipgloss for TTY-aware colored output (auto-strips ANSI when not a TTY).
// If conflicts is empty, writes nothing.
func RenderBanner(conflicts []ConflictInfo, w io.Writer) {
	if len(conflicts) == 0 {
		return
	}

	renderer := lipgloss.NewRenderer(w)
	yellow := renderer.NewStyle().Foreground(lipgloss.Color("3"))

	noun := "conflict"
	if len(conflicts) != 1 {
		noun = "conflicts"
	}
	header := yellow.Render(fmt.Sprintf("⚠ %d sync %s need attention:", len(conflicts), noun))

	shown := bannerMaxPaths
	if len(conflicts) < shown {
		shown = len(conflicts)
	}
	var paths []string
	for _, c := range conflicts[:shown] {
		desc := conflictDescription(c.LocalState, c.ContainerState)
		paths = append(paths, fmt.Sprintf("%-30s (%s)", c.Path, desc))
	}

	moreCount := 0
	if len(conflicts) > bannerMaxPaths {
		moreCount = len(conflicts) - bannerMaxPaths
	}

	footer := yellow.Render("Run 'alca experimental sync resolve' to resolve.")

	data := bannerData{
		Header:    header,
		Paths:     paths,
		MoreCount: moreCount,
		Footer:    footer,
	}

	var buf strings.Builder
	_ = bannerTmpl.Execute(&buf, data)
	_, _ = io.WriteString(w, buf.String())
}

// conflictDescription returns a human-readable description of the conflict.
func conflictDescription(localState, containerState string) string {
	if localState == containerState {
		return localState + " on both sides"
	}
	return localState + " locally, " + containerState + " in container"
}
