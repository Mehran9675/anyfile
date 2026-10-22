// Command anyprobe prints AnyFile's verdict for files and walks their
// contents recursively, for exercising the engines without the UI.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/all"
)

var (
	depth = flag.Int("depth", 3, "how deep to explore nested containers")
	limit = flag.Int("limit", 0, "extract at most this many entries per container (0: all, -1: only summarise)")
	out   = flag.String("out", "", "keep extracted files in this folder instead of a temp one")
	match = flag.String("match", "", "only extract entries whose path contains this text")
	deep  = flag.Bool("deep", false, "run the signature carver (deep scan) instead of the format engines")
)

func main() {
	flag.Parse()
	tmp := *out
	if tmp == "" {
		tmp, _ = os.MkdirTemp("", "anyprobe-")
		defer os.RemoveAll(tmp)
	}
	for _, p := range flag.Args() {
		probe(p, filepath.Base(p), tmp, 0)
	}
}

func probe(p, name, tmp string, level int) {
	ind := strings.Repeat("    ", level)
	r := detect.Detect(p, detect.Options{Name: name})
	fmt.Printf("%s%s: %s [.%s] %s %.2f via %s\n", ind, name, r.Label, r.Ext, r.Category, r.Confidence, r.Source)
	for _, n := range r.Notes {
		fmt.Printf("%s  ! %s\n", ind, n)
	}
	if level >= *depth {
		return
	}
	explorer := explore.Explore
	if *deep && level == 0 {
		explorer = explore.DeepScan
	}
	l, err := explorer(p, r)
	if err != nil {
		return
	}
	defer l.Close()
	fmt.Printf("%s  ⤷ %s, %d entries\n", ind, l.Engine, len(l.Entries))
	if *limit < 0 {
		summary(l)
		return
	}
	done := 0
	for i, e := range l.Entries {
		if e.Dir || !strings.Contains(e.Path, *match) {
			continue
		}
		if *limit > 0 && done >= *limit {
			fmt.Printf("%s    ... %d more\n", ind, len(l.Entries)-i)
			break
		}
		done++
		dst := filepath.Join(tmp, fmt.Sprintf("%d_%d_%s", level, i, explore.SafeName(e.Name())))
		if err := explore.WriteTo(e, dst); err != nil {
			fmt.Printf("%s    %s: extract failed: %v\n", ind, e.Path, err)
			continue
		}
		probe(dst, e.Path, tmp, level+1)
	}
}
