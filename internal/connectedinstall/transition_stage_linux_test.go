//go:build linux

package connectedinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

func syntheticStageHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func syntheticStageFixture(t *testing.T) (string, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config")
	stagePath := filepath.Join(configPath, transitionStageName)
	if err := os.Mkdir(configPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stagePath, 0700); err != nil {
		t.Fatal(err)
	}
	c := connectedprofile.Config{
		Version: connectedprofile.Version, State: "INSTALLED_READY",
		ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32),
		CollectorUID: 2001, UploaderUID: 2002, UploaderGID: 2003, SharedGID: 2004,
		ArtifactSHA256: strings.Repeat("a", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"},
	}
	next := c
	next.PolicyGeneration = 2
	next.Addresses = []string{"8.8.8.8"}
	oldCA := []byte("synthetic old CA\n")
	newCA := []byte("synthetic new CA\n")
	previousConfig, err := connectedprofile.Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	nextConfig, err := connectedprofile.Encode(next)
	if err != nil {
		t.Fatal(err)
	}
	hosts := func(address string) string {
		return syntheticStageHash([]byte(address + " " + connectedprofile.Hostname + "\n"))
	}
	record := connectedtransition.Record{
		Version: connectedtransition.Version, Operation: "refresh", Previous: c, Next: next,
		ContractSHA256: syntheticStageHash([]byte("synthetic contract")),
		PreviousResources: connectedtransition.Resources{
			CA: syntheticStageHash(oldCA), Hosts: hosts(c.Addresses[0]),
			CollectorUnit: syntheticStageHash([]byte("old collector")),
			UploaderUnit: syntheticStageHash([]byte("old uploader")),
			InstalledConfig: syntheticStageHash(previousConfig),
		},
		NextResources: connectedtransition.Resources{
			CA: syntheticStageHash(newCA), Hosts: hosts(next.Addresses[0]),
			CollectorUnit: syntheticStageHash([]byte("new collector")),
			UploaderUnit: syntheticStageHash([]byte("new uploader")),
			InstalledConfig: syntheticStageHash(nextConfig),
		},
		Ledger: connectedtransition.Ledger{
			LogicalSHA256: syntheticStageHash([]byte("synthetic private ledger")),
			Physical: connectedtransition.FromWitness(ledgeridentity.Witness{
				FilesystemUUID: strings.Repeat("1", 32),
				Directory: ledgeridentity.Object{Inode: 7, Generation: 9},
				Database: ledgeridentity.Object{Inode: 8, Generation: 10},
			}),
		},
	}
	proposal, err := connectedtransition.Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := connectedtransition.Prepare(record)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := connectedtransition.EncodePreparation(witness)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		transitionPreparationName: preparation,
		connectedtransition.StageProposalName: proposal,
		connectedtransition.StageOldCAName: oldCA,
		connectedtransition.StageNewCAName: newCA,
	} {
		parent := stagePath
		if name == transitionPreparationName {
			parent = configPath
		}
		if err := os.WriteFile(filepath.Join(parent, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return configPath, stagePath
}

func openSyntheticStage(t *testing.T, configPath, stagePath string) (*os.File, *os.File) {
	t.Helper()
	config, err := os.Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := os.Open(stagePath)
	if err != nil {
		config.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { stage.Close(); config.Close() })
	return config, stage
}

func TestOwnedTransitionStageReadOnlyFixture(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		if os.Getenv("OBSERVER_CONNECTED_STAGE_ACCEPTANCE") == "1" {
			t.Fatal("root acceptance fixture was not privileged")
		}
		t.Skip("root-owned synthetic fixture")
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(t.TempDir(), &fs); err != nil || fs.Type != unix.EXT4_SUPER_MAGIC {
		if os.Getenv("OBSERVER_CONNECTED_STAGE_ACCEPTANCE") == "1" {
			t.Fatal("ext4 acceptance fixture unavailable", err)
		}
		t.Skip("ext4 fixture unavailable")
	}
	for name, mutate := range map[string]func(t *testing.T, config, stage string){
		"valid": func(*testing.T, string, string) {},
		"config mode": func(t *testing.T, config, _ string) {
			if err := os.Chmod(config, 0750); err != nil {
				t.Fatal(err)
			}
		},
		"stage mode": func(t *testing.T, _, stage string) {
			if err := os.Chmod(stage, 0755); err != nil {
				t.Fatal(err)
			}
		},
		"stage owner": func(t *testing.T, _, stage string) {
			if err := os.Chown(stage, 65534, 0); err != nil {
				t.Fatal(err)
			}
		},
		"unknown entry": func(t *testing.T, _, stage string) {
			if err := os.WriteFile(filepath.Join(stage, "foreign"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"missing member": func(t *testing.T, _, stage string) {
			if err := os.Remove(filepath.Join(stage, connectedtransition.StageOldCAName)); err != nil {
				t.Fatal(err)
			}
		},
		"substituted bytes": func(t *testing.T, _, stage string) {
			if err := os.WriteFile(filepath.Join(stage, connectedtransition.StageOldCAName), []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"wrong file mode": func(t *testing.T, _, stage string) {
			if err := os.Chmod(filepath.Join(stage, connectedtransition.StageOldCAName), 0644); err != nil {
				t.Fatal(err)
			}
		},
		"wrong file owner": func(t *testing.T, _, stage string) {
			if err := os.Chown(filepath.Join(stage, connectedtransition.StageOldCAName), 65534, 0); err != nil {
				t.Fatal(err)
			}
		},
		"wrong file group": func(t *testing.T, _, stage string) {
			if err := os.Chown(filepath.Join(stage, connectedtransition.StageOldCAName), 0, 65534); err != nil {
				t.Fatal(err)
			}
		},
		"symlink member": func(t *testing.T, _, stage string) {
			old := filepath.Join(stage, connectedtransition.StageOldCAName)
			if err := os.Remove(old); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(connectedtransition.StageNewCAName, old); err != nil {
				t.Fatal(err)
			}
		},
		"hardlink member": func(t *testing.T, config, stage string) {
			old := filepath.Join(stage, connectedtransition.StageOldCAName)
			if err := os.Link(old, filepath.Join(config, "linked-ca")); err != nil {
				t.Fatal(err)
			}
		},
		"missing preparation": func(t *testing.T, config, _ string) {
			if err := os.Remove(filepath.Join(config, transitionPreparationName)); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			configPath, stagePath := syntheticStageFixture(t)
			mutate(t, configPath, stagePath)
			config, stage := openSyntheticStage(t, configPath, stagePath)
			result, err := inspectTransitionStageAt(config, stage)
			if name == "valid" {
				if err != nil || result.record.Operation != "refresh" {
					t.Fatal("valid owned stage refused", err)
				}
			} else if err == nil {
				t.Fatal("unsafe stage admitted")
			}
		})
	}
	t.Run("symlink stage", func(t *testing.T) {
		configPath, stagePath := syntheticStageFixture(t)
		if err := os.Rename(stagePath, stagePath+"-real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(transitionStageName+"-real", stagePath); err != nil {
			t.Fatal(err)
		}
		config, err := os.Open(configPath)
		if err != nil {
			t.Fatal(err)
		}
		defer config.Close()
		if fd, err := openFixedStageMember(int(config.Fd()), transitionStageName, true); err == nil {
			unix.Close(fd)
			t.Fatal("linked stage directory admitted")
		}
	})
}
