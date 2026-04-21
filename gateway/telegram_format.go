package gateway

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

const phPrivate = "\uE000"
const phEnd = "\uE001"

var (
	reMultiSpace = regexp.MustCompile(`\s{2,}`)
	reDigitsOnly = regexp.MustCompile(`^\d+$`)
)

// FormatForTelegramReply converts common Markdown-style assistant output into
// Telegram Bot API HTML. Telegram has no real tables; pipe tables and
// space-aligned “tables” become normal message text (<b> + \n line breaks; HTML
// mode has no <br> tag). Fenced
// blocks that look like source code stay in <pre> (Telegram shows those as code
// bubbles with a copy button).
func FormatForTelegramReply(text string) string {
	if text == "" {
		return text
	}
	s := text
	var fenced []string
	s, fenced = extractFencedCodeBlocks(s)
	var inline []string
	s, inline = extractInlineCode(s)
	var tables []string
	s, tables = extractAndFormatTables(s)

	s = html.EscapeString(s)
	s = applyTelegramBold(s)
	s = restoreInlineCodePlaceholders(s, inline)
	s = restoreTablePlaceholders(s, tables)
	s = restoreFencedCodePlaceholders(s, fenced)
	return s
}

func placeholder(kind string, idx int) string {
	return phPrivate + kind + fmt.Sprintf("%d", idx) + phEnd
}

// extractFencedCodeBlocks replaces ```...``` regions with placeholders. The
// opening fence may be followed by an optional language tag on the same line.
func extractFencedCodeBlocks(s string) (string, []string) {
	const fence = "```"
	var blocks []string
	var b strings.Builder
	rest := s
	for {
		idx := strings.Index(rest, fence)
		if idx < 0 {
			b.WriteString(rest)
			return b.String(), blocks
		}
		b.WriteString(rest[:idx])
		afterOpen := rest[idx+len(fence):]
		if nl := strings.IndexByte(afterOpen, '\n'); nl >= 0 {
			afterOpen = afterOpen[nl+1:]
		} else {
			b.WriteString(rest[idx:])
			return b.String(), blocks
		}
		closeIdx := strings.Index(afterOpen, fence)
		if closeIdx < 0 {
			b.WriteString(rest[idx:])
			return b.String(), blocks
		}
		body := strings.TrimSuffix(afterOpen[:closeIdx], "\r")
		body = strings.TrimSuffix(body, "\n")
		blocks = append(blocks, body)
		b.WriteString(placeholder("C", len(blocks)-1))
		rest = afterOpen[closeIdx+len(fence):]
		rest = strings.TrimPrefix(rest, "\n")
		rest = strings.TrimPrefix(rest, "\r\n")
	}
}

func extractInlineCode(s string) (string, []string) {
	var chunks []string
	var b strings.Builder
	i := 0
	for i < len(s) {
		j := strings.IndexByte(s[i:], '`')
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		b.WriteString(s[i:j])
		k := j + 1
		for k < len(s) && s[k] != '`' {
			k++
		}
		if k >= len(s) {
			b.WriteString(s[j:])
			break
		}
		chunks = append(chunks, s[j+1:k])
		b.WriteString(placeholder("I", len(chunks)-1))
		i = k + 1
	}
	return b.String(), chunks
}

func extractAndFormatTables(s string) (string, []string) {
	lines := strings.Split(s, "\n")
	var tables []string
	var out []string
	i := 0
	for i < len(lines) {
		if isGFMTableStart(lines, i) {
			end := scanGFMTableEnd(lines, i)
			tables = append(tables, gfmTableToTelegramHTML(lines[i:end]))
			out = append(out, placeholder("T", len(tables)-1))
			i = end
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return strings.Join(out, "\n"), tables
}

func isGFMTableStart(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	return isGFMTableRow(lines[i]) && isGFMTableSeparator(lines[i+1])
}

func isGFMTableRow(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.Contains(s, "|") {
		return false
	}
	parts := strings.Split(s, "|")
	return len(parts) >= 3
}

func isGFMTableSeparator(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.Contains(s, "|") || !strings.Contains(s, "-") {
		return false
	}
	for _, part := range strings.Split(s, "|") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		for _, r := range p {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func scanGFMTableEnd(lines []string, i int) int {
	j := i
	for j < len(lines) && isGFMTableRow(lines[j]) {
		j++
	}
	return j
}

func parseGFMTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	raw := strings.Split(s, "|")
	out := make([]string, len(raw))
	for i, p := range raw {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func stripMarkdownBoldCell(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "**") && strings.HasSuffix(s, "**") && utf8.RuneCountInString(s) > 4 {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "**"), "**")
		s = strings.TrimSpace(s)
	}
	return s
}

func gfmTableToTelegramHTML(tableLines []string) string {
	if len(tableLines) == 0 {
		return ""
	}
	if len(tableLines) == 1 {
		row := parseGFMTableRow(tableLines[0])
		for i := range row {
			row[i] = stripMarkdownBoldCell(row[i])
		}
		return renderSingleRowCells(row)
	}
	if len(tableLines) == 2 {
		header := parseGFMTableRow(tableLines[0])
		for i := range header {
			header[i] = stripMarkdownBoldCell(header[i])
		}
		return renderSingleRowCells(header)
	}
	header := parseGFMTableRow(tableLines[0])
	for i := range header {
		header[i] = stripMarkdownBoldCell(header[i])
	}
	var body [][]string
	body = append(body, header)
	for _, ln := range tableLines[2:] {
		r := parseGFMTableRow(ln)
		for i := range r {
			r[i] = stripMarkdownBoldCell(r[i])
		}
		body = append(body, r)
	}
	return renderRowsAsTelegramHTML(body)
}

func applyTelegramBold(s string) string {
	// Non-greedy **...** after HTML escape (no nested **).
	const marker = "**"
	var b strings.Builder
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], marker)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		b.WriteString(s[i:j])
		k := strings.Index(s[j+len(marker):], marker)
		if k < 0 {
			b.WriteString(s[j:])
			break
		}
		k += j + len(marker)
		inner := s[j+len(marker) : k]
		if inner == "" {
			b.WriteString(marker)
			i = j + len(marker)
			continue
		}
		b.WriteString("<b>")
		b.WriteString(inner)
		b.WriteString("</b>")
		i = k + len(marker)
	}
	return b.String()
}

