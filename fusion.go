// Copyright 2023 Sauce Labs Inc., all rights reserved.

// Package dashboardfusion provides utilities for merging Grafana dashboard
// panel definitions. It operates on the JSON structure of Grafana dashboards,
// allowing panels from multiple sources to be combined into a single dashboard
// while preserving layout positions, panel identities, and row grouping.
//
// Grafana dashboards use a 24-column grid system where each panel has a
// position (x, y) and dimensions (width, height). This package handles the
// complexity of reconciling positions when panels from different dashboards
// are merged together.
package dashboardfusion

import (
	"bytes"
	"encoding/json"
	"slices"
)

// Dashboard represents a Grafana dashboard as a loosely-typed JSON object.
// Keys correspond to top-level dashboard fields (e.g. "panels", "title", "uid").
// Using json.RawMessage as values allows the merge logic to manipulate only
// the fields it cares about while passing everything else through unchanged.
type Dashboard map[string]json.RawMessage

// Panels extracts and returns the list of panels from the dashboard.
// Returns nil if the dashboard has no "panels" field.
func (d Dashboard) Panels() []Panel {
	if ps, ok := d["panels"]; ok {
		var panels []Panel
		if err := json.Unmarshal(ps, &panels); err != nil {
			panic(err)
		}
		return panels
	}

	return nil
}

// Panel represents a single Grafana dashboard panel as a loosely-typed JSON
// object. Common fields include "title", "type", "id", "gridPos", and
// "panels" (for row-type panels that contain collapsed child panels).
type Panel map[string]json.RawMessage

// Equals reports whether two panels represent the same logical panel.
// Two panels match if they share the same title and type, regardless of
// other fields like id, gridPos, or content.
func (p Panel) Equals(p2 Panel) bool {
	return bytes.Equal(p["title"], p2["title"]) &&
		bytes.Equal(p["type"], p2["type"])
}

// IDRaw returns the raw JSON value of the panel's "id" field.
func (p Panel) IDRaw() json.RawMessage {
	return p["id"]
}

// GridPosRaw returns the raw JSON value of the panel's "gridPos" field.
func (p Panel) GridPosRaw() json.RawMessage {
	return p["gridPos"]
}

// TypeRaw returns the raw JSON value of the panel's "type" field.
// Panel types include "graph", "table", "stat", "row", etc.
func (p Panel) TypeRaw() json.RawMessage {
	return p["type"]
}

// TitleRaw returns the raw JSON value of the panel's "title" field.
func (p Panel) TitleRaw() json.RawMessage {
	return p["title"]
}

// PanelsRaw returns the raw JSON value of the panel's "panels" field.
// This is only relevant for "row"-type panels, which can contain
// collapsed child panels nested inside them.
func (p Panel) PanelsRaw() json.RawMessage {
	return p["panels"]
}

// GridPos parses and returns the panel's grid position as a typed struct.
// Returns a zero-value GridPos if the field is absent.
func (p Panel) GridPos() GridPos {
	if gp, ok := p["gridPos"]; ok {
		var gridPos GridPos
		if err := json.Unmarshal(gp, &gridPos); err != nil {
			panic(err)
		}
		return gridPos
	}

	return GridPos{}
}

// GridPos describes a panel's position and size on Grafana's 24-column grid.
//   - X, Y: top-left corner of the panel (X: column 0-23, Y: row from top)
//   - W, H: width (in columns, max 24) and height (in grid units)
type GridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// MergePanels merges two sets of panels.
//
// If a panel in ps2 matches a panel in ps1, the panel in ps2 overwrites the
// content of the panel in ps1, but preserves its position and id.
//
// If a panel in ps2 does not match any panel in ps1 it is appended and placed at the end of the dashboard.
func MergePanels(ps1, ps2 []Panel) []Panel {
	var maxY int
	res := make([]Panel, 0, len(ps1)+len(ps2))
	for _, p1 := range ps1 {
		if gp := p1.GridPos(); gp.Y+gp.H > maxY {
			maxY = gp.Y + gp.H
		}
		res = append(res, p1)
	}

	for len(ps2) > 0 {
		p2 := ps2[0]
		ps2 = ps2[1:]

		var matched bool
		for i := range res {
			if res[i].Equals(p2) {
				// When we find a match, the panel's content is overwritten,
				// except for the gridPos(to preserve the layout) and id.
				p2["gridPos"], p2["id"] = res[i].GridPosRaw(), res[i].IDRaw()
				res[i] = p2
				matched = true
			}
		}

		if !matched {
			if _, hasGridPos := p2["gridPos"]; !hasGridPos {
				// No position info: place at the end of the current layout.
				g := p2.GridPos()
				g.X = 0
				g.Y = maxY
				graw, err := json.Marshal(g)
				if err != nil {
					panic(err)
				}
				p2["gridPos"] = graw
				maxY += g.H
			} else {
				// Keep the original gridPos; update maxY so subsequent
				// positionless panels are placed below this one.
				if gp := p2.GridPos(); gp.Y+gp.H > maxY {
					maxY = gp.Y + gp.H
				}
			}
			res = append(res, p2)
		}
	}

	return res
}

