package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
)

func TestCodexRsCaptureVersionStaysOnItsOwnPin(t *testing.T) {
	manifest, err := portrait.EmbeddedManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CodexCLIVersion != portrait.RequiredHostCodexCLIVersion {
		t.Fatalf("capture manifest %q != plugin pin %q", manifest.CodexCLIVersion, portrait.RequiredHostCodexCLIVersion)
	}
	if NormalizeCodexClientVersion(codexCLIVersion) == "" {
		t.Fatalf("host outbound version %q is not a Codex version", codexCLIVersion)
	}
}
