package handler

import "testing"

// A batch may contain several clicks on the same link; each one must be
// counted, otherwise short_urls.clicks drifts permanently below the number of
// click_logs rows.
func TestCountClicksPerURL(t *testing.T) {
	events := []ClickEvent{
		{ShortUrlID: 7},
		{ShortUrlID: 7},
		{ShortUrlID: 7},
		{ShortUrlID: 9},
		{ShortUrlID: 0, UID: "unresolved"}, // skipped
		{ShortUrlID: 9},
	}

	got := countClicksPerURL(events)

	if got[7] != 3 {
		t.Errorf("link 7: got %d clicks, want 3", got[7])
	}
	if got[9] != 2 {
		t.Errorf("link 9: got %d clicks, want 2", got[9])
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2 (unresolved events must be skipped)", len(got))
	}

	var total uint32
	for _, n := range got {
		total += n
	}
	if total != 5 {
		t.Errorf("total clicks %d, want 5", total)
	}
}

func TestCountClicksPerURLEmpty(t *testing.T) {
	if got := countClicksPerURL(nil); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
