package database

import (
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"

	_ "github.com/expki/go-vectorsearch/env"
)

type DocumentField json.RawMessage

func (d DocumentField) JSON() (value any) {
	json.Unmarshal(d, &value)
	return value
}

// Scan scan value into DocumentField, implements sql.Scanner interface
func (d *DocumentField) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal DocumentField value: %v", value)
	}
	original, err := decompress(bytes)
	if err != nil {
		return fmt.Errorf("failed to decompress DocumentField value: %s", hex.EncodeToString(subSlice(bytes, 20)))
	}
	result := json.RawMessage{}
	err = json.Unmarshal(original, &result)
	*d = DocumentField(result)
	return err
}

// Value return json value, implement driver.Valuer interface
func (d DocumentField) Value() (driver.Value, error) {
	if len(d) == 0 {
		return nil, nil
	}
	raw, err := json.RawMessage(d).MarshalJSON()
	if err != nil {
		return nil, err
	}
	return compress(raw), nil
}

func subSlice[T any](list []T, max int) []T {
	if len(list) > max {
		return list[:max]
	}
	return list
}
