// Copyright 2023 Sauce Labs Inc., all rights reserved.

// dashboard-fusion is a CLI tool that merges Grafana dashboard panel definitions
// from multiple JSON files into a single dashboard.
//
// It reads a base dashboard, merges one or more panel sources into it (matching
// existing panels by title+type, appending new ones), assigns a new UID and
// title, and writes the result to a file or stdout.
//
// Usage:
//
//	dashboard-fusion --dash base.json --panels overlay1.json,overlay2.json --name "My Dashboard" [--out merged.json] [--top]
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"

	fusion "github.com/saucelabs/dashboard-fusion"
	"github.com/spf13/pflag"
)

// args holds the parsed command-line flags.
var args = struct {
	dash   *string   // Path to the base dashboard JSON file.
	panels *[]string // Paths to panel JSON files (or full dashboards) to merge in.
	out    *string   // Output file path; empty means write to stdout.
	top    *bool     // If true, new groups are placed above existing ones.
	name   *string   // Title to assign to the resulting merged dashboard.
}{
	dash:   pflag.String("dash", "", "Location of base dashboard [required]"),
	panels: pflag.StringSlice("panels", []string{}, "Location of panel(s) to be merged into base dashboard [required]"),
	out:    pflag.String("out", "", "Location of output dashboard, defaults to stdout"),
	top:    pflag.Bool("top", false, "Append new panels to the top instead of bottom of the destination dashboard"),
	name:   pflag.String("name", "", "Title of the resulting dashboard [required]"),
}

// generateUID creates a random UUID v4 string to serve as the dashboard's
// unique identifier. Each merged dashboard gets a fresh UID so it can be
// imported into Grafana without conflicting with existing dashboards.
func generateUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Set version nibble to 4 (UUID v4).
	b[8] = (b[8] & 0x3f) | 0x80 // Set variant bits to RFC 4122.
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	// Parse CLI flags and validate required arguments.
	if !pflag.Parsed() {
		pflag.Parse()
	}
	if *args.dash == "" || len(*args.panels) == 0 || *args.name == "" {
		pflag.Usage()
		return
	}

	// Load the base dashboard from disk.
	d, err := readFromFile[fusion.Dashboard](*args.dash)
	if err != nil {
		log.Fatal("reading dashboard ", err)
	}

	// Merge each panel source into the base dashboard's panels, one at a time.
	// Each source file can be either a bare panel array or a full dashboard
	// JSON (in which case we extract its "panels" field).
	ps := d.Panels()
	for i := range *args.panels {
		ps2, err := readFromFile[[]fusion.Panel]((*args.panels)[i])
		if err != nil {
			// Not a panel array — try reading as a full dashboard instead.
			dd, err2 := readFromFile[fusion.Dashboard]((*args.panels)[i])
			if err2 != nil {
				log.Fatal("reading panels ", err, err2)
			}
			ps2 = dd.Panels()
		}

		ps = fusion.MergePanelsByGroup(ps, ps2, *args.top)
	}

	// Write the merged panels back into the base dashboard object.
	d["panels"], err = json.Marshal(ps)
	if err != nil {
		log.Fatal("marshalling merged panels ", err)
	}

	// Assign a fresh UID so the merged dashboard won't collide with any
	// existing dashboard when imported into Grafana.
	d["uid"], err = json.Marshal(generateUID())
	if err != nil {
		log.Fatal("marshalling uid ", err)
	}

	// Set the dashboard title to the user-provided name.
	d["title"], err = json.Marshal(*args.name)
	if err != nil {
		log.Fatal("marshalling title ", err)
	}

	// Write the resulting dashboard JSON to the output file (or stdout).
	var out *os.File
	if *args.out != "" {
		var err error
		out, err = os.Create(*args.out)
		if err != nil {
			log.Fatal("creating output dashboard ", err)
		}
		defer func() {
			if err := out.Close(); err != nil {
				panic(err)
			}
		}()
	} else {
		out = os.Stdout
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(d); err != nil {
		log.Println("encoding output dashboard ", err)
	}
}

// readFromFile reads and JSON-decodes a file into a value of type T.
// T is typically fusion.Dashboard or []fusion.Panel.
func readFromFile[T any](filename string) (T, error) {
	var obj T

	f, err := os.Open(filename)
	if err != nil {
		return obj, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	if err := json.NewDecoder(f).Decode(&obj); err != nil {
		return obj, err
	}

	return obj, nil
}
