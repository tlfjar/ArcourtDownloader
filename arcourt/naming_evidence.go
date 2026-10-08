package arcourt

import (
	"context"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	pdf "github.com/giraffesyo/pdf"
)

// NamingEvidence is a bounded, local view of the opening pages. Reason is empty
// when text is usable. No PDF bytes, images, URLs, or parser errors leave here.
type NamingEvidence struct {
	Small    string // targeted evidence, at most 512 characters / 2 KiB
	Medium   string // targeted evidence, at most 1,024 characters / 4 KiB
	Baseline string // opening-page baseline, at most 4,096 characters / 8 KiB
	Pages    int    // pages inspected, at most two
	Reason   string // too_large, encrypted, image_only, unreadable, timeout, or canceled
}

const (
	maxNamingPDFBytes    = 16 << 20
	maxNamingReadBytes   = 64 << 20
	maxNamingPages       = 2
	maxNamingPageGlyphs  = 16000
	maxNamingStreamBytes = 1 << 20
	maxNamingOperators   = 100000
	maxNamingTextChars   = 4096
	maxNamingTextBytes   = 8192
	namingExtractionTime = 5 * time.Second
)

var namingTypeWord = regexp.MustCompile(`(?i)\b(motion|order|response|reply|complaint|petition|affidavit|brief|notice|exhibit|summons|subpoena|judgment|writ|certificate|objection|application|memorandum|stipulation|declaration|pleading|answer|discovery|interrogator(?:y|ies))\b`)
var namingMaterialQualifier = regexp.MustCompile(`(?i)\b(response|reply|proposed|entered|amended|first|second|third|fourth|granting|denying)\b`)
var namingResponseOrReply = regexp.MustCompile(`(?i)\b(response|reply)\b`)
var namingStatusQualifier = regexp.MustCompile(`(?i)\b(proposed|entered|amended|first|second|third|fourth)\b`)

// ExtractNamingEvidence reads only the first two pages of a complete local PDF.
// The parser traverses the page tree to locate them (up to its 50,000-node,
// depth-64 structural ceiling); it never extracts other pages. Large files
// and files that exceed parser work/read limits fall back locally.
func ExtractNamingEvidence(ctx context.Context, file io.ReaderAt, size int64) (out NamingEvidence) {
	if err := ctx.Err(); err != nil {
		out.Reason = namingContextReason(err)
		return out
	}
	if file == nil || size < 8 {
		out.Reason = "unreadable"
		return out
	}
	if size > maxNamingPDFBytes {
		out.Reason = "too_large"
		return out
	}
	// Optional content inspection must not turn a saved PDF into a failed
	// download if a parser encounters a malformed construct.
	defer func() {
		if recover() != nil {
			out = NamingEvidence{Reason: "unreadable"}
		}
	}()
	parseCtx, cancel := context.WithTimeout(ctx, namingExtractionTime)
	defer cancel()
	readCount := new(atomic.Int64)
	doc, err := pdf.ExtractWithOptions(parseCtx, namingReaderAt{ctx: parseCtx, source: file, size: size, used: readCount}, size, pdf.Options{
		Pages:           []pdf.PageRange{{First: 1, Last: maxNamingPages}},
		Concurrency:     1,
		IgnoreArtifacts: true,
		Limits: pdf.Limits{
			MaxStreamBytes:       maxNamingStreamBytes,
			MaxOperatorsPerPage:  maxNamingOperators,
			MaxGlyphsPerPage:     maxNamingPageGlyphs,
			MaxFormDepth:         8,
			MaxImagesPerPage:     64,
			MaxImageBytesPerPage: 1 << 20,
			MaxImagePixels:       1 << 20,
		},
	})
	if parseCtx.Err() != nil {
		out.Reason = namingContextReason(parseCtx.Err())
		return out
	}
	if errors.Is(err, pdf.ErrPasswordRequired) {
		out.Reason = "encrypted"
		return out
	}
	if err != nil || doc == nil || len(doc.Pages) == 0 || len(doc.Warnings) != 0 {
		out.Reason = "unreadable"
		return out
	}
	out.Pages = len(doc.Pages)
	pages := make([][]namingLine, 0, out.Pages)
	imageCount := 0
	for _, page := range doc.Pages {
		if len(page.Warnings) != 0 {
			return NamingEvidence{Pages: out.Pages, Reason: "unreadable"}
		}
		imageCount += page.ImageCount
		pages = append(pages, namingPageLines(page.Text()))
		if parseCtx.Err() != nil {
			return NamingEvidence{Pages: out.Pages, Reason: namingContextReason(parseCtx.Err())}
		}
	}
	all := flattenNamingLines(pages, false)
	substantive := flattenNamingLines(pages, true)
	useful := strings.Join(substantive, " ")
	if len(substantive) == 0 || (utf8.RuneCountInString(useful) < 8 && !namingTypeWord.MatchString(useful)) ||
		(imageCount > 0 && utf8.RuneCountInString(useful) < 40) {
		out.Reason = "unreadable"
		if imageCount > 0 {
			out.Reason = "image_only"
		}
		return out
	}
	out.Baseline = limitNamingText(strings.Join(all, "\n"), maxNamingTextChars, maxNamingTextBytes)
	targeted := selectNamingLines(pages)
	out.Small = limitNamingText(strings.Join(targeted, "\n"), 512, 2048)
	out.Medium = limitNamingText(strings.Join(targeted, "\n"), 1024, 4096)
	if strings.TrimSpace(out.Small) == "" {
		out.Reason = "unreadable"
	}
	return out
}

