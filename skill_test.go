package tau

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The agent skill under skills/tau ships an API reference written by hand.
// This test fails when the package exports something the reference does not
// mention, or the reference names a function the package does not have, so
// the two cannot drift apart.
func TestSkillReferenceMatchesTheAPI(t *testing.T) {
	ref := string(Must(os.ReadFile("skills/tau/references/api.md")))
	skill := string(Must(os.ReadFile("skills/tau/SKILL.md")))

	fset := token.NewFileSet()
	pkgs := Must(parser.ParseDir(fset, ".", func(f os.FileInfo) bool { return !strings.HasSuffix(f.Name(), "_test.go") }, 0))
	exported := map[string]bool{}
	for _, file := range pkgs["tau"].Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() {
					exported[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							exported[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								exported[n.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if len(exported) < 40 {
		t.Fatalf("found only %d exported names; the parser is not seeing the package", len(exported))
	}
	for name := range exported {
		if !regexp.MustCompile(`\b` + name + `\b`).MatchString(ref) {
			t.Errorf("exported %s is missing from skills/tau/references/api.md", name)
		}
	}

	// Every tau.X the skill or reference mentions must exist.
	for _, doc := range []string{skill, ref} {
		for _, m := range regexp.MustCompile(`\btau\.([A-Z][A-Za-z]*)`).FindAllStringSubmatch(doc, -1) {
			if !exported[m[1]] {
				t.Errorf("the skill mentions tau.%s, which the package does not export", m[1])
			}
		}
	}
	// And every func the reference declares must exist, with nothing invented.
	for _, m := range regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?([A-Z][A-Za-z]*)`).FindAllStringSubmatch(ref, -1) {
		if !exported[m[1]] {
			t.Errorf("the reference declares func %s, which the package does not have", m[1])
		}
	}
	if !strings.HasPrefix(skill, "---\nname: tau\ndescription: ") {
		t.Error("SKILL.md must start with frontmatter holding name and description")
	}
}
