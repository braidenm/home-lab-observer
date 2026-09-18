//go:build linux

package connectedstagefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

func TestOwnedTransitionStageFirstJournalFixture(t *testing.T) {
	requireOwnedTransitionStageFixture(t)
	for _, scenario := range []string{
		"publish", "already-proposed", "partial-temp", "file-sync-interruption",
		"rename-interruption", "parent-sync-interruption", "already-proposed-sync-interruption",
		"unknown-temp", "linked-temp", "broad-temp", "foreign-journal",
		"premature-receipt", "canceled", "busy-directory", "overfull-directory",
		"legacy-publish", "legacy-rename-interruption",
	} {
		t.Run(scenario, func(t *testing.T) {
			path, stagePath := syntheticStageFixture(t)
			var legacyJournal, legacyReceipt []byte
			if strings.HasPrefix(scenario, "legacy-") {
				stageFile, openErr := os.Open(path)
				if openErr != nil {
					t.Fatal(openErr)
				}
				initial, inspectErr := InspectAt(stageFile)
				stageFile.Close()
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
				writeSyntheticLegacyPredecessor(t, path, stagePath, initial.Record)
				legacyJournal, inspectErr = os.ReadFile(filepath.Join(path, LegacyJournalName))
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
				legacyReceipt, inspectErr = os.ReadFile(filepath.Join(path, LegacyCompletionName))
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
			}
			config, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer config.Close()
			stage, err := InspectAt(config)
			if err != nil {
				t.Fatal(err)
			}
			proposal := stage.Files[connectedtransition.StageProposalName]
			sum := sha256.Sum256(proposal)
			temp := filepath.Join(path, journalTempPrefix+hex.EncodeToString(sum[:]))
			journal := filepath.Join(path, JournalName)
			switch scenario {
			case "already-proposed", "already-proposed-sync-interruption":
				if err := os.WriteFile(journal, proposal, 0600); err != nil {
					t.Fatal(err)
				}
			case "partial-temp":
				if err := os.WriteFile(temp, []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown-temp":
				if err := os.WriteFile(filepath.Join(path, journalTempPrefix+strings.Repeat("0", 64)), []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "linked-temp":
				if err := os.Symlink("missing", temp); err != nil {
					t.Fatal(err)
				}
			case "broad-temp":
				if err := os.WriteFile(temp, []byte("partial"), 0644); err != nil || os.Chmod(temp, 0644) != nil {
					t.Fatal("broad temporary fixture unavailable", err)
				}
			case "foreign-journal":
				if err := os.WriteFile(journal, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "premature-receipt":
				if err := os.WriteFile(filepath.Join(path, CompletionName), []byte("old receipt"), 0600); err != nil {
					t.Fatal(err)
				}
			case "busy-directory", "overfull-directory":
				count := 130
				if scenario == "overfull-directory" {
					count = maxJournalScanEntries
				}
				for i := 0; i < count; i++ {
					if err := os.WriteFile(filepath.Join(path, "unrelated-"+strconv.Itoa(i)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			var after func(string) error
			if strings.HasSuffix(scenario, "-interruption") {
				step := strings.TrimSuffix(strings.TrimPrefix(scenario, "legacy-"), "-interruption")
				if scenario == "already-proposed-sync-interruption" {
					step = "parent-sync"
				}
				after = func(at string) error {
					if at == step {
						return ErrRecovery
					}
					return nil
				}
			}
			err = publishFirstJournalAt(ctx, config, after)
			switch scenario {
			case "publish", "already-proposed", "partial-temp", "busy-directory", "legacy-publish":
				if err != nil {
					t.Fatal("exact first journal refused", err)
				}
			case "file-sync-interruption", "rename-interruption", "parent-sync-interruption", "already-proposed-sync-interruption", "legacy-rename-interruption":
				if err != ErrRecovery {
					t.Fatal("interrupted journal publication looked complete", err)
				}
				if err := PublishFirstJournalAt(context.Background(), config); err != nil {
					t.Fatal("exact first journal retry refused", err)
				}
			case "canceled":
				if err != ErrUnsafe {
					t.Fatal("canceled publication admitted", err)
				}
			default:
				if err != ErrRecovery {
					t.Fatal("foreign first-journal evidence accepted", err)
				}
			}
			if scenario == "publish" || scenario == "already-proposed" || scenario == "partial-temp" ||
				scenario == "busy-directory" || scenario == "legacy-publish" ||
				strings.HasSuffix(scenario, "-interruption") {
				authority, readErr := ReadAuthorityAt(config)
				if readErr != nil || !bytes.Equal(authority.Journal, proposal) || authority.Completion != nil {
					t.Fatal("exact first journal not authoritative after success", readErr)
				}
				if _, statErr := os.Lstat(temp); !os.IsNotExist(statErr) {
					t.Fatal("known journal temporary residue remained", statErr)
				}
				if strings.HasPrefix(scenario, "legacy-") {
					retainedJournal, journalErr := os.ReadFile(filepath.Join(path, LegacyJournalName))
					retainedReceipt, receiptErr := os.ReadFile(filepath.Join(path, LegacyCompletionName))
					if journalErr != nil || receiptErr != nil || !bytes.Equal(retainedJournal, legacyJournal) || !bytes.Equal(retainedReceipt, legacyReceipt) {
						t.Fatal("legacy authority changed during first journal publication", journalErr, receiptErr)
					}
				}
			}
		})
	}
}
