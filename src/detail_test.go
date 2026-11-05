package main

import "testing"

func TestDetailTableSortFollowsAppSort(t *testing.T) {
	for _, sort := range []string{"name", "name-desc", "size", "size-desc", "date", "date-desc"} {
		if got := sortForTable(tableSortFor(sort)); got != sort {
			t.Fatalf("round trip %q = %q", sort, got)
		}
	}
	for _, sort := range []string{"type", "random", "exif-desc", "created"} {
		if o := tableSortFor(sort); o.Column != "" {
			t.Fatalf("%q shows an arrow on %q", sort, o.Column)
		}
	}
	if o := tableSortFor(""); o.Column != "name" || o.Descending {
		t.Fatalf("default sort = %+v", o)
	}
}
