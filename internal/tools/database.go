package tools

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/utils"
)

var db *sql.DB
var cachedSchema string

func InitDatabase(cfg *config.Config) error {
	// Build MySQL DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
	)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		utils.Log.Error("database", "Failed to open database", err)
		return err
	}

	// Test connection
	if err = db.Ping(); err != nil {
		utils.Log.Error("database", "Failed to ping database", err)
		return err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Load schema on startup
	loadDatabaseSchema()

	utils.Log.Success("database", "MySQL database connected successfully", map[string]interface{}{
		"host":     cfg.Database.Host,
		"port":     cfg.Database.Port,
		"database": cfg.Database.Database,
	})

	return nil
}

func loadDatabaseSchema() {
	// First, get list of tables
	tableRows, err := db.Query("SHOW TABLES")
	if err != nil {
		utils.Log.Error("database", "Failed to get table list", err)
		cachedSchema = "\n## DATABASE SCHEMA\n\nNo tables found. The AI can create tables as needed.\n\n"
		return
	}
	defer tableRows.Close()

	var tables []string
	for tableRows.Next() {
		var tableName string
		if err := tableRows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}

	if len(tables) == 0 {
		cachedSchema = "\n## DATABASE SCHEMA\n\nNo tables found. The AI can create tables as needed.\n\n"
		utils.Log.Success("startup", "Database schema cached (no tables)", nil)
		return
	}

	// Get CREATE TABLE statement for each table
	var schema strings.Builder
	schema.WriteString("\n## DATABASE SCHEMA (Use these exact column names!)\n\n")

	for _, tableName := range tables {
		var createStmt string
		var unused string
		err := db.QueryRow("SHOW CREATE TABLE "+tableName).Scan(&unused, &createStmt)
		if err != nil {
			utils.Log.Debug("database", fmt.Sprintf("Failed to get CREATE TABLE for %s", tableName), err)
			continue
		}
		schema.WriteString(createStmt)
		schema.WriteString(";\n\n")
	}

	cachedSchema = schema.String()
	utils.Log.Success("startup", fmt.Sprintf("Database schema cached for %d table(s)", len(tables)), nil)
}

func GetCachedSchema() string {
	return cachedSchema
}

type DatabaseResult struct {
	Success         bool                     `json:"success"`
	Rows            []map[string]interface{} `json:"rows,omitempty"`
	Count           int                      `json:"count,omitempty"`
	Changes         int64                    `json:"changes,omitempty"`
	LastInsertRowID int64                    `json:"lastInsertId,omitempty"`
	Message         string                   `json:"message,omitempty"`
	Error           string                   `json:"error,omitempty"`
	Duration        int64                    `json:"duration,omitempty"`
}

func ExecuteDatabaseQuery(query string, params []interface{}, mode string) DatabaseResult {
	startTime := time.Now()

	queryPreview := query
	if len(query) > 100 {
		queryPreview = query[:100] + "..."
	}

	utils.Log.Database(fmt.Sprintf("Executing %s query", strings.ToUpper(mode)), map[string]interface{}{
		"query":       queryPreview,
		"paramsCount": len(params),
		"hasParams":   len(params) > 0,
	})

	if len(params) > 0 {
		utils.Log.Debug("database", "Query parameters", params)
	}

	var result DatabaseResult
	result.Success = false

	// Trim and check query type
	queryUpper := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(queryUpper, "SELECT") ||
		strings.HasPrefix(queryUpper, "SHOW") ||
		strings.HasPrefix(queryUpper, "DESCRIBE") ||
		strings.HasPrefix(queryUpper, "DESC") ||
		strings.HasPrefix(queryUpper, "EXPLAIN")

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
			"changes":      changes,
			"lastInsertId": lastID,
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
