// Package migrations holds migrations related helpers
package migrations

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

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

func errorf(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, s+"\n", args...)
}

// Exitf exits command printing usage
func Exitf(s string, args ...interface{}) {
	errorf(s, args...)
	Usage()
	os.Exit(1)
}

// CreateSchema creates schema from models
func CreateSchema(db *pg.Tx, models ...interface{}) error {
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
func DropTables(db *pg.Tx, models ...interface{}) error {
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
func InsertEntry(db *pg.Tx, entries ...interface{}) error {
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
func TruncateTables(db *pg.Tx, models ...interface{}) error {
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
	query := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)", indexName, tableName, columns)
	_, err := db.Exec(query)
	return err
}

// DropIndex drops an index from the database
func DropIndex(db *pg.Tx, indexName string) error {
	query := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
	_, err := db.Exec(query)
	return err
}
