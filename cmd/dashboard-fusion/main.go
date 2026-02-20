// Copyright 2023 Sauce Labs Inc., all rights reserved.

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

var args = struct {
	dash   *string
	panels *[]string
	out    *string
	top    *bool
	name   *string
}{
	dash:   pflag.String("dash", "", "Location of base dashboard [required]"),
	panels: pflag.StringSlice("panels", []string{}, "Location of panel(s) to be merged into base dashboard [required]"),
	out:    pflag.String("out", "", "Location of output dashboard, defaults to stdout"),
	top:    pflag.Bool("top", false, "Append new panels to the top instead of bottom of the destination dashboard"),
	name:   pflag.String("name", "", "Title of the resulting dashboard [required]"),
}

func generateUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	if !pflag.Parsed() {
		pflag.Parse()
	}
	if *args.dash == "" || len(*args.panels) == 0 || *args.name == "" {
		pflag.Usage()
		return
	}

	d, err := readFromFile[fusion.Dashboard](*args.dash)
	if err != nil {
		log.Fatal("reading dashboard ", err)
	}

	ps := d.Panels()
	for i := range *args.panels {
		ps2, err := readFromFile[[]fusion.Panel]((*args.panels)[i])
		if err != nil {
			dd, err2 := readFromFile[fusion.Dashboard]((*args.panels)[i])
			if err2 != nil {
				log.Fatal("reading panels ", err, err2)
			}
			ps2 = dd.Panels()
		}

		ps = fusion.MergePanelsByGroup(ps, ps2, *args.top)
	}

	d["panels"], err = json.Marshal(ps)
	if err != nil {
		log.Fatal("marshalling merged panels ", err)
	}

	d["uid"], err = json.Marshal(generateUID())
	if err != nil {
		log.Fatal("marshalling uid ", err)
	}

	d["title"], err = json.Marshal(*args.name)
	if err != nil {
		log.Fatal("marshalling title ", err)
	}

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
