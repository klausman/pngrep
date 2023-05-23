// Simple PNG grepper
//
// Copyright 2023 Tobias Klausmann
// Licensed under the GPLv3, see COPYING for details
//
// Searches for the supplied regex in the text (tEXt) chunks of the supplied
// PNG images. If a match is found, prints the filename.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

var (
	caseins   = flag.Bool("i", false, "Make regexp case-insensitive")
	showmatch = flag.Bool("w", false, "Show matching text chunks")
)

func main() {
	ret := 1
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [options] <regex> <file> [file, ...]\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(-1)
	}
	re := args[0]
	if *caseins {
		re = "(?i)" + re
	}
	rx, err := regexp.Compile(re)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid regexp '%s': %s\n", re, err)
		os.Exit(2)
	}
	for _, filename := range args[1:] {
		found, chunks, err := grepOneFile(filename, rx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			ret = 2
			break
		}
		if found {
			fmt.Println(filename)
			if *showmatch {
				for _, m := range chunks {
					fmt.Printf("%#v\n", m)
				}
			}
			ret = 0
		}
	}
	os.Exit(ret)
}

func grepOneFile(filename string, rx *regexp.Regexp) (bool, []string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return false, []string{}, err
	}
	defer file.Close()
	found, chunk, err := grePNG(file, rx)
	if err != nil {
		return false, []string{}, err
	}
	return found, chunk, nil
}

func grePNG(r io.Reader, rx *regexp.Regexp) (bool, []string, error) {
	var chunks []string
	png, err := Load(r)
	if err != nil {
		return false, chunks, err
	}

	for _, tc := range png.GetTextChunks() {
		ret := rx.FindStringIndex(tc)
		if ret != nil {
			chunks = append(chunks, tc)
		}
	}
	return len(chunks) > 0, chunks, nil
}
