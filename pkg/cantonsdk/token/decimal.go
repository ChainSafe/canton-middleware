package token

import (
	"fmt"

	"github.com/shopspring/decimal"
)

func addDecimalStrings(a, b string) (string, error) {
	da, err := decimal.NewFromString(a)
	if err != nil {
		return "", fmt.Errorf("parse decimal: %w", err)
	}
	db, err := decimal.NewFromString(b)
	if err != nil {
		return "", fmt.Errorf("parse decimal: %w", err)
	}
	return da.Add(db).String(), nil
}

func compareDecimalStrings(a, b string) (int, error) {
	da, err := decimal.NewFromString(a)
	if err != nil {
		return 0, fmt.Errorf("parse decimal: %w", err)
	}
	db, err := decimal.NewFromString(b)
	if err != nil {
		return 0, fmt.Errorf("parse decimal: %w", err)
	}
	return da.Cmp(db), nil
}
