# datarecording

Package `datarecording` provides a structured data recording and reading
infrastructure backed by SQLite. It is used by the tracing, monitoring, and
simulation packages to persist simulation results.

## DataRecorder

The `DataRecorder` interface writes structured data into SQLite tables:

```go
type DataRecorder interface {
    CreateTable(tableName string, sampleEntry any)
    InsertData(tableName string, entry any)
    ListTables() []string
    Flush()
    Close() error
}
```

### Creating a Recorder

```go
recorder := datarecording.NewDataRecorder("my_simulation")
defer recorder.Close()
```

This creates a SQLite file `my_simulation.sqlite3`. An execution info table
(`exec_info`) is automatically created to record start time, command, and
working directory.

### Defining Tables

Tables are defined by passing a sample struct. Fields become columns;
struct tags control indexing:

```go
type MyEntry struct {
    ID        uint64  `akita_data:"unique"`
    Category  string  `akita_data:"index"`
    Value     float64
    Ignore    string  `akita_data:"ignore"`
    Location  string  `akita_data:"location"` // auto-mapped to int IDs
}

recorder.CreateTable("my_results", MyEntry{})
```

#### Struct Tags

| Tag | Effect |
|-----|--------|
| `akita_data:"unique"` | Creates a unique index on this column |
| `akita_data:"index"` | Creates a non-unique index |
| `akita_data:"ignore"` | Field is not stored |
| `akita_data:"location"` | String auto-mapped to integer ID via a `location` table |

Only primitive types are allowed as fields (bool, int/uint variants, float,
complex, string).

### Inserting Data

```go
recorder.InsertData("my_results", MyEntry{
    ID:       1,
    Category: "latency",
    Value:    42.5,
    Location: "Cache.L1",
})
```

Data is batched internally (default 100,000 entries) and flushed automatically
or via `Flush()`.

## DataReader

The `DataReader` interface reads from SQLite databases:

```go
reader := datarecording.NewReader("my_simulation.sqlite3")
defer reader.Close()

reader.MapTable("my_results", MyEntry{})

results, totalCount, err := reader.Query(ctx, "my_results", datarecording.QueryParams{
    Where:   "Category = ?",
    Args:    []any{"latency"},
    OrderBy: "Value DESC",
    Limit:   100,
    Offset:  0,
})
```

### QueryParams

| Field | Description |
|-------|-------------|
| `Where` | SQL WHERE clause (without `WHERE` keyword) |
| `Args` | Placeholder arguments for the WHERE clause |
| `Limit` | Max rows to return (0 = unlimited) |
| `Offset` | Number of rows to skip |
| `OrderBy` | SQL ORDER BY clause (without `ORDER BY` keyword) |

Results are returned as `[]any` where each element is a pointer to the
mapped struct type.