func extractNamingEvidence(ctx context.Context, file io.ReaderAt, size int64) NamingEvidence {
	return ExtractNamingEvidence(ctx, file, size)
}

func namingContextReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "canceled"
}

// ReaderAt checks cancellation and keeps parser reads inside the declared
// bounded local file. A normal os.File supplies the underlying ReaderAt.
type namingReaderAt struct {
	ctx    context.Context
	source io.ReaderAt
	size   int64
	used   *atomic.Int64
}

func (r namingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off < 0 || off > r.size {
		return 0, io.EOF
	}
	short := int64(len(p)) > r.size-off
	if short {
		p = p[:int(r.size-off)]
	}
	if len(p) == 0 {
		return 0, io.EOF
	}
	if r.used != nil && r.used.Add(int64(len(p))) > maxNamingReadBytes {
		return 0, errors.New("PDF naming read budget exceeded")
	}
	n, err := r.source.ReadAt(p, off)
	if short && err == nil {
		err = io.EOF
	}
	return n, err
}

type namingLine struct {
	text string
}

func namingPageLines(raw string) []namingLine {
	var lines []namingLine
	for _, part := range strings.Split(raw, "\n") {
		var b strings.Builder
		for _, r := range part {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				continue
			}
			b.WriteRune(r)
		}
		line := strings.Join(strings.Fields(b.String()), " ")
		if line == "" {
			continue
		}
		lines = append(lines, namingLine{text: line})
	}
	return lines
}

func flattenNamingLines(pages [][]namingLine, discardBoilerplate bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, page := range pages {
		for _, line := range page {
			if discardBoilerplate && isNamingBoilerplate(line.text) {
				continue
			}
			key := strings.ToLower(line.text)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, line.text)
		}
	}
	return out
}

func isNamingBoilerplate(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(lower, "in the circuit court"),
		strings.HasPrefix(lower, "in the district court"),
		strings.HasPrefix(lower, "state of arkansas"),
		strings.HasPrefix(lower, "county of "),
		strings.HasPrefix(lower, "case no"),
		strings.HasPrefix(lower, "case number"),
		strings.HasPrefix(lower, "civil action no"),
		strings.HasPrefix(lower, "filed: "),
		strings.HasPrefix(lower, "filed "),
		strings.HasPrefix(lower, "electronically filed"),
		strings.HasPrefix(lower, "respectfully submitted"),
		strings.HasPrefix(lower, "attorney for"),
		strings.HasPrefix(lower, "telephone:"),
		strings.HasPrefix(lower, "phone:"),
		strings.HasPrefix(lower, "email:"),
		strings.HasPrefix(lower, "address:"):
		return true
	case lower == "plaintiff", lower == "defendant", lower == "petitioner", lower == "respondent", lower == "v.", lower == "vs.":
		return true
	case strings.HasPrefix(lower, "page ") && len(lower) < 16:
		return true
	}
	return false
}

