// SPDX-License-Identifier: Apache-2.0

package relayerdb

import (
	"context"
	"log"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	relayerstore "github.com/chainsafe/canton-middleware/pkg/relayer/store"

	"github.com/uptrace/bun"
)

// Multi-token bridging (#356): add the TokenBridge adapter dimension to
// transfers. Existing rows (and rows written by the legacy single-token
// pipeline until its port) are keyed 'wayfinder' via the column default.
// All statements are idempotent so fresh databases, whose migration 1
// already created the full model, pass through unchanged.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding multi-token columns to transfers...")
		ddls := []string{
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS bridge_key varchar(50) NOT NULL DEFAULT 'wayfinder'`,
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS token_symbol varchar(50)`,
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS stage varchar(100)`,
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS metadata jsonb`,
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS next_step_at timestamptz`,
		}
		for _, ddl := range ddls {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return err
			}
		}
		return mghelper.CreateModelIndexes(ctx, db, &relayerstore.TransferDao{}, "bridge_key", "next_step_at")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping multi-token columns from transfers...")
		if err := mghelper.DropModelIndexes(ctx, db, &relayerstore.TransferDao{}, "bridge_key", "next_step_at"); err != nil {
			return err
		}
		ddls := []string{
			`ALTER TABLE transfers DROP COLUMN IF EXISTS next_step_at`,
			`ALTER TABLE transfers DROP COLUMN IF EXISTS metadata`,
			`ALTER TABLE transfers DROP COLUMN IF EXISTS stage`,
			`ALTER TABLE transfers DROP COLUMN IF EXISTS token_symbol`,
			`ALTER TABLE transfers DROP COLUMN IF EXISTS bridge_key`,
		}
		for _, ddl := range ddls {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return err
			}
		}
		return nil
	})
}
