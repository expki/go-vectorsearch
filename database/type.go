package database

import (
	"database/sql/driver"
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
		return fmt.Errorf("failed to unmarshal DocumentField value: %+v", value)
	}
	original, err := decompress(bytes)
	if err != nil {
		return fmt.Errorf("failed to decompress DocumentField value: %+v", subSlice(bytes, 10))
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

type VectorField []uint8

// Scan scan value into Vector, implements sql.Scanner interface
func (v *VectorField) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal Vector value: %+v", value)
	}
	original, err := decompress(bytes)
	if err != nil {
		return fmt.Errorf("failed to decompress Vector value: %+v", subSlice(bytes, 10))
	}
	*v = VectorField(original)
	return nil
}

// Value return Vector value, implement driver.Valuer interface
func (v VectorField) Value() (driver.Value, error) {
	return compress([]byte(v)), nil
}

func (e VectorField) Underlying() []uint8 {
	out := make([]uint8, len(e))
	for i, value := range e {
		out[i] = uint8(value)
	}
	return out
}
