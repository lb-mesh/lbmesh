package recipe

import (
	"strings"
	"testing"
)

func validRecipe() Recipe {
	return Recipe{
		RecipeVersion: RecipeVersion,
		Metadata:      Metadata{Name: "mysql", Category: "database", Scope: "system"},
		Container:     Container{Image: "mysql:8.4"},
		Addons: []Addon{{
			Name:      "admin",
			Container: Container{Image: "phpmyadmin:5", Ports: []PortBinding{{Container: 80, Host: 8120}}},
		}},
	}
}

func TestValidate_Addons(t *testing.T) {
	r := validRecipe()
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.Addon("admin") == nil || r.Addon("missing") != nil {
		t.Fatal("Addon lookup returned the wrong result")
	}

	cases := map[string]func(*Recipe){
		"name":                              func(r *Recipe) { r.Addons[0].Name = "../admin" },
		"more than once":                    func(r *Recipe) { r.Addons = append(r.Addons, r.Addons[0]) },
		"image":                             func(r *Recipe) { r.Addons[0].Container.Image = "" },
		"negative":                          func(r *Recipe) { r.Addons[0].Resources.MinMemoryMB = -1 },
		"addons[0].container.ports[0].host": func(r *Recipe) { r.Addons[0].Container.Ports[0].Host = 0 },
	}
	for want, mutate := range cases {
		r := validRecipe()
		mutate(&r)
		err := r.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() = %v, want error containing %q", err, want)
		}
	}
}

func TestValidate_Files(t *testing.T) {
	ok := validRecipe()
	ok.Container.Files = []File{{Path: "/etc/app/app.conf", Content: "x", Mode: "0600"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	bad := map[string][]File{
		"relative":  {{Path: "etc/app.conf"}},
		"unclean":   {{Path: "/etc/../app.conf"}},
		"root":      {{Path: "/"}},
		"duplicate": {{Path: "/a"}, {Path: "/a"}},
		"mode":      {{Path: "/a", Mode: "rw-r--r--"}},
		"too wide":  {{Path: "/a", Mode: "07777"}},
		"too big":   {{Path: "/a", Content: strings.Repeat("x", maxFileBytes+1)}},
	}
	for name, files := range bad {
		r := validRecipe()
		r.Addons[0].Container.Files = files
		if err := r.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}