func selectNamingLines(pages [][]namingLine) []string {
	bestPage, bestIndex, bestScore := -1, -1, -1
	for pi, page := range pages {
		for i, line := range page {
			if isNamingBoilerplate(line.text) || utf8.RuneCountInString(line.text) > 180 {
				continue
			}
			if !namingTypeWord.MatchString(line.text) {
				continue
			}
			score := 30
			if i < 25 {
				score += 20 - i/2
			}
			if pi == 0 {
				score += 5
			}
			if isMostlyUpper(line.text) {
				score += 20
			}
			// An adjudicative title at the very start is often
			// followed by an all-caps quotation of the filing being ruled on.
			// Preserve the opening title in that layout rather than treating
			// the quoted response as the document's own heading.
			if pi == 0 && i == 0 &&
				(namingStartsWithType(line.text, "order") || namingStartsWithType(line.text, "judgment")) {
				score += 50
			}
			// A response, reply, amended filing, or proposed/entered order
			// names a more specific document than an earlier quoted motion.
			if namingResponseOrReply.MatchString(line.text) {
				score += 24
			} else if namingStatusQualifier.MatchString(line.text) {
				score += 15
			} else if namingMaterialQualifier.MatchString(line.text) {
				score += 8
			}
			if strings.ContainsAny(line.text, ".;:") {
				score -= 3
			}
			if score > bestScore {
				bestPage, bestIndex, bestScore = pi, i, score
			}
		}
	}
	if bestPage < 0 {
		return flattenNamingLines(pages, true)
	}
	page := pages[bestPage]
	selected := []string{page[bestIndex].text}
	// A material qualifier may be its own heading line immediately before
	// the document type (for example PROPOSED / ORDER or SECOND AMENDED /
	// COMPLAINT). Do not pull arbitrary caption names into the excerpt.
	for i := bestIndex - 1; i >= 0 && i >= bestIndex-2; i-- {
		previous := page[i].text
		if isNamingBoilerplate(previous) || !isMostlyUpper(previous) ||
			utf8.RuneCountInString(previous) > 80 || !namingMaterialQualifier.MatchString(previous) {
			break
		}
		selected = append([]string{previous}, selected...)
	}
	seenSelected := map[string]bool{}
	for _, line := range selected {
		seenSelected[strings.ToLower(line)] = true
	}
	// Court titles often wrap over several all-capitals lines. Include those
	// lines intact before taking adjacent explanatory text.
	last := bestIndex
	for i := bestIndex + 1; i < len(page) && i <= bestIndex+3; i++ {
		next := page[i].text
		if isNamingBoilerplate(next) || !isMostlyUpper(next) || utf8.RuneCountInString(next) > 120 {
			break
		}
		key := strings.ToLower(next)
		if !seenSelected[key] {
			selected = append(selected, next)
			seenSelected[key] = true
		}
		last = i
	}
	type passage struct {
		text  string
		score int
	}
	var nearby []passage
	for i := last + 1; i < len(page) && i <= last+12; i++ {
		text := page[i].text
		if isNamingBoilerplate(text) {
			continue
		}
		score := 0
		if namingTypeWord.MatchString(text) {
			score += 2
		}
		if namingMaterialQualifier.MatchString(text) {
			score++
		}
		nearby = append(nearby, passage{text: text, score: score})
	}
	// Keep nearby lines, but put those establishing type or qualifiers before
	// generic background. This avoids sending a page of caption/filler before
	// the one sentence that distinguishes a motion from its response.
	sort.SliceStable(nearby, func(i, j int) bool { return nearby[i].score > nearby[j].score })
	for _, line := range nearby {
		key := strings.ToLower(line.text)
		if !seenSelected[key] {
			selected = append(selected, line.text)
			seenSelected[key] = true
		}
	}
	// A heading at the end of page one may need its opening paragraph on page
	// two. The total evidence budget still applies below.
	if len(selected) < 3 && bestPage+1 < len(pages) {
		for _, line := range pages[bestPage+1] {
			key := strings.ToLower(line.text)
			if !isNamingBoilerplate(line.text) && !seenSelected[key] {
				selected = append(selected, line.text)
				seenSelected[key] = true
			}
			if len(selected) >= 5 {
				break
			}
		}
	}
	return selected
}

func namingStartsWithType(line, kind string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, kind+" ") || lower == kind
}

func isMostlyUpper(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 4 && upper*5 >= letters*4
}

func limitNamingText(s string, characters, bytes int) string {
	if utf8.RuneCountInString(s) <= characters && len(s) <= bytes {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	for _, r := range s {
		width := utf8.RuneLen(r)
		if characters == 0 || width > bytes {
			break
		}
		b.WriteRune(r)
		characters--
		bytes -= width
	}
	return strings.TrimSpace(b.String())
}
