package tools

import "testing"

func TestDefinitionsSubset(t *testing.T) {
	defs, err := DefinitionsSubset([]string{"web_search", "read_file"})
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 {
		t.Fatalf("got %d", len(defs))
	}
}

func TestDefinitionsSubset_Unknown(t *testing.T) {
	_, err := DefinitionsSubset([]string{"not_a_real_tool"})
	if err == nil {
		t.Fatal("expected error")
	}
}
