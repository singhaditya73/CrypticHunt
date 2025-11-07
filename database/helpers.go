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
	converted = strings.ReplaceAll(converted, "INSERT OR IGNORE", "INSERT")
	converted = strings.ReplaceAll(converted, "INSERT OR REPLACE", "INSERT")
	
	// Add ON CONFLICT DO NOTHING for INSERT ... ON CONFLICT
	if strings.Contains(converted, "INSERT INTO team_completed_questions") {
		converted = strings.Replace(converted, "VALUES", "VALUES", 1)
		if !strings.Contains(converted, "ON CONFLICT") {
			converted += " ON CONFLICT (team_id, question_id) DO NOTHING"
		}
	}
	
	return converted
}
