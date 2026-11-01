package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecipes(t *testing.T) {
	dir := t.TempDir()
	for _, f := range SourceFiles {
		os.WriteFile(filepath.Join(dir, f+".json"), []byte("[]"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "Secondary.json"), []byte(`[
		{"uniqueName":"/bronco","name":"Bronco","masterable":true},
		{"uniqueName":"/akbronco","name":"Akbronco","masterable":true,"components":[
			{"uniqueName":"/Lotus/Types/Recipes/Weapons/AkbroncoBlueprint","itemCount":1},
			{"uniqueName":"/bronco","itemCount":1},
			{"uniqueName":"/bronco","itemCount":1},
			{"uniqueName":"/Lotus/Types/Items/MiscItems/Ferrite","itemCount":500}
		]}]`), 0o644)
	items, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ak, bronco := items[0], items[1]
	if parts := ak.Parts(); len(parts) != 2 || parts[1].Name != "Bronco" || parts[1].Count != 2 {
		t.Errorf("Akbronco parts = %+v, want blueprint and 2 Broncos, no Ferrite", parts)
	}
	if len(bronco.UsedIn) != 1 || bronco.UsedIn[0].Name != "Akbronco" || bronco.UsedIn[0].Count != 2 {
		t.Errorf("Bronco used in = %+v", bronco.UsedIn)
	}
}
