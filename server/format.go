package server

import (
	"fmt"
	"sort"
	"strings"

	_ "github.com/expki/go-vectorsearch/env"
)

func FlattenMap(data map[string]any) string {
	var result strings.Builder
	flatten("", data, &result)
	return result.String()
}

func flatten(prefix string, data any, result *strings.Builder) {
	switch v := data.(type) {
	case map[string]any:
		// Sort the keys alphabetically
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			newPrefix := key
			if prefix != "" {
				newPrefix = fmt.Sprintf("%s.%s", prefix, key)
			}
			value := v[key]
			flatten(newPrefix, value, result)
		}
	case []any:
		for _, item := range v {
			flatten(prefix, item, result)
		}
	default:
		if prefix != "" {
			if result.Len() > 0 {
				result.WriteRune(' ')
			}
			result.WriteString(fmt.Sprintf("%s=%v", prefix, format(v)))
		}
	}
}

func format(data any) any {
	switch value := data.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(value, `\`, `'`))
	default:
		return data
	}
}
