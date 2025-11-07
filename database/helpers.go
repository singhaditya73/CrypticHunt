package database

import (
	"os"
	"strconv"
	"strings"
)

// ConvertPlaceholders converts ? placeholders to $1, $2, etc. for PostgreSQL
// Also converts SQLite-specific syntax to PostgreSQL
// Returns the original query for SQLite
func ConvertPlaceholders(query string) string {
	// If using SQLite (no DATABASE_URL), return as-is
	if os.Getenv("DATABASE_URL") == "" {
		return query
	}
	
	// For PostgreSQL, convert ? to $1, $2, $3...
	count := 1
	result := strings.Builder{}
	
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			result.WriteString("$" + strconv.Itoa(count))
			count++
		} else {
			result.WriteByte(query[i])
		}
	}
	
	converted := result.String()
	
	// Convert SQLite-specific syntax to PostgreSQL
	// Handle INSERT OR IGNORE for team_completed_questions
	if strings.Contains(converted, "INSERT OR IGNORE INTO team_completed_questions") {
		converted = strings.ReplaceAll(converted, "INSERT OR IGNORE INTO", "INSERT INTO")
		if !strings.Contains(converted, "ON CONFLICT") {
			converted = strings.TrimSuffix(converted, ";")
			converted += " ON CONFLICT (team_id, question_id) DO NOTHING"
		}
	} else if strings.Contains(converted, "INSERT OR IGNORE INTO team_hint_unlocked") {
		converted = strings.ReplaceAll(converted, "INSERT OR IGNORE INTO", "INSERT INTO")
		if !strings.Contains(converted, "ON CONFLICT") {
			converted = strings.TrimSuffix(converted, ";")
			converted += " ON CONFLICT (team_id, hint_id) DO NOTHING"
		}
	} else {
		// General case: just replace INSERT OR IGNORE
		converted = strings.ReplaceAll(converted, "INSERT OR IGNORE", "INSERT")
		converted = strings.ReplaceAll(converted, "INSERT OR REPLACE", "INSERT")
	}
	
	return converted
}
