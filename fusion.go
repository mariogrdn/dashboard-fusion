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
			g := GridPos{
				H: 2,
				W: 6,
				X: 0,
				Y: maxY + 1,
			}
			graw, err := json.Marshal(g)
			if err != nil {
				panic(err)
			}
			p2["gridPos"] = graw

			res = append(res, p2)
			maxY += g.H
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

	// make the grid positions consistent
	const maxWidth = 24
	currentY := 0
	currentRowWidth := 0
	currentRowMaxBottom := 0 // Track tallest panel in row for next Y

	for i := range res {
		panel := res[i]
		pos := panel.GridPos()
		if currentRowWidth+pos.W > maxWidth {
			// New row
			currentY += currentRowMaxBottom
			currentRowWidth = 0
			currentRowMaxBottom = 0
		}
		// Place at next X in row
		pos.X = currentRowWidth
		pos.Y = currentY
		posRaw, err := json.Marshal(pos)
		if err != nil {
			panic(err)
		}
		panel["gridPos"] = posRaw
		res[i] = panel
		// Update row tracking
		currentRowWidth += pos.W
		if pos.H > currentRowMaxBottom {
			currentRowMaxBottom = pos.H
		}
	}
	return res
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
