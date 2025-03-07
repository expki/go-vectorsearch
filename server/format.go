package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	_ "github.com/expki/go-vectorsearch/env"
)

func Flatten(data any) string {
	switch value := data.(type) {
	case nil:
		return "null"
	case string:
		return formatString(value)
	case float64:
		return flattenFloat(value)
	case bool:
		return flattenBool(value)
	case []any:
		return flattenArray(value)
	case map[string]any:
		return flattenMap(value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func formatString(value string) string {
	value = strings.ReplaceAll(value, `"`, `'`)
	if strings.ContainsRune(value, ',') {
		value = strconv.Quote(value)
	}
	return value
}

func flattenFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 32)
}

func flattenBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func flattenArray(data []any) string {
	var builder strings.Builder
	for idx, item := range data {
		builder.WriteString(Flatten(item))
		if idx != len(data)-1 {
			builder.WriteString(", ")
		}
	}
	return builder.String()
}

func flattenMap(data map[string]any) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for idx, key := range keys {
		builder.WriteString(key)
		builder.WriteString(": ")
		value := data[key]
		switch value.(type) {
		case map[string]any, []any:
			builder.WriteRune('"')
			builder.WriteString(Flatten(value))
			builder.WriteRune('"')
		default:
			builder.WriteString(Flatten(value))
		}
		if idx != len(keys)-1 {
			builder.WriteString(", ")
		}
	}
	return builder.String()
}
