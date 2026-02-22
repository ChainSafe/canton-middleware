// Package migrations holds migrations related helpers
package migrations

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

const usageText = `Usage:
  go run cmd/db-migrate/*.go <command> [args]

This program runs command on the database. Supported commands are:
  - init - creates version info table in the database
  - up - runs all available migrations.
  - up [target] - runs available migrations up to the target one.
  - down - reverts last migration.
  - reset - reverts all migrations.
  - version - prints current db version.
  - set_version [version] - sets db version without running migrations.

Examples:
  go run cmd/db-migrate/*.go init
  go run cmd/db-migrate/*.go up
  go run cmd/db-migrate/*.go down
  go run cmd/db-migrate/*.go version
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
func CreateSchema(db *pg.Tx, models ...any) error {
	for _, model := range models {
		log.Println("Creating Table for", reflect.TypeOf(model))
		err := db.Model(model).CreateTable(&orm.CreateTableOptions{
			FKConstraints: true,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// DropTables drops tables from database
func DropTables(db *pg.Tx, models ...any) error {
	for _, model := range models {
		log.Println("Dropping Table for", reflect.TypeOf(model))
		err := db.Model(model).DropTable(&orm.DropTableOptions{
			IfExists: true,
			Cascade:  true,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// InsertEntry inserts entries to the db
func InsertEntry(db *pg.Tx, entries ...any) error {
	for _, entry := range entries {
		log.Println("Inserting entry")
		_, err := db.Model(entry).Insert()
		if err != nil {
			return err
		}
	}
	return nil
}

// TruncateTables removes entries from tables
func TruncateTables(db *pg.Tx, models ...any) error {
	for _, model := range models {
		_, err := db.Model(model).Exec(`DELETE FROM ?TableName`)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateIndex creates an index on the database
func CreateIndex(db *pg.Tx, tableName, indexName, columns string) error {
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS ? ON ? (?)", pg.Ident(indexName), pg.Safe(tableName), pg.Safe(columns))
	return err
}

// CreateIndexes creates multiple indexes on the table for the given columns.
// Index names are generated as idx_<table>_<column>.
func CreateIndexes(db *pg.Tx, tableName string, columns ...string) error {
	for _, column := range columns {
		indexName := fmt.Sprintf("idx_%s_%s", strings.Trim(tableName, `"`), column)
		if err := CreateIndex(db, tableName, indexName, column); err != nil {
			return err
		}
	}
	return nil
}

// CreateModelIndexes creates multiple indexes on the table associated with the model.
func CreateModelIndexes(db *pg.Tx, model any, columns ...string) error {
	tableName := getTableName(model)
	return CreateIndexes(db, tableName, columns...)
}

// CreateUniqueIndexes creates multiple unique indexes on the table for the given columns.
func CreateUniqueIndexes(db *pg.Tx, tableName string, columns ...string) error {
	for _, column := range columns {
		indexName := fmt.Sprintf("idx_%s_%s", strings.Trim(tableName, `"`), column)
		if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ? ON ? (?)",
			pg.Ident(indexName), pg.Safe(tableName), pg.Safe(column)); err != nil {
			return err
		}
	}
	return nil
}

// CreateModelUniqueIndexes creates multiple unique indexes on the table associated with the model.
func CreateModelUniqueIndexes(db *pg.Tx, model any, columns ...string) error {
	tableName := getTableName(model)
	return CreateUniqueIndexes(db, tableName, columns...)
}

func getTableName(model any) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return string(orm.GetTable(t).SQLName)
}

// DropIndex drops an index from the database
func DropIndex(db *pg.Tx, indexName string) error {
	_, err := db.Exec("DROP INDEX IF EXISTS ?", pg.Ident(indexName))
	return err
}
