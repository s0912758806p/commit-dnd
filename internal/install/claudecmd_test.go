package install

import "testing"

func TestClaudeCommands(t *testing.T) {
	if got := HookCommand("/opt/it's/dnd"); got != `'/opt/it'\''s/dnd' hook claude` {
		t.Fatalf("HookCommand = %q", got)
	}
	for cmd, want := range map[string]bool{
		HookCommand("/a/b/dnd"):                true,
		StatuslineCommand("/a/b/dnd"):          true,
		"dnd hook claude":                      true,
		`"/x/dnd.exe" statusline`:              true,
		"bash ~/.claude/statusline-command.sh": false,
		"/a/b/other hook claude":               false,
		"/a/b/dnd hook git":                    false,
		"":                                     false,
	} {
		if IsDndCommand(cmd) != want {
			t.Errorf("IsDndCommand(%q) != %v", cmd, want)
		}
	}
}
