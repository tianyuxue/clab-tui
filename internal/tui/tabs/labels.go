package tabs

import "unicode"

// AssignLabels assigns each item a unique single-letter label. The letter is
// the item's first letter; when that collides with an earlier label, later
// letters of the item are tried in order until an unused one is found. If no
// unique letter exists, the label is "" (that item is unreachable by letter).
func AssignLabels(items []string) []string {
	return AssignLabelsExcluded(items)
}

// AssignLabelsExcluded assigns unique labels the same way AssignLabels does,
// but never assigns a rune in excluded. This lets callers reserve keys that
// are bound to other actions (e.g. navigation keys) so they cannot be labels.
func AssignLabelsExcluded(items []string, excluded ...rune) []string {
	skip := map[rune]bool{}
	for _, r := range excluded {
		skip[unicode.ToLower(r)] = true
	}
	labels := make([]string, len(items))
	used := map[rune]bool{}
	for i, name := range items {
		for _, r := range name {
			lower := unicode.ToLower(r)
			if !unicode.IsLetter(lower) || skip[lower] {
				continue
			}
			if used[lower] {
				continue
			}
			used[lower] = true
			labels[i] = string(lower)
			break
		}
	}
	return labels
}

// AssignPoolLabels assigns labels from a fixed alphabetical pool, in order,
// skipping excluded letters. Unlike AssignLabels it never derives labels from
// the item text, so short or identically-prefixed names (e.g. sw1/sw2) cannot
// collide. Items past the end of the pool get an empty label.
func AssignPoolLabels(n int, excluded ...rune) []string {
	skip := map[rune]bool{}
	for _, r := range excluded {
		skip[unicode.ToLower(r)] = true
	}
	labels := make([]string, n)
	next := 0
	for r := 'a'; r <= 'z' && next < n; r++ {
		if skip[r] {
			continue
		}
		labels[next] = string(r)
		next++
	}
	return labels
}

// LabelIndex returns the index of the item whose label equals r (case-insensitive).
func LabelIndex(r rune, labels []string) (int, bool) {
	lower := unicode.ToLower(r)
	for i, l := range labels {
		if l == string(lower) {
			return i, true
		}
	}
	return 0, false
}

const defaultPageSize = 9

// Paginate splits items into pages of at most perPage items. An empty items
// slice yields a single empty page. A non-positive perPage uses the default
// page size.
func Paginate[T any](items []T, perPage int) [][]T {
	if perPage <= 0 {
		perPage = defaultPageSize
	}
	var pages [][]T
	for start := 0; start < len(items); start += perPage {
		end := start + perPage
		if end > len(items) {
			end = len(items)
		}
		pages = append(pages, items[start:end])
	}
	if len(pages) == 0 {
		pages = [][]T{{}}
	}
	return pages
}
