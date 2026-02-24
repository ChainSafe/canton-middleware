// Package migrations holds migrations related helpers
package migrations

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

const usageText = `Usage:
  go run cmd/*/migrate/*.go <command> [args]

This program runs command on the database. Supported commands are:
  - init - creates migration info table in the database
  - up - runs all available migrations.
  - down - reverts last migration.
  - status - prints migration status.

Examples:
  go run cmd/relayer/migrate/main.go -config config.yaml init
  go run cmd/relayer/migrate/main.go -config config.yaml up
  go run cmd/api-server/migrate/main.go -config config.api-server.yaml up
`

// Usage prints command usage
func Usage() {
	fmt.Print(usageText)
	flag.PrintDefaults()
	os.Exit(2)
}

func errorf(s string, args ...any) {
	fmt.Fprintf(os.Stderr, s+"\n", args...)
}

// Exitf exits command printing usage
func Exitf(s string, args ...any) {
	errorf(s, args...)
	Usage()
	os.Exit(1)
}

// CreateSchema creates schema from models
func CreateSchema(ctx context.Context, db bun.IDB, models ...any) error {
	for _, model := range models {
		log.Println("Creating Table for", reflect.TypeOf(model))
		_, err := db.NewCreateTable().
			Model(model).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// DropTables drops tables from database
func DropTables(ctx context.Context, db bun.IDB, models ...any) error {
	for _, model := range models {
		log.Println("Dropping Table for", reflect.TypeOf(model))
		_, err := db.NewDropTable().
			Model(model).
			IfExists().
			Cascade().
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// InsertEntry inserts entries to the db
func InsertEntry(ctx context.Context, db bun.IDB, entries ...any) error {
	for _, entry := range entries {
		log.Println("Inserting entry")
		_, err := db.NewInsert().
			Model(entry).
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// TruncateTables removes entries from tables
func TruncateTables(ctx context.Context, db bun.IDB, models ...any) error {
	for _, model := range models {
		_, err := db.NewDelete().
			Model(model).
			Where("1=1").
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateIndex creates an index on the database
func CreateIndex(ctx context.Context, db bun.IDB, tableName, indexName, columns string) error {
	_, err := db.NewCreateIndex().
		Table(tableName).
		Index(indexName).
		Column(columns).
		IfNotExists().
		Exec(ctx)
	return err
}

// CreateIndexes creates multiple indexes on the table for the given columns.
// Index names are generated as idx_<table>_<column>.
func CreateIndexes(ctx context.Context, db bun.IDB, tableName string, columns ...string) error {
	for _, column := range columns {
		indexName := fmt.Sprintf("idx_%s_%s", strings.Trim(tableName, `"`), column)
		if err := CreateIndex(ctx, db, tableName, indexName, column); err != nil {
			return err
		}
	}
	return nil
}

// CreateModelIndexes creates multiple indexes on the table associated with the model.
func CreateModelIndexes(ctx context.Context, db bun.IDB, model any, columns ...string) error {
	for _, column := range columns {
		indexName, err := modelIndexName(db, model, column)
		if err != nil {
			return err
		}
		if _, err = db.NewCreateIndex().
			Model(model).
			Index(indexName).
			Column(column).
			IfNotExists().
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CreateModelUniqueIndexes creates multiple unique indexes on the table associated with the model.
func CreateModelUniqueIndexes(ctx context.Context, db bun.IDB, model any, columns ...string) error {
	for _, column := range columns {
		indexName, err := modelIndexName(db, model, column)
		if err != nil {
			return err
		}
		if _, err = db.NewCreateIndex().
			Model(model).
			Index(indexName).
			Column(column).
			Unique().
			IfNotExists().
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CreateUniqueIndexes creates multiple unique indexes on the table for the given columns.
// Index names are generated as idx_<table>_<column>.
func CreateUniqueIndexes(ctx context.Context, db bun.IDB, tableName string, columns ...string) error {
	for _, column := range columns {
		indexName := fmt.Sprintf("idx_%s_%s", strings.Trim(tableName, `"`), column)
		if _, err := db.NewCreateIndex().
			Table(tableName).
			Index(indexName).
			Column(column).
			Unique().
			IfNotExists().
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// DropIndex drops an index from the database.
func DropIndex(ctx context.Context, db bun.IDB, indexName string) error {
	_, err := db.NewDropIndex().
		Index(indexName).
		IfExists().
		Exec(ctx)
	return err
}

// DropModelIndexes drops indexes from the database using model + column names.
func DropModelIndexes(ctx context.Context, db bun.IDB, model any, columns ...string) error {
	for _, column := range columns {
		indexName, err := modelIndexName(db, model, column)
		if err != nil {
			return err
		}
		if _, err = db.NewDropIndex().
			Model(model).
			Index(indexName).
			IfExists().
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func modelIndexName(db bun.IDB, model any, column string) (string, error) {
	if model == nil {
		return "", fmt.Errorf("model cannot be nil")
	}
	tableName := db.NewCreateIndex().Model(model).GetTableName()
	if tableName == "" {
		return "", fmt.Errorf("failed to resolve table name for model %T", model)
	}

	indexTableName := strings.NewReplacer(`"`, "", ".", "_").Replace(tableName)
	return fmt.Sprintf("idx_%s_%s", indexTableName, column), nil
}

// RunMigrations runs migrations based on provided command arguments
func RunMigrations(migrator *migrate.Migrator, args ...string) error {
	ctx := context.Background()

	if len(args) == 0 {
		Exitf("no command provided")
	}

	switch args[0] {
	case "init":
		if err := migrator.Init(ctx); err != nil {
			return err
		}
		log.Println("migration table created")
		return nil

	case "up":
		if err := migrator.Lock(ctx); err != nil {
			return fmt.Errorf("failed to acquire migration lock: %w", err)
		}
		defer func() {
			if err := migrator.Unlock(ctx); err != nil {
				log.Printf("failed to release migration lock: %v", err)
			}
		}()

		group, err := migrator.Migrate(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Println("no new migrations to run (database is up to date)")
		} else {
			log.Printf("migrated to %s\n", group)
		}
		return nil

	case "down":
		if err := migrator.Lock(ctx); err != nil {
			return fmt.Errorf("failed to acquire migration lock: %w", err)
		}
		defer func() {
			if err := migrator.Unlock(ctx); err != nil {
				log.Printf("failed to release migration lock: %v", err)
			}
		}()

		group, err := migrator.Rollback(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Println("no migrations to rollback")
		} else {
			log.Printf("rolled back %s\n", group)
		}
		return nil

	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			return err
		}
		log.Printf("migrations: %s\n", ms)
		log.Printf("unapplied migrations: %s\n", ms.Unapplied())
		log.Printf("last migration group: %s\n", ms.LastGroup())
		return nil

	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}
