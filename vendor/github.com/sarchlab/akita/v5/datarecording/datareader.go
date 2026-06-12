package datarecording

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
)

// QueryParams encapsulates all query parameters
type QueryParams struct {
	// Where holds the WHERE clause without the "WHERE" keyword
	// Example: "timestamp > ? AND category = ?"
	Where string

	// Args holds the arguments for the placeholders in Where
	Args []any

	// Limit is the maximum number of records to return (pagination)
	// Set to 0 for no limit
	Limit int

	// Offset is the number of records to skip (pagination)
	Offset int

	// OrderBy specifies sorting, without the "ORDER BY" keywords
	// Example: "timestamp DESC"
	OrderBy string
}

// DataReader can read and store data
type DataReader interface {
	// MapTable establishes a mapping between a database table and a Go struct
	// type. This mapping is required before querying a table.
	MapTable(tableName string, sampleEntry any)

	// ListTables returns a list of all tables that have been mapped.
	ListTables() []string

	// Query executes a query on a table and returns the results.
	Query(ctx context.Context, tableName string, params QueryParams) (
		results []any,
		totalCount int,
		err error,
	)

	// Close closes the reader
	Close() error
}

// SQLiteReader reads data from SQLite database
type sqliteReader struct {
	*sql.DB

	typeMap map[string]reflect.Type // Maps table names to struct types
}

// NewReader creates a new DataReader
func NewReader(dbFilename string) DataReader {
	// Open the database
	db, err := sql.Open("sqlite", dbFilename)
	if err != nil {
		panic(err)
	}

	return &sqliteReader{
		DB:      db,
		typeMap: make(map[string]reflect.Type),
	}
}

// NewReaderWithDB creates a new DataReader with a given database
func NewReaderWithDB(db *sql.DB) DataReader {
	return &sqliteReader{
		DB:      db,
		typeMap: make(map[string]reflect.Type),
	}
}

func (r *sqliteReader) MapTable(tableName string, sampleEntry any) {
	r.typeMap[tableName] = reflect.TypeOf(sampleEntry)
}

func (r *sqliteReader) ListTables() []string {
	tables := make([]string, 0, len(r.typeMap))
	for table := range r.typeMap {
		tables = append(tables, table)
	}

	return tables
}

func (r *sqliteReader) Query(
	ctx context.Context,
	tableName string,
	params QueryParams,
) ([]any, int, error) {
	structType, ok := r.typeMap[tableName]
	if !ok {
		return nil, 0, fmt.Errorf("no mapping found for table: %s", tableName)
	}

	query := fmt.Sprintf("SELECT * FROM %s", tableName)

	if params.Where != "" {
		query += " WHERE " + params.Where
	}

	if params.OrderBy != "" {
		query += " ORDER BY " + params.OrderBy
	}

	if params.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", params.Limit)
		if params.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", params.Offset)
		}
	}

	totalCount, err := r.queryTotalCount(ctx, tableName, params)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, query, params.Args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanRowsToSlice(ctx, rows, structType), totalCount, nil
}

func (r *sqliteReader) queryTotalCount(
	ctx context.Context,
	tableName string,
	params QueryParams,
) (int, error) {
	var totalCount int

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)

	if params.Where != "" {
		countQuery += " WHERE " + params.Where
	}

	err := r.DB.QueryRowContext(ctx, countQuery, params.Args...).Scan(&totalCount)
	if err != nil {
		return 0, err
	}

	return totalCount, nil
}

// Helper function to scan rows into struct instances
func (r *sqliteReader) scanRowsToSlice(
	ctx context.Context,
	rows *sql.Rows,
	structType reflect.Type,
) []any {
	var results []any

	columns, err := rows.Columns()
	if err != nil {
		return nil // Error getting columns
	}

	hasLocation := r.checkLocationTag(structType)

	fieldMap := make(map[string]int)

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldMap[field.Name] = i
	}

	for rows.Next() {
		structPtr := reflect.New(structType)
		structVal := structPtr.Elem()
		scanTargets := make([]interface{}, len(columns))

		for i, colName := range columns {
			if fieldIdx, ok := fieldMap[colName]; ok {
				fieldVal := structVal.Field(fieldIdx)
				scanTargets[i] = fieldVal.Addr().Interface()
			} else {
				var placeholder interface{}

				scanTargets[i] = &placeholder
			}
		}

		err := rows.Scan(scanTargets...)
		if err != nil {
			panic(err)
		}

		if hasLocation {
			r.restoreStrLocation(ctx, structVal, structType)
		}

		results = append(results, structPtr.Interface())
	}

	err = rows.Err()
	if err != nil {
		panic(err)
	}

	return results
}

func (r *sqliteReader) Close() error {
	return r.DB.Close()
}

// Helper function that restores location index back to string
func (r *sqliteReader) restoreStrLocation(
	ctx context.Context,
	structVal reflect.Value,
	structType reflect.Type,
) {
	var strLocation string // Retrieves the real location

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		dbTag, ok := field.Tag.Lookup("akita_data")
		if ok && dbTag == "location" {
			fieldVal := structVal.Field(i)
			index := fieldVal.String()

			stmt := fmt.Sprintf("SELECT Locale FROM"+
				" location WHERE ID = %s", index)
			r.DB.QueryRowContext(ctx, stmt).Scan(&strLocation)

			fieldVal.SetString(strLocation)
		}
	}
}

// Helper function that checks whehter this struct has location tag
func (r *sqliteReader) checkLocationTag(
	structType reflect.Type,
) bool {
	hasLocation := false

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		dbTag, ok := field.Tag.Lookup("akita_data")
		if ok && dbTag == "location" {
			kind := field.Type.Kind()
			if kind != reflect.String {
				panic("location field type mismatch")
			}

			hasLocation = true
		}
	}

	return hasLocation
}
