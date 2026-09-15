package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
)

func TestCodexRsCaptureVersionMatchesIdentity(t *testing.T) {
	if portrait.RequiredHostCodexCLIVersion != codexCLIVersion {
		t.Fatalf("plugin pin %q != host codexCLIVersion %q", portrait.RequiredHostCodexCLIVersion, codexCLIVersion)
	}
	manifest, err := portrait.EmbeddedManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CodexCLIVersion != codexCLIVersion {
		t.Fatalf("capture manifest %q != host %q", manifest.CodexCLIVersion, codexCLIVersion)
	}
}
