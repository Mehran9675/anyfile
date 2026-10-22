package detect

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		file string
		ext  string
		cat  Category
	}{
		{"book.xlsx", "xlsx", CatSpreadsheet},
		{"report.dat", "xlsx", CatSpreadsheet}, // wrong extension
		{"bundle.tar.gz", "tar.gz", CatArchive},
		{"notes.txt.gz", "gz", CatCompressed},
		{"data.json", "json", CatData},
		{"doc.pdf", "pdf", CatDocument},
		{"encoded.txt", "b64", CatData},
		{"firmware.bin", "bin", CatData}, // unknown wrapper, embedded objects
		{"mail.eml", "eml", CatEmail},
		{"pic.png", "png", CatImage},
		{"pic.jpg", "jpg", CatImage},
		{"random.bin", "bin", CatEncrypted},
		{"table", "csv", CatSpreadsheet}, // no extension
	}
	for _, c := range cases {
		r := Detect("testdata/"+c.file, Options{})
		if r.Ext != c.ext || r.Category != c.cat {
			t.Errorf("%s: got %s .%s (%s), want .%s (%s)", c.file, r.Label, r.Ext, r.Category, c.ext, c.cat)
		}
	}
}