// MergePanelsByGroup merges two sets of panels using row-based grouping.
//
// Panels are first grouped by their enclosing row header (panels of type "row").
// Groups with the same row title are merged using MergePanels (matching by
// title+type within each group). Groups that exist only in ps2 are treated as
// new and appended.
//
// The 'top' flag controls where new groups (those only in ps2) are placed:
//   - top=true:  new groups appear above existing groups
//   - top=false: new groups appear below existing groups
//
// Ungrouped panels (those not under any row header) always appear at the top.
// After assembly, all panels are re-positioned vertically so groups stack
// without overlaps, while preserving the 2D layout within each group.
func MergePanelsByGroup(ps1, ps2 []Panel, top bool) []Panel {
	// Split both panel sets into groups keyed by row title.
	// "none" is used as the key for panels that appear before any row header.
	groupsPs1, rowsPs1 := groupByRow(ps1)
	groupsPs2, rowsPs2 := groupByRow(ps2)

	// Phase 1: Merge child panels for groups that exist in both sets.
	// Groups only in one set are carried over as-is.
	mergedGroups := make(map[string][]Panel)
	for name, g1 := range groupsPs1 {
		if g2, ok := groupsPs2[name]; ok {
			mergedGroups[name] = MergePanels(g1, g2)
		} else {
			mergedGroups[name] = g1
		}
	}
	for name, g2 := range groupsPs2 {
		if _, ok := mergedGroups[name]; !ok {
			mergedGroups[name] = g2
		}
	}

	// Phase 2: Assemble the final panel list by interleaving row headers
	// with their merged child panels.
	//
	// tmp1 holds groups that are new (only in ps2).
	// tmp2 holds groups that existed in ps1 (preserving ps1's row order).
	tmp1 := make([]Panel, 0)
	tmp2 := make([]Panel, 0)
	seen := make(map[string]bool)

	// Identify which rows exist only in ps2 (i.e. entirely new groups).
	var onlyPs2 []string
	for title := range rowsPs2 {
		if _, ok := rowsPs1[title]; !ok {
			onlyPs2 = append(onlyPs2, title)
		}
	}

	// Collect new groups (ps2-only) into tmp1 with their row headers.
	for title, panels := range mergedGroups {
		if slices.Contains(onlyPs2, title) {
			header := rowsPs2[title]
			tmp1 = append(tmp1, header)
			tmp1 = append(tmp1, panels...)
			seen[title] = true
		}
	}

	// Walk ps1's panels in order to preserve the original row ordering.
	// For each row header encountered, emit the header followed by the
	// merged group's child panels into tmp2.
	for _, p := range ps1 {
		if t := p.TypeRaw(); t != nil {
			var panelType string
			if err := json.Unmarshal(t, &panelType); err != nil {
				continue
			}

			if panelType == "row" {
				var title string
				if tr := p.TitleRaw(); tr != nil {
					_ = json.Unmarshal(tr, &title)
				} else {
					title = "none"
				}

				// Use the ps1 row header if available; fall back to ps2's.
				if header, ok := rowsPs1[title]; ok {
					tmp2 = append(tmp2, header)
				} else if header, ok := rowsPs2[title]; ok {
					tmp2 = append(tmp2, header)
				} else {
					tmp2 = append(tmp2, p)
				}

				if !seen[title] {
					if panels, ok := mergedGroups[title]; ok {
						tmp2 = append(tmp2, panels...)
					}
					seen[title] = true
				}
			}
		}
	}

	// Phase 3: Combine everything into the final result.
	// Ungrouped panels ("none" group) always go first.
	// The 'top' flag determines whether new groups (tmp1) appear
	// before or after existing groups (tmp2).
	res := make([]Panel, 0, len(mergedGroups["none"])+len(tmp1)+len(tmp2))
	res = append(res, mergedGroups["none"]...)
	if top {
		res = append(res, tmp1...)
		res = append(res, tmp2...)
	} else {
		res = append(res, tmp2...)
		res = append(res, tmp1...)
	}

	// Phase 4: Re-position all panels vertically so groups stack without
	// overlaps. Each group's internal 2D layout (relative X/Y positions)
	// is preserved, but the group as a whole is shifted to start
	// immediately after the previous group ends.
	yOffset := 0

	// First, position ungrouped panels (everything before the first row header).
	ungroupedEnd := len(res)
	for j, p := range res {
		if t := p.TypeRaw(); t != nil {
			var panelType string
			if err := json.Unmarshal(t, &panelType); err == nil && panelType == "row" {
				ungroupedEnd = j
				break
			}
		}
	}
	yOffset = shiftGroupToY(res[:ungroupedEnd], yOffset)

	// Then, process each row-delimited group: place the row header as a
	// full-width bar, then shift the group's child panels below it.
	i := ungroupedEnd
	for i < len(res) {
		// Position the row header: full width (24 columns), at current yOffset.
		pos := res[i].GridPos()
		pos.X = 0
		pos.Y = yOffset
		pos.W = 24
		posRaw, err := json.Marshal(pos)
		if err != nil {
			panic(err)
		}
		res[i]["gridPos"] = posRaw
		yOffset += pos.H
		i++

		// Find the extent of this group's child panels (up to the next
		// row header or the end of the slice).
		groupEnd := i
		for j := i; j < len(res); j++ {
			if t := res[j].TypeRaw(); t != nil {
				var panelType string
				if err := json.Unmarshal(t, &panelType); err == nil && panelType == "row" {
					break
				}
			}
			groupEnd = j + 1
		}
		yOffset = shiftGroupToY(res[i:groupEnd], yOffset)
		i = groupEnd
	}
	return res
}

