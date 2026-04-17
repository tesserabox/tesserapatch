package assets

import (
	"strings"
	"testing"
)

// Required CLI commands that must appear in all skill formats.
var requiredCommands = []string{
	"tpatch init",
	"tpatch add",
	"tpatch status",
	"tpatch analyze",
	"tpatch define",
	"tpatch explore",
	"tpatch implement",
	"tpatch apply",
	"tpatch record",
	"tpatch reconcile",
	"tpatch provider",
	"tpatch config",
	"tpatch cycle",
	"tpatch test",
	"tpatch next",
}

// Skill format files that must mention all CLI commands.
var skillFiles = []struct {
	name string
	path string
}{
	{"Claude", "skills/claude/tessera-patch/SKILL.md"},
	{"Copilot", "skills/copilot/tessera-patch/SKILL.md"},
	{"Copilot Prompt", "prompts/copilot/tessera-patch-apply.prompt.md"},
	{"Cursor", "skills/cursor/tessera-patch.mdc"},
	{"Windsurf", "skills/windsurf/windsurfrules"},
	{"Generic", "workflows/tessera-patch-generic.md"},
}

func TestSkillParityGuard(t *testing.T) {
	for _, sf := range skillFiles {
		t.Run(sf.name, func(t *testing.T) {
			data, err := Skills.ReadFile(sf.path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", sf.path, err)
			}
			content := string(data)

			for _, cmd := range requiredCommands {
				if !strings.Contains(content, cmd) {
					t.Errorf("%s (%s) missing CLI command: %q", sf.name, sf.path, cmd)
				}
			}
		})
	}
}

func TestAllSkillFilesExist(t *testing.T) {
	for _, sf := range skillFiles {
		t.Run(sf.name, func(t *testing.T) {
			_, err := Skills.ReadFile(sf.path)
			if err != nil {
				t.Fatalf("skill file %s (%s) not found: %v", sf.name, sf.path, err)
			}
		})
	}
}
