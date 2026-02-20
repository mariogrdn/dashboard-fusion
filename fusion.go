// Copyright 2023 Sauce Labs Inc., all rights reserved.

package dashboardfusion

import (
	"bytes"
	"encoding/json"
	"slices"
)

type Dashboard map[string]json.RawMessage

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

type Panel map[string]json.RawMessage

func (p Panel) Equals(p2 Panel) bool {
	return bytes.Equal(p["title"], p2["title"]) &&
		bytes.Equal(p["type"], p2["type"])
}

func (p Panel) IDRaw() json.RawMessage {
	return p["id"]
}

func (p Panel) GridPosRaw() json.RawMessage {
	return p["gridPos"]
}

func (p Panel) TypeRaw() json.RawMessage {
	return p["type"]
}

func (p Panel) TitleRaw() json.RawMessage {
	return p["title"]
}

func (p Panel) PanelsRaw() json.RawMessage {
	return p["panels"]
}

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

// MergePanelsByGroup merges two sets of panels
// first by group and then, if possible, by panels name and type.
// The new panels are appended to either top or bottom of the
// res dashboard based on the value of the 'top' flag.

func MergePanelsByGroup(ps1, ps2 []Panel, top bool) []Panel {
	groupsPs1, rowsPs1 := groupByRow(ps1)
	groupsPs2, rowsPs2 := groupByRow(ps2)

	// merge child panels per group
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

	tmp1 := make([]Panel, 0)
	tmp2 := make([]Panel, 0)
	seen := make(map[string]bool)

	// check what rows belong only to ps2
	var onlyPs2 []string

	for title := range rowsPs2 {
		if _, ok := rowsPs1[title]; !ok {
			onlyPs2 = append(onlyPs2, title)
		}
	}

	// append groups that were only in ps2
	for title, panels := range mergedGroups {
		if slices.Contains(onlyPs2, title) {
			header := rowsPs2[title]
			tmp1 = append(tmp1, header)
			tmp1 = append(tmp1, panels...)
			seen[title] = true
		}
	}

	// preserve order of row headers from ps1
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

				// append header (prefer ps1 header)
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

	res := make([]Panel, 0, len(mergedGroups["none"])+len(tmp1)+len(tmp2))

	// ungrouped panels will always be appended to the top
	// if top is true append the new panels and groups to the top
	// otherwise to the bottom
	res = append(res, mergedGroups["none"]...)
	if top {
		res = append(res, tmp1...)
		res = append(res, tmp2...)
	} else {
		res = append(res, tmp2...)
		res = append(res, tmp1...)
	}

	// Stack groups vertically, preserving within-group 2D layout.
	yOffset := 0

	// Handle ungrouped panels first (before any row header).
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

	// Process remaining panels group by group, separated by row headers.
	i := ungroupedEnd
	for i < len(res) {
		// Place the row header.
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

		// Find the end of this group's child panels (up to next row header or end).
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
				if tr := p.TitleRaw(); tr != nil {
					var title string
					if err := json.Unmarshal(tr, &title); err == nil {
						groupName = title
					}
				}
				groups[groupName] = append(groups[groupName], retrieveEmbeddedPanels(p)...)
				p["panels"], _ = json.Marshal([]Panel{})
				p["collapsed"], _ = json.Marshal(false)
				rows[groupName] = p
			} else {
				groups[groupName] = append(groups[groupName], p)
			}
		}
	}

	return groups, rows
}

func retrieveEmbeddedPanels(p Panel) []Panel {
	if panelsRaw := p.PanelsRaw(); panelsRaw != nil {
		var panels []Panel
		if err := json.Unmarshal(panelsRaw, &panels); err == nil {
			return panels
		}
	}
	return []Panel{}
}
