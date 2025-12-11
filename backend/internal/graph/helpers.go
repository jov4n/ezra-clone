package graph

import (
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ============================================================================
// Helper Functions
// ============================================================================

func getStringFromRecord(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

func getIntFromRecord(record *neo4j.Record, key string) int {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	if i, ok := val.(int64); ok {
		return int(i)
	}
	if i, ok := val.(int); ok {
		return i
	}
	return 0
}

func getInt64FromRecord(record *neo4j.Record, key string) int64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	if i, ok := val.(int64); ok {
		return i
	}
	if i, ok := val.(int); ok {
		return int64(i)
	}
	return 0
}

func getFloat64FromRecord(record *neo4j.Record, key string) float64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0.0
	}
	if f, ok := val.(float64); ok {
		return f
	}
	if i, ok := val.(int64); ok {
		return float64(i)
	}
	return 0.0
}

func getStringSliceFromRecord(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return []string{}
	}
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, v := range slice {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

func getStringFromMap(m map[string]interface{}, key, defaultValue string) string {
	val, ok := m[key]
	if !ok || val == nil {
		return defaultValue
	}
	if str, ok := val.(string); ok {
		return str
	}
	return defaultValue
}

func getFloat64FromMap(m map[string]interface{}, key string, defaultValue float64) float64 {
	val, ok := m[key]
	if !ok || val == nil {
		return defaultValue
	}
	if f, ok := val.(float64); ok {
		return f
	}
	if i, ok := val.(int64); ok {
		return float64(i)
	}
	return defaultValue
}

