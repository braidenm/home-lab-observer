//go:build linux

package actionbroker

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

// ProvisionJournal runs only in the prepared broker view. Normal startup
// never calls it. The protected policy supplies the sole accepted binding.
func ProvisionJournal(ctx context.Context) error {
	files, err := loadBrokerFiles(ctx)
	if err != nil {
		return ErrRecoveryRequired
	}
	p := files.policy
	journal, err := actionjournal.Provision(ctx, actionprofile.JournalPath, actionjournal.Binding{
		ServerID: p.ServerID, InstallationID: p.ConnectorInstanceID, OwnerID: p.OwnerUserID,
	})
	if err != nil {
		return ErrRecoveryRequired
	}
	checkErr := validateJournalMappings(ctx, journal, p, files.engine.generation)
	_, sourceErr := files.Current(ctx)
	closeErr := journal.Close()
	if checkErr != nil || sourceErr != nil || closeErr != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	return nil
}