// shiftGroupToY sorts panels by (Y, X), shifts the group to start at targetY,
// resolves cross-column overlaps using a per-column floor map, and then
// extends shorter panels to fill vertical gaps left beside taller neighbours.
// Returns the Y value immediately after the last panel in the group.
func shiftGroupToY(panels []Panel, targetY int) int {
	if len(panels) == 0 {
		return targetY
	}
	slices.SortFunc(panels, func(a, b Panel) int {
		pa, pb := a.GridPos(), b.GridPos()
		if pa.Y != pb.Y {
			return pa.Y - pb.Y
		}
		return pa.X - pb.X
	})
	minY := panels[0].GridPos().Y

	// floor[x] = next available Y at column x.
	floor := make([]int, 24)
	for i := range floor {
		floor[i] = targetY
	}

	maxYH := targetY
	for i, p := range panels {
		pos := p.GridPos()
		idealY := targetY + (pos.Y - minY)

		end := pos.X + pos.W
		if end > 24 {
			end = 24
		}

		// Find the highest floor across the panel's column span.
		floorY := targetY
		for x := pos.X; x < end; x++ {
			if floor[x] > floorY {
				floorY = floor[x]
			}
		}

		// Place at ideal position, but push down if the floor requires it.
		newY := idealY
		if floorY > newY {
			newY = floorY
		}

		pos.Y = newY
		posRaw, err := json.Marshal(pos)
		if err != nil {
			panic(err)
		}
		p["gridPos"] = posRaw
		panels[i] = p

		// Raise the floor for all columns this panel occupies.
		newBottom := newY + pos.H
		for x := pos.X; x < end; x++ {
			if newBottom > floor[x] {
				floor[x] = newBottom
			}
		}
		if newBottom > maxYH {
			maxYH = newBottom
		}
	}

	extendPanelsToFillGaps(panels)
	return maxYH
}

