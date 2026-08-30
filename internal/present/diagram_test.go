package present

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query"
)

func TestRenderErDiagram(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		Columns: []DiagramColumn{
			{Name: "AlbumId", Primary: true},
			{Name: "Title"},
			{Name: "ArtistId", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{{
			Name: "fk", Columns: []string{"ArtistId"},
			TargetSchema: "main", TargetTable: "Artist",
			TargetColumns: []string{"ArtistId"},
		}},
	}
	related := []DiagramTable{{
		Schema: "main", Name: "Artist",
		Columns: []DiagramColumn{{Name: "ArtistId", Primary: true}, {Name: "Name"}},
	}}

	lines := RenderErDiagram(root, related)
	drawn := strings.Join(lines, "\n")
	for _, wanted := range []string{"main.Album", "main.Artist", "PK", "FK", "▶"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the diagram has no %q:\n%s", wanted, drawn)
		}
	}
	width := MeasureText(lines[0])
	for at, line := range lines {
		if MeasureText(line) != width {
			t.Errorf("row %d is %d wide, wanted %d", at, MeasureText(line), width)
		}
	}
}

func TestCollectDiagramNeighbours(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		ForeignKeys: []query.ForeignKey{
			{TargetSchema: "main", TargetTable: "Artist"},
		},
	}
	found := CollectDiagramNeighbours(root, nil)
	if !found["main.Artist"] {
		t.Errorf("the target of a key is not a neighbour: %v", found)
	}
}
