// Copyright 2023 Sauce Labs Inc., all rights reserved.

package dashboardfusion

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMergePanels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		base   []Panel
		extra  []Panel
		wanted []Panel
	}{
		{
			name: "replace content",
			base: []Panel{
				{
					"title":   json.RawMessage("Panel1"),
					"type":    json.RawMessage("graph"),
					"gridPos": json.RawMessage(`{"x":0,"y":0,"h":2,"w":6}`),
					"id":      json.RawMessage("1"),
				},
				{
					"title":   json.RawMessage("Panel2"),
					"type":    json.RawMessage("row"),
					"gridPos": json.RawMessage(`{"x":0,"y":2,"h":2,"w":6}`),
					"id":      json.RawMessage("2"),
				},
			},
			extra: []Panel{
				{
					"title":   json.RawMessage("Panel1"),
					"type":    json.RawMessage("graph"),
					"content": json.RawMessage("Wanted Content for Panel1"),
					"id":      json.RawMessage("3"),
				},
			},
			wanted: []Panel{
				{
					"title":   json.RawMessage("Panel1"),
					"type":    json.RawMessage("graph"),
					"content": json.RawMessage("Wanted Content for Panel1"),
					"gridPos": json.RawMessage(`{"x":0,"y":0,"h":2,"w":6}`),
					"id":      json.RawMessage("1"),
				},
				{
					"title":   json.RawMessage("Panel2"),
					"type":    json.RawMessage("row"),
					"gridPos": json.RawMessage(`{"x":0,"y":2,"h":2,"w":6}`),
					"id":      json.RawMessage("2"),
				},
			},
		},
		{
			name: "append new panel",
			base: []Panel{
				{
					"title":   json.RawMessage("Panel1"),
					"type":    json.RawMessage("graph"),
					"gridPos": json.RawMessage(`{"x":0,"y":0,"h":3,"w":5}`),
					"id":      json.RawMessage("1"),
				},
				{
					"title":   json.RawMessage("Panel2"),
					"type":    json.RawMessage("row"),
					"gridPos": json.RawMessage(`{"x":0,"y":2,"h":2,"w":6}`),
					"id":      json.RawMessage("2"),
				},
			},
			extra: []Panel{
				{
					"title":   json.RawMessage("Panel3"),
					"type":    json.RawMessage("graph"),
					"context": json.RawMessage("Wanted Content for Panel3"),
					"id":      json.RawMessage("3"),
				},
			},
			wanted: []Panel{
				{
					"title":   json.RawMessage("Panel1"),
					"type":    json.RawMessage("graph"),
					"gridPos": json.RawMessage(`{"x":0,"y":0,"h":3,"w":5}`),
					"id":      json.RawMessage("1"),
				},
				{
					"title":   json.RawMessage("Panel2"),
					"type":    json.RawMessage("row"),
					"gridPos": json.RawMessage(`{"x":0,"y":2,"h":2,"w":6}`),
					"id":      json.RawMessage("2"),
				},
				{
					"title":   json.RawMessage("Panel3"),
					"type":    json.RawMessage("graph"),
					"context": json.RawMessage("Wanted Content for Panel3"),
					"gridPos": json.RawMessage(`{"h":0,"w":0,"x":0,"y":4}`),
					"id":      json.RawMessage("3"),
				},
			},
		},
		{
			name: "preserve gridPos of unmatched ps2 panel",
			base: []Panel{
				{
					"title":   json.RawMessage(`"Feed"`),
					"type":    json.RawMessage(`"graph"`),
					"gridPos": json.RawMessage(`{"h":11,"w":12,"x":12,"y":0}`),
					"id":      json.RawMessage("1"),
				},
			},
			extra: []Panel{
				{
					"title":   json.RawMessage(`"Status"`),
					"type":    json.RawMessage(`"graph"`),
					"gridPos": json.RawMessage(`{"h":8,"w":12,"x":0,"y":12}`),
					"id":      json.RawMessage("2"),
				},
			},
			wanted: []Panel{
				{
					"title":   json.RawMessage(`"Feed"`),
					"type":    json.RawMessage(`"graph"`),
					"gridPos": json.RawMessage(`{"h":11,"w":12,"x":12,"y":0}`),
					"id":      json.RawMessage("1"),
				},
				{
					"title":   json.RawMessage(`"Status"`),
					"type":    json.RawMessage(`"graph"`),
					"gridPos": json.RawMessage(`{"h":8,"w":12,"x":0,"y":12}`),
					"id":      json.RawMessage("2"),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			merged := MergePanels(tc.base, tc.extra)
			if diff := cmp.Diff(tc.wanted, merged); diff != "" {
				t.Fatalf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergePanelsByGroup_unequalRowHeights(t *testing.T) {
	t.Parallel()

	// ps1: Panel A (h=4) with a full-width Panel C just below it (y=4).
	// ps2: Panel B (h=8) placed beside Panel A (same y=0, x=12).
	// After merge, Panel A must be extended to h=8 to match Panel B,
	// so Panel C (full-width) can be placed at y=8 with no gap.
	ps1 := []Panel{
		{
			"title":   json.RawMessage(`"A"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":4,"w":12,"x":0,"y":0}`),
			"id":      json.RawMessage(`1`),
		},
		{
			"title":   json.RawMessage(`"C"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":24,"x":0,"y":4}`),
			"id":      json.RawMessage(`3`),
		},
	}
	ps2 := []Panel{
		{
			"title":   json.RawMessage(`"B"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":8,"w":12,"x":12,"y":0}`),
			"id":      json.RawMessage(`2`),
		},
	}

	wanted := []Panel{
		{
			"title":   json.RawMessage(`"A"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":8,"w":12,"x":0,"y":0}`),
			"id":      json.RawMessage(`1`),
		},
		{
			"title":   json.RawMessage(`"B"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":8,"w":12,"x":12,"y":0}`),
			"id":      json.RawMessage(`2`),
		},
		{
			"title":   json.RawMessage(`"C"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":24,"x":0,"y":8}`),
			"id":      json.RawMessage(`3`),
		},
	}

	merged := MergePanelsByGroup(ps1, ps2, false)
	if diff := cmp.Diff(wanted, merged); diff != "" {
		t.Fatalf("unexpected result (-want +got):\n%s", diff)
	}
}

func TestMergePanelsByGroup_halfWidthPanelsPreservePosition(t *testing.T) {
	t.Parallel()

	// ps1: Panel A (h=4, left) with Panel C (h=3, left) just below it.
	// ps2: Panel B (h=8, right) with Panel D (h=3, right) just below it.
	// Each column set is self-contained: Panel C fits at y=4 (no conflict
	// with Panel B's columns) and Panel D fits at y=8. Panel A should NOT
	// be extended — there is no gap in its column range.
	ps1 := []Panel{
		{
			"title":   json.RawMessage(`"A"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":4,"w":12,"x":0,"y":0}`),
			"id":      json.RawMessage(`1`),
		},
		{
			"title":   json.RawMessage(`"C"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":12,"x":0,"y":4}`),
			"id":      json.RawMessage(`3`),
		},
	}
	ps2 := []Panel{
		{
			"title":   json.RawMessage(`"B"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":8,"w":12,"x":12,"y":0}`),
			"id":      json.RawMessage(`2`),
		},
		{
			"title":   json.RawMessage(`"D"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":12,"x":12,"y":8}`),
			"id":      json.RawMessage(`4`),
		},
	}

	wanted := []Panel{
		{
			"title":   json.RawMessage(`"A"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":4,"w":12,"x":0,"y":0}`),
			"id":      json.RawMessage(`1`),
		},
		{
			"title":   json.RawMessage(`"B"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":8,"w":12,"x":12,"y":0}`),
			"id":      json.RawMessage(`2`),
		},
		{
			"title":   json.RawMessage(`"C"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":12,"x":0,"y":4}`),
			"id":      json.RawMessage(`3`),
		},
		{
			"title":   json.RawMessage(`"D"`),
			"type":    json.RawMessage(`"graph"`),
			"gridPos": json.RawMessage(`{"h":3,"w":12,"x":12,"y":8}`),
			"id":      json.RawMessage(`4`),
		},
	}

	merged := MergePanelsByGroup(ps1, ps2, false)
	if diff := cmp.Diff(wanted, merged); diff != "" {
		t.Fatalf("unexpected result (-want +got):\n%s", diff)
	}
}
