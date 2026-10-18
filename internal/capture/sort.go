package capture

import (
	"encoding/json"
	"sort"
)

func sortShapes(values []any) {
	sort.Slice(values, func(i, j int) bool {
		a, _ := json.Marshal(values[i])
		b, _ := json.Marshal(values[j])
		return string(a) < string(b)
	})
}
