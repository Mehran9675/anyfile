package main

import (
	"fmt"
	"path"
	"sort"

	"github.com/mehran9675/anyfile/internal/explore"
)

// summary prints how many entries of each top-level folder a listing has.
func summary(l *explore.Listing) {
	counts := map[string]int{}
	for _, e := range l.Entries {
		if !e.Dir {
			top := e.Path
			for path.Dir(top) != "." {
				top = path.Dir(top)
			}
			counts[top]++
		}
	}
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("      %s: %d\n", k, counts[k])
	}
}
