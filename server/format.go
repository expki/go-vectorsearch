package server

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "github.com/expki/go-vectorsearch/env"
)

var (
	excessNewlineRegex = regexp.MustCompile("\n\n+")
)

// Flatten data with each sentence on a new line
func Flatten(data any) string {
	switch value := data.(type) {
	case nil:
		return "null."
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
	value = strings.ReplaceAll(value, "\r", "")
	value = excessNewlineRegex.ReplaceAllString(value, "\n")
	value = strings.TrimSpace(value)
	value, _ = strings.CutSuffix(value, "\n")
	if !strings.HasSuffix(value, ".") {
		value += "."
	}
	return value
}

func flattenFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 32)
}

func flattenBool(value bool) string {
	if value {
		return "true."
	}
	return "false."
}

func flattenArray(data []any) string {
	var builder strings.Builder
	for idx, item := range data {
		builder.WriteString(Flatten(item))
		if idx != len(data)-1 {
			builder.WriteString("\n")
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
		list := strings.Split(Flatten(data[key]), "\n")
		for jdx, value := range list {
			builder.WriteString(key)
			builder.WriteString(": ")
			builder.WriteString(value)
			if jdx != len(list)-1 && idx != len(keys)-1 {
				builder.WriteString("\n")
			}
		}
	}
	return builder.String()
}

func Split(text string, ctxNum int) (list []string) {
	maxWords := ((ctxNum * 9) / 10) / 4
	list = make([]string, 0, 10)
	current := ""
	currentNumWords := 0
	for _, sentence := range strings.Split(text, "\n") {
		numWords := len(strings.Fields(sentence))
		if numWords+currentNumWords > maxWords && current != "" {
			list = append(list, current)
			current = ""
			currentNumWords = 0
		}
		current = fmt.Sprintf("%s %s", current, sentence)
		currentNumWords += numWords
	}
	list = append(list, current)
	return list
}
