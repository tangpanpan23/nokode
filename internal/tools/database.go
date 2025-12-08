package tools

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nokode/nokode/internal/utils"
)

var db *sql.DB
var cachedSchema string

func init() {
	// Initialize database
	dbPath := "database.db"
	
	// Try to find it in project root
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(dbPath); err == nil {
			break
		}
		dbPath = filepath.Join("..", dbPath)
	}
	
	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=1")
	if err != nil {
		utils.Log.Error("database", "Failed to open database", err)
		return
	}

	// Enable foreign keys
	db.Exec("PRAGMA foreign_keys = ON")

	// Load schema on startup
	loadDatabaseSchema()
}

func loadDatabaseSchema() {
	query := "SELECT sql FROM sqlite_master WHERE type='table' AND sql IS NOT NULL"
	rows, err := db.Query(query)
	if err != nil {
		utils.Log.Error("database", "Failed to load database schema", err)
		cachedSchema = ""
		return
	}
	defer rows.Close()

	var schema strings.Builder
	schema.WriteString("\n## DATABASE SCHEMA (Use these exact column names!)\n\n")

	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			continue
		}
		schema.WriteString(sql)
		schema.WriteString(";\n\n")
	}

	cachedSchema = schema.String()
	utils.Log.Success("startup", "Database schema cached for performance", nil)
}

func GetCachedSchema() string {
	return cachedSchema
}

type DatabaseResult struct {
	Success        bool                   `json:"success"`
	Rows           []map[string]interface{} `json:"rows,omitempty"`
	Count          int                    `json:"count,omitempty"`
	Changes        int64                  `json:"changes,omitempty"`
	LastInsertRowID int64                 `json:"lastInsertRowid,omitempty"`
	Message        string                 `json:"message,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Duration       int64                  `json:"duration,omitempty"`
}

func ExecuteDatabaseQuery(query string, params []interface{}, mode string) DatabaseResult {
	startTime := time.Now()
	
	queryPreview := query
	if len(query) > 100 {
		queryPreview = query[:100] + "..."
	}
	
	utils.Log.Database(fmt.Sprintf("Executing %s query", strings.ToUpper(mode)), map[string]interface{}{
		"query":      queryPreview,
		"paramsCount": len(params),
		"hasParams":  len(params) > 0,
	})

	if len(params) > 0 {
		utils.Log.Debug("database", "Query parameters", params)
	}

	var result DatabaseResult
	result.Success = false

	// Trim and check query type
	queryUpper := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(queryUpper, "SELECT") ||
		strings.Contains(queryUpper, "RETURNING") ||
		strings.HasPrefix(queryUpper, "PRAGMA")

	if mode == "exec" && len(params) == 0 {
		// Exec mode for DDL or multiple statements without parameters
		utils.Log.Debug("database", "Using exec mode (DDL/multiple statements)", nil)
		_, err := db.Exec(query)
		duration := time.Since(startTime).Milliseconds()
		
		if err != nil {
			utils.Log.Error("database", fmt.Sprintf("Query failed after %dms", duration), err)
			result.Error = err.Error()
			result.Duration = duration
			return result
		}

		utils.Log.Success("database", fmt.Sprintf("Exec completed in %dms", duration), nil)
		result.Success = true
		result.Message = "Query executed successfully"
		result.Duration = duration
		return result
	}

	// Prepared statement mode
	utils.Log.Debug("database", "Using prepared statement mode", nil)

	if isSelect {
		// SELECT query
		rows, err := db.Query(query, params...)
		if err != nil {
			duration := time.Since(startTime).Milliseconds()
			utils.Log.Error("database", fmt.Sprintf("Query failed after %dms", duration), err)
			result.Error = err.Error()
			result.Duration = duration
			return result
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			duration := time.Since(startTime).Milliseconds()
			result.Error = err.Error()
			result.Duration = duration
			return result
		}

		var allRows []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}

			row := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					// Try to unmarshal JSON
					var jsonVal interface{}
					if err := json.Unmarshal(b, &jsonVal); err == nil {
						val = jsonVal
					} else {
						val = string(b)
					}
				}
				row[col] = val
			}
			allRows = append(allRows, row)
		}

		duration := time.Since(startTime).Milliseconds()
		utils.Log.Success("database", fmt.Sprintf("SELECT returned %d rows in %dms", len(allRows), duration), nil)
		
		result.Success = true
		result.Rows = allRows
		result.Count = len(allRows)
		result.Duration = duration
		return result
	} else {
		// INSERT, UPDATE, DELETE
		res, err := db.Exec(query, params...)
		if err != nil {
			duration := time.Since(startTime).Milliseconds()
			utils.Log.Error("database", fmt.Sprintf("Query failed after %dms", duration), err)
			result.Error = err.Error()
			result.Duration = duration
			return result
		}

		changes, _ := res.RowsAffected()
		lastID, _ := res.LastInsertId()
		
		queryType := strings.Fields(queryUpper)[0]
		duration := time.Since(startTime).Milliseconds()
		utils.Log.Success("database", fmt.Sprintf("%s affected %d rows in %dms", queryType, changes, duration), map[string]interface{}{
			"changes":        changes,
			"lastInsertRowid": lastID,
		})

		result.Success = true
		result.Changes = changes
		result.LastInsertRowID = lastID
		result.Duration = duration
		return result
	}
}

func GetDatabaseContext() string {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM contacts").Scan(&count)
	if err != nil {
		// Table doesn't exist yet
		return ""
	}

	if count > 0 {
		return fmt.Sprintf("\n## DATABASE CONTEXT\n\nThe database currently contains %d contact(s). Use the database tool to query them if needed for this request.\n\n", count)
	}
	return ""
}