func restoreInlineCodePlaceholders(s string, chunks []string) string {
	for i, raw := range chunks {
		ph := placeholder("I", i)
		escaped := html.EscapeString(raw)
		s = strings.ReplaceAll(s, ph, "<code>"+escaped+"</code>")
	}
	return s
}

func restoreTablePlaceholders(s string, tables []string) string {
	for i, htmlBlock := range tables {
		ph := placeholder("T", i)
		s = strings.ReplaceAll(s, ph, htmlBlock)
	}
	return s
}

func restoreFencedCodePlaceholders(s string, blocks []string) string {
	for i, raw := range blocks {
		ph := placeholder("C", i)
		s = strings.ReplaceAll(s, ph, fencedBlockToTelegramHTML(raw))
	}
	return s
}

func fencedBlockToTelegramHTML(raw string) string {
	if looksLikeSourceCode(raw) {
		return "<pre>" + html.EscapeString(raw) + "</pre>"
	}
	rows := parseFencedTabularRows(raw)
	if len(rows) == 1 && len(rows[0]) >= 2 {
		return renderSingleRowCells(rows[0])
	}
	if len(rows) >= 2 && tabularColumnUniform(rows) {
		return renderRowsAsTelegramHTML(rows)
	}
	return "<pre>" + html.EscapeString(raw) + "</pre>"
}

func parseFencedTabularRows(raw string) [][]string {
	var rows [][]string
	for _, ln := range strings.Split(raw, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		cols := reMultiSpace.Split(ln, -1)
		var trimmed []string
		for _, c := range cols {
			c = strings.TrimSpace(c)
			if c != "" {
				trimmed = append(trimmed, c)
			}
		}
		if len(trimmed) > 0 {
			rows = append(rows, trimmed)
		}
	}
	return rows
}

func tabularColumnUniform(rows [][]string) bool {
	if len(rows) < 2 {
		return false
	}
	n := len(rows[0])
	if n < 2 {
		return false
	}
	for _, r := range rows[1:] {
		if len(r) != n {
			return false
		}
	}
	return true
}

func looksLikeSourceCode(s string) bool {
	sl := strings.ToLower(s)
	markers := []string{
		"package ",
		"\nimport ",
		"func ",
		"func(",
		"def ",
		"class ",
		"select ",
		"\nfrom ",
		" where ",
		"#include",
		"console.log",
		"public static",
		"const ",
		"let ",
		"var ",
		"=>",
		"```",
	}
	for _, m := range markers {
		if strings.Contains(sl, m) {
			return true
		}
	}
	if strings.Count(s, ";") >= 2 && strings.Contains(s, "{") {
		return true
	}
	return false
}

func renderSingleRowCells(cells []string) string {
	hi0 := 0
	if len(cells) > 0 && strings.TrimSpace(cells[0]) == "#" {
		hi0 = 1
	}
	if hi0 >= len(cells) {
		return ""
	}
	var b strings.Builder
	for i := hi0; i < len(cells); i++ {
		if i > hi0 {
			b.WriteString(" — ")
		}
		b.WriteString("<b>")
		b.WriteString(html.EscapeString(strings.TrimSpace(cells[i])))
		b.WriteString("</b>")
	}
	return b.String()
}

func renderRowsAsTelegramHTML(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	if len(rows) == 1 {
		return renderSingleRowCells(rows[0])
	}
	h := rows[0]
	var b strings.Builder
	hi0 := 0
	if len(h) > 0 && strings.TrimSpace(h[0]) == "#" {
		hi0 = 1
	}
	for i := hi0; i < len(h); i++ {
		if i > hi0 {
			b.WriteString(" — ")
		}
		b.WriteString("<b>")
		b.WriteString(html.EscapeString(strings.TrimSpace(h[i])))
		b.WriteString("</b>")
	}
	b.WriteByte('\n')
	for _, r := range rows[1:] {
		if len(r) != len(h) {
			continue
		}
		first := strings.TrimSpace(r[0])
		if reDigitsOnly.MatchString(first) {
			b.WriteString("<b>")
			b.WriteString(html.EscapeString(first))
			b.WriteString(".</b>")
			if len(r) > 1 {
				b.WriteString(" ")
				b.WriteString(html.EscapeString(strings.TrimSpace(r[1])))
			}
			for j := 2; j < len(r); j++ {
				b.WriteString(" — ")
				b.WriteString(html.EscapeString(strings.TrimSpace(r[j])))
			}
		} else {
			for j := 0; j < len(r); j++ {
				if j > 0 {
					b.WriteString(" — ")
				}
				b.WriteString(html.EscapeString(strings.TrimSpace(r[j])))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
