// Command anyfile is a desktop app that tells you what any file really
// is, lets you explore everything inside it, and hands each item to
// the viewer your system uses for its type.
//
//	anyfile [file]
//	anyfile --version
package main

import (
	"fmt"
	"log"
	"os"

	_ "github.com/mehran9675/anyfile/internal/explore/engines/all"
	"github.com/mehran9675/anyfile/internal/ui"
)

// version is set at build time by the release pipeline
// (-ldflags "-X main.version=v1.2.3").
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("AnyFile", version)
		return
	}
	if err := ui.Run(os.Args[1:], version); err != nil {
		log.Fatal(err)
	}
}
