package ui

import (
	"testing"

	"github.com/mehran9675/anyfile/internal/session"
)

func tree() (root, docs, book, img, notes *session.Node) {
	root = &session.Node{Name: "bundle.zip"}
	docs = &session.Node{Name: "docs", Dir: true, Parent: root}
	book = &session.Node{Name: "Budget.xlsx", Parent: docs, Size: 300}
	img = &session.Node{Name: "photo.png", Parent: root, Size: 900}
	notes = &session.Node{Name: "notes.txt", Parent: root, Size: 10}
	docs.Kids = []*session.Node{book}
	root.Kids = []*session.Node{docs, img, notes}
	return
}

func TestFilterKeepsAncestorsOfMatches(t *testing.T) {
	root, docs, book, img, notes := tree()
	keep, open, hits := viewOptions{query: "budget"}.visibleSet(root)
	if !keep[docs] || !keep[book] || keep[img] || keep[notes] || hits != 1 {
		t.Fatalf("keep=%v hits=%d", keep, hits)
	}
	if len(open) != 2 { // docs and the root hold the match
		t.Fatalf("open = %d nodes, want 2", len(open))
	}
}

func TestTypeFilter(t *testing.T) {
	root, _, book, img, _ := tree()
	images := 0
	for i, g := range typeGroups {
		if g.label == "Images" {
			images = i
		}
	}
	keep, _, hits := viewOptions{group: images}.visibleSet(root)
	if !keep[img] || keep[book] || hits != 1 {
		t.Fatalf("keep=%v hits=%d", keep, hits)
	}
}

func TestSortFoldersFirstBothWays(t *testing.T) {
	root, docs, _, img, notes := tree()
	ns := append([]*session.Node(nil), root.Kids...)
	viewOptions{sort: sortSize, desc: true}.sortNodes(ns)
	if ns[0] != docs || ns[1] != img || ns[2] != notes {
		t.Fatalf("got %s, %s, %s", ns[0].Name, ns[1].Name, ns[2].Name)
	}
	viewOptions{sort: sortName}.sortNodes(ns)
	if ns[0] != docs || ns[1] != notes || ns[2] != img {
		t.Fatalf("got %s, %s, %s", ns[0].Name, ns[1].Name, ns[2].Name)
	}
}
