package chum

import (
	"regexp"
	"strings"
)

// Badge images from sailfishos-chum-gui, the same files Chum shows beside a description.
const aiBadgeBase = "https://raw.githubusercontent.com/sailfishos-chum/sailfishos-chum-gui/main/icons/theme/ai-score/svgs/icons/"

var (
	chumMetaLine = regexp.MustCompile(`^(Title|Type|DeveloperName|PackagedBy|Categories|AIRating|AINote|AINoteMD|Custom|PackageIcon|Screenshots|Links|PackageName|Icon|PackagerName|Url):`)
	aiRatingLine = regexp.MustCompile(`(?m)^AIRating:[ \t]*([^\n]*)`)
	blankLines   = regexp.MustCompile(`\n[ \t]*\n`)
)

// descriptionWithAI drops a trailing Chum metadata paragraph, then appends the
// badge for its AIRating. A missing or unrecognized rating uses the unknown badge.
func descriptionWithAI(description string) string {
	body, code := chumAI(description)
	return appendAIImage(body, code)
}

// packageCategory follows SailfishOS:Chum. desktop-application and
// console-application are applications, and so is a harbour- package or an
// Applications RPM group when Type is absent. Any other explicit Type is a library.
func packageCategory(rows []primaryPkg) string {
	harbour := false
	groupApp := false
	consoleOnly := false
	explicit := ""
	for _, row := range rows {
		if strings.HasPrefix(row.Name, "harbour-") {
			harbour = true
		}
		if strings.HasPrefix(strings.TrimSpace(row.Format.Group), "Applications") {
			groupApp = true
		}
		meta := chumMetaParagraph(row.Description)
		if meta == "" {
			continue
		}
		switch strings.ToLower(metaScalar(meta, "Type")) {
		case "":
		case "desktop-application", "console-application":
			explicit = "application"
		default:
			if explicit != "application" {
				explicit = "other"
			}
		}
		for _, cat := range metaList(meta, "Categories") {
			if strings.EqualFold(cat, "ConsoleOnly") {
				consoleOnly = true
			}
		}
	}
	if consoleOnly || explicit == "application" {
		return "application"
	}
	if explicit == "other" {
		return "library"
	}
	if harbour || groupApp {
		return "application"
	}
	return "library"
}

func packageIcon(id string, rows []primaryPkg) string {
	fallback := ""
	for _, row := range rows {
		meta := chumMetaParagraph(row.Description)
		if meta == "" {
			continue
		}
		icon := metaScalar(meta, "PackageIcon")
		if icon == "" {
			icon = metaScalar(meta, "Icon")
		}
		if !strings.HasPrefix(icon, "https://") && !strings.HasPrefix(icon, "http://") {
			continue
		}
		if row.Name == id {
			return icon
		}
		if fallback == "" {
			fallback = icon
		}
	}
	return fallback
}

func chumMetaParagraph(description string) string {
	description = strings.ReplaceAll(description, "\r\n", "\n")
	parts := paragraphs(description)
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if !isChumMeta(last) {
		return ""
	}
	return last
}

func metaScalar(paragraph, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(paragraph, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.Trim(strings.TrimSpace(line[len(prefix):]), `"'`)
		if val == "" || strings.HasPrefix(val, "[") {
			return ""
		}
		return val
	}
	return ""
}

func metaList(paragraph, key string) []string {
	lines := strings.Split(paragraph, "\n")
	prefix := key + ":"
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, prefix) {
			continue
		}
		rest := strings.TrimSpace(trim[len(prefix):])
		if rest != "" {
			return splitMetaList(rest)
		}
		var items []string
		for _, next := range lines[i+1:] {
			item := strings.TrimSpace(next)
			if item == "" || !strings.HasPrefix(item, "-") {
				break
			}
			item = strings.Trim(strings.TrimSpace(strings.TrimPrefix(item, "-")), `"'`)
			if item != "" {
				items = append(items, item)
			}
		}
		return items
	}
	return nil
}

func splitMetaList(rest string) []string {
	rest = strings.Trim(rest, "[]")
	var out []string
	for _, part := range strings.Split(rest, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func chumAI(description string) (body, code string) {
	description = strings.ReplaceAll(description, "\r\n", "\n")
	description = strings.TrimRight(description, "\n")
	parts := paragraphs(description)
	if len(parts) == 0 {
		return "", ""
	}
	last := parts[len(parts)-1]
	if !isChumMeta(last) {
		return description, ""
	}
	body = strings.TrimRight(strings.Join(parts[:len(parts)-1], "\n\n"), "\n")
	return body, ratingFrom(last)
}

func paragraphs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := blankLines.Split(s, -1)
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.Trim(part, "\n")
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func isChumMeta(paragraph string) bool {
	for _, line := range strings.Split(paragraph, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return chumMetaLine.MatchString(line)
	}
	return false
}

func ratingFrom(paragraph string) string {
	m := aiRatingLine.FindStringSubmatch(paragraph)
	if m == nil {
		return ""
	}
	return normalizeAI(m[1])
}

func normalizeAI(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, " \t"); i >= 0 {
		raw = raw[:i]
	}
	switch strings.ToUpper(raw) {
	case "H", "A", "B", "C", "V", "X":
		return strings.ToUpper(raw)
	default:
		return ""
	}
}

func appendAIImage(description, code string) string {
	tag := `<img src="` + aiBadgeBase + aiBadgeFile(code) + `">`
	description = strings.TrimRight(description, "\n")
	if description == "" {
		return tag
	}
	return description + "\n\n" + tag
}

func aiBadgeFile(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "H", "A", "B", "C", "V":
		return "AI-score-" + strings.ToUpper(strings.TrimSpace(code)) + ".svg"
	case "X":
		return "AI-score-will-not-tell.svg"
	default:
		return "AI-score-unknown.svg"
	}
}
