package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

func TestOverlayIsOpenOnlyWhileItOwnsTheKeyboard(t *testing.T) {
	if (app.Overlay{}).IsOpen() || (app.Overlay{Kind: app.OverlayNone}).IsOpen() {
		t.Error("an overlay that is not shown reads as open")
	}
	if !(app.Overlay{Kind: app.OverlayConfirm}).IsOpen() {
		t.Error("a confirm card does not read as open")
	}
}

func TestBuildObjectActionsOffersNothingWithoutDDL(t *testing.T) {
	node := present.TreeNode{Kind: present.NodeTable, Table: db.TableRef{
		Name: "orders", Kind: db.RelationTable,
	}}
	if actions := app.BuildObjectActions(node, core.Capabilities{}); len(actions) != 0 {
		t.Errorf("a server that writes no DDL was offered %v", actions)
	}
}

func TestBuildObjectActionsDropsTruncateWhereTheServerCannot(t *testing.T) {
	node := present.TreeNode{Kind: present.NodeTable, Table: db.TableRef{
		Name: "orders", Kind: db.RelationTable,
	}}
	actions := app.BuildObjectActions(node, core.Capabilities{WritesDDL: true})
	for _, action := range actions {
		if action.ID == "truncate" {
			t.Fatal("truncate was offered on a server that cannot empty a table")
		}
	}
	actions = app.BuildObjectActions(node, core.Capabilities{
		WritesDDL: true, TruncatesTable: true,
	})
	found := false
	for _, action := range actions {
		if action.ID == "truncate" {
			found = true
		}
	}
	if !found {
		t.Error("truncate was not offered on a server that can empty a table")
	}
}

func TestBuildObjectTitleNamesTheNode(t *testing.T) {
	for _, held := range []struct {
		node present.TreeNode
		want string
	}{
		{present.TreeNode{Kind: present.NodeSchema, Schema: "public"}, "schema public"},
		{present.TreeNode{Kind: present.NodeTable, Table: db.TableRef{
			Name: "orders", Kind: db.RelationTable,
		}}, "table orders"},
		{present.TreeNode{Kind: present.NodeObject, Object: db.SchemaObject{
			Name: "total_of", Kind: db.ObjectFunction,
		}}, "function total_of"},
		{present.TreeNode{}, ""},
	} {
		if title := app.BuildObjectTitle(held.node); title != held.want {
			t.Errorf("title reads %q, wanted %q", title, held.want)
		}
	}
}
