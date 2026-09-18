package tabs

import "testing"

func TestAssignLabelsDedupsFirstLetters(t *testing.T) {
	items := []string{"SSH", "Start", "Stop", "Restart", "Pause", "Unpause", "View Logs", "Edit YAML"}
	labels := AssignLabels(items)
	want := []string{"s", "t", "o", "r", "p", "u", "v", "e"}
	if len(labels) != len(want) {
		t.Fatalf("expected %d labels, got %d: %v", len(want), len(labels), labels)
	}
	for i, w := range want {
		if labels[i] != w {
			t.Fatalf("labels[%d]=%q, want %q", i, labels[i], w)
		}
	}
}

func TestAssignLabelsAllUnique(t *testing.T) {
	items := []string{"SSH", "Start", "Stop", "Restart", "Pause", "Unpause", "View Logs", "Edit YAML"}
	labels := AssignLabels(items)
	seen := map[string]bool{}
	for _, l := range labels {
		if l == "" {
			t.Fatal("empty label")
		}
		if seen[l] {
			t.Fatalf("duplicate label %q", l)
		}
		seen[l] = true
	}
}

func TestAssignLabelsShortItems(t *testing.T) {
	labels := AssignLabels([]string{"S", "SSH"})
	if labels[0] != "s" {
		t.Fatalf("labels[0]=%q, want s", labels[0])
	}
	if labels[1] != "h" {
		t.Fatalf("labels[1]=%q, want h (S taken, second S dup, fall to h)", labels[1])
	}
}

func TestAssignLabelsNoLetters(t *testing.T) {
	labels := AssignLabels([]string{"123"})
	if labels[0] != "" {
		t.Fatalf("labels[0]=%q, want empty string for item with no letters", labels[0])
	}
}

func TestLabelIndex(t *testing.T) {
	labels := []string{"s", "t", "o"}
	if idx, ok := LabelIndex('o', labels); !ok || idx != 2 {
		t.Fatalf("LabelIndex(o)=%d,%v, want 2,true", idx, ok)
	}
	if _, ok := LabelIndex('z', labels); ok {
		t.Fatal("expected not found for z")
	}
	if idx, ok := LabelIndex('P', []string{"p"}); !ok || idx != 0 {
		t.Fatalf("LabelIndex(P)=%d,%v, want 0,true (case-insensitive)", idx, ok)
	}
}

func TestAssignLabelsExcludedSkipsReserved(t *testing.T) {
	labels := AssignLabelsExcluded([]string{"Jump", "Kite"}, 'j', 'k')
	if labels[0] != "u" {
		t.Fatalf("labels[0]=%q, want u (j reserved, fall back)", labels[0])
	}
	if labels[1] != "i" {
		t.Fatalf("labels[1]=%q, want i (k reserved, fall back)", labels[1])
	}
}

func TestAssignLabelsExcludedNoFallback(t *testing.T) {
	labels := AssignLabelsExcluded([]string{"J", "K"}, 'j', 'k')
	if labels[0] != "" {
		t.Fatalf("labels[0]=%q, want empty (only letter j reserved)", labels[0])
	}
	if labels[1] != "" {
		t.Fatalf("labels[1]=%q, want empty (only letter k reserved)", labels[1])
	}
}

func TestAssignLabelsExcludedCaseInsensitive(t *testing.T) {
	labels := AssignLabelsExcluded([]string{"JUMP"}, 'J')
	if labels[0] != "u" {
		t.Fatalf("labels[0]=%q, want u (J reserved case-insensitively)", labels[0])
	}
}

func TestAssignLabelsWrapperMatchesOriginal(t *testing.T) {
	items := []string{"SSH", "Start", "Stop", "Restart"}
	want := AssignLabels(items)
	got := AssignLabelsExcluded(items)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AssignLabelsExcluded(%q)=%q, want %q (wrapper must match)", items[i], got[i], want[i])
		}
	}
}

func TestPaginate(t *testing.T) {
	items := make([]string, 12)
	for i := range items {
		items[i] = "item"
	}
	pages := Paginate(items, 9)
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if len(pages[0]) != 9 || len(pages[1]) != 3 {
		t.Fatalf("page sizes: %d,%d, want 9,3", len(pages[0]), len(pages[1]))
	}
}

func TestPaginateEmpty(t *testing.T) {
	pages := Paginate([]string{}, 9)
	if len(pages) != 1 || len(pages[0]) != 0 {
		t.Fatalf("expected single empty page, got %v", pages)
	}
}

func TestPaginateDefaultsPageSize(t *testing.T) {
	items := make([]string, 10)
	pages := Paginate(items, 0)
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages at default size 9, got %d", len(pages))
	}
	if len(pages[0]) != 9 {
		t.Fatalf("page size=%d, want default 9", len(pages[0]))
	}
}