// extendPanelsToFillGaps extends panel heights to fill vertical gaps that arise
// when a taller panel sits beside a shorter one in the same row: without this,
// a wider panel placed below is pushed down to clear the taller neighbour,
// leaving empty space beside the shorter one.
// For each column, consecutive panel pairs are inspected; if a gap exists the
// upper panel is extended downward to reach the lower panel's top edge.
func extendPanelsToFillGaps(panels []Panel) {
	type entry struct{ top, bottom, idx int }

	cols := make([][]entry, 24)
	for i, p := range panels {
		pos := p.GridPos()
		end := pos.X + pos.W
		if end > 24 {
			end = 24
		}
		for x := pos.X; x < end; x++ {
			cols[x] = append(cols[x], entry{pos.Y, pos.Y + pos.H, i})
		}
	}
	for x := range cols {
		slices.SortFunc(cols[x], func(a, b entry) int { return a.top - b.top })
	}

	newH := make([]int, len(panels))
	for i, p := range panels {
		newH[i] = p.GridPos().H
	}
	for x := range cols {
		col := cols[x]
		for k := 0; k+1 < len(col); k++ {
			curr, next := col[k], col[k+1]
			if curr.bottom < next.top {
				needed := next.top - panels[curr.idx].GridPos().Y
				if needed > newH[curr.idx] {
					newH[curr.idx] = needed
				}
			}
		}
	}

	for i, p := range panels {
		pos := p.GridPos()
		if newH[i] == pos.H {
			continue
		}
		pos.H = newH[i]
		posRaw, err := json.Marshal(pos)
		if err != nil {
			panic(err)
		}
		p["gridPos"] = posRaw
		panels[i] = p
	}
}

// groupByRow splits a flat list of panels into groups based on row headers.
//
// In Grafana, panels of type "row" act as collapsible section headers. When a
// row is collapsed, its child panels are stored inside the row's "panels"
// field. When expanded, the child panels appear as siblings after the row
// panel in the flat list.
//
// This function handles both cases:
//   - Collapsed rows: child panels are extracted from the row's embedded "panels" field.
//   - Expanded rows: subsequent non-row panels are collected until the next row header.
//
// Returns two maps keyed by row title:
//   - groups: maps each row title to its child panels (flattened).
//   - rows:   maps each row title to its row header panel (with "panels"
//     cleared and "collapsed" set to false for consistent output).
//
// Panels appearing before any row header are grouped under the key "none".
func groupByRow(ps []Panel) (map[string][]Panel, map[string]Panel) {
	groups := make(map[string][]Panel)
	rows := make(map[string]Panel)
	var groupName string = "none"

	for _, p := range ps {
		if t := p.TypeRaw(); t != nil {
			var panelType string
			if err := json.Unmarshal(t, &panelType); err != nil {
				continue
			}

			if panelType == "row" {
				// Start a new group named after the row's title.
				if tr := p.TitleRaw(); tr != nil {
					var title string
					if err := json.Unmarshal(tr, &title); err == nil {
						groupName = title
					}
				}
				// Extract any child panels embedded inside a collapsed row.
				groups[groupName] = append(groups[groupName], retrieveEmbeddedPanels(p)...)
				// Normalize the row header: clear embedded panels and mark as expanded.
				p["panels"], _ = json.Marshal([]Panel{})
				p["collapsed"], _ = json.Marshal(false)
				rows[groupName] = p
			} else {
				// Non-row panel: belongs to the current group.
				groups[groupName] = append(groups[groupName], p)
			}
		}
	}

	return groups, rows
}

// retrieveEmbeddedPanels extracts child panels from a collapsed row panel.
// Grafana stores child panels inside the row's "panels" field when the row is
// collapsed. Returns an empty slice if the field is absent or unparseable.
func retrieveEmbeddedPanels(p Panel) []Panel {
	if panelsRaw := p.PanelsRaw(); panelsRaw != nil {
		var panels []Panel
		if err := json.Unmarshal(panelsRaw, &panels); err == nil {
			return panels
		}
	}
	return []Panel{}
}
