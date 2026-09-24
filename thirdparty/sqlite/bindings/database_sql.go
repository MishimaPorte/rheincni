package sqlite3

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"time"

	unsafeutil "rheincni/thirdparty/unsafe"
)

type DatabaseSQLConnector struct {
	Path      string
	Existing  *DB
	Mutex     sync.Mutex
	Connected bool
}

type DatabaseSQLDriver struct{}

type DatabaseSQLConnection struct {
	Database      *DB
	CloseDatabase bool
}

type DatabaseSQLTransaction struct {
	Database *DB
}

type DatabaseSQLStatement struct {
	Statement *Statement
}

type DatabaseSQLRows struct {
	Statement      *Statement
	ColumnsValue   []string
	CloseStatement bool
	Finished       bool
}

type DatabaseSQLResult struct {
	LastInsertIDValue int64
	RowsAffectedValue int64
}

func OpenDatabaseSQL(path string) *sql.DB {
	database := sql.OpenDB(&DatabaseSQLConnector{Path: path})
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return database
}

func OpenDatabaseSQLFromDB(database *DB) *sql.DB {
	result := sql.OpenDB(&DatabaseSQLConnector{Existing: database})
	result.SetMaxOpenConns(1)
	result.SetMaxIdleConns(1)
	return result
}

func (connector *DatabaseSQLConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if connector == nil || connector.Path == "" && connector.Existing == nil {
		return nil, fmt.Errorf("sqlite database path must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connector.Mutex.Lock()
	defer connector.Mutex.Unlock()
	if connector.Connected {
		return nil, fmt.Errorf("sqlite database/sql connector supports one connection")
	}
	connector.Connected = true
	if connector.Existing != nil {
		return &DatabaseSQLConnection{Database: connector.Existing}, nil
	}
	database, err := Open(connector.Path)
	if err != nil {
		connector.Connected = false
		return nil, err
	}
	return &DatabaseSQLConnection{Database: database, CloseDatabase: true}, nil
}

func (connector *DatabaseSQLConnector) Driver() driver.Driver {
	return &DatabaseSQLDriver{}
}

func (value *DatabaseSQLDriver) Open(path string) (driver.Conn, error) {
	return (&DatabaseSQLConnector{Path: path}).Connect(context.Background())
}

func (connection *DatabaseSQLConnection) Prepare(query string) (driver.Stmt, error) {
	return connection.PrepareContext(context.Background(), query)
}

func (connection *DatabaseSQLConnection) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	statement, err := connection.Database.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &DatabaseSQLStatement{Statement: statement}, nil
}

func (connection *DatabaseSQLConnection) Close() error {
	if connection == nil || connection.Database == nil || !connection.CloseDatabase {
		return nil
	}
	return connection.Database.Close()
}

func (connection *DatabaseSQLConnection) Begin() (driver.Tx, error) {
	return connection.BeginTx(context.Background(), driver.TxOptions{})
}

func (connection *DatabaseSQLConnection) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if options.ReadOnly {
		if err := connection.Database.Exec("begin;"); err != nil {
			return nil, err
		}
	} else if err := connection.Database.Exec("begin immediate;"); err != nil {
		return nil, err
	}
	return &DatabaseSQLTransaction{Database: connection.Database}, nil
}

func (connection *DatabaseSQLConnection) ExecContext(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Result, error) {
	if len(arguments) == 0 {
		if err := connection.Database.Exec(query); err != nil {
			return nil, err
		}
		return DatabaseSQLResult{LastInsertIDValue: connection.Database.LastInsertRowID(), RowsAffectedValue: connection.Database.Changes()}, nil
	}
	statement, err := connection.Database.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer statement.Close()
	if err := BindDatabaseSQLNamedValues(statement, arguments); err != nil {
		return nil, err
	}
	for {
		hasRow, err := statement.Step()
		if err != nil {
			return nil, err
		}
		if !hasRow {
			break
		}
	}
	return DatabaseSQLResult{LastInsertIDValue: connection.Database.LastInsertRowID(), RowsAffectedValue: connection.Database.Changes()}, nil
}

func (connection *DatabaseSQLConnection) QueryContext(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	statement, err := connection.Database.Prepare(query)
	if err != nil {
		return nil, err
	}
	if err := BindDatabaseSQLNamedValues(statement, arguments); err != nil {
		_ = statement.Close()
		return nil, err
	}
	return NewDatabaseSQLRows(statement, true), nil
}

func (transaction *DatabaseSQLTransaction) Commit() error {
	return transaction.Database.Exec("commit;")
}

func (transaction *DatabaseSQLTransaction) Rollback() error {
	return transaction.Database.Exec("rollback;")
}

func (statement *DatabaseSQLStatement) Close() error {
	return statement.Statement.Close()
}

func (statement *DatabaseSQLStatement) NumInput() int {
	return -1
}

func (statement *DatabaseSQLStatement) Exec(arguments []driver.Value) (driver.Result, error) {
	return statement.ExecContext(context.Background(), DatabaseSQLNamedValues(arguments))
}

func (statement *DatabaseSQLStatement) ExecContext(ctx context.Context, arguments []driver.NamedValue) (driver.Result, error) {
	if err := BindDatabaseSQLNamedValues(statement.Statement, arguments); err != nil {
		return nil, err
	}
	for {
		hasRow, err := statement.Statement.Step()
		if err != nil {
			return nil, err
		}
		if !hasRow {
			break
		}
	}
	database := statement.Statement.SharedState().Database
	result := DatabaseSQLResult{LastInsertIDValue: database.LastInsertRowID(), RowsAffectedValue: database.Changes()}
	return result, statement.Statement.Reset()
}

func (statement *DatabaseSQLStatement) Query(arguments []driver.Value) (driver.Rows, error) {
	return statement.QueryContext(context.Background(), DatabaseSQLNamedValues(arguments))
}

func (statement *DatabaseSQLStatement) QueryContext(ctx context.Context, arguments []driver.NamedValue) (driver.Rows, error) {
	if err := BindDatabaseSQLNamedValues(statement.Statement, arguments); err != nil {
		return nil, err
	}
	return NewDatabaseSQLRows(statement.Statement, false), nil
}

func NewDatabaseSQLRows(statement *Statement, closeStatement bool) *DatabaseSQLRows {
	count := statement.ColumnCount()
	columns := make([]string, count)
	for index := range columns {
		columns[index] = statement.ColumnName(index)
	}
	return &DatabaseSQLRows{Statement: statement, ColumnsValue: columns, CloseStatement: closeStatement}
}

func (rows *DatabaseSQLRows) Columns() []string {
	return rows.ColumnsValue
}

func (rows *DatabaseSQLRows) Close() error {
	if rows.Statement == nil {
		return nil
	}
	rows.Finished = true
	statement := rows.Statement
	rows.Statement = nil
	if rows.CloseStatement {
		return statement.Close()
	}
	return statement.Reset()
}

func (rows *DatabaseSQLRows) Next(destination []driver.Value) error {
	if rows.Finished {
		return io.EOF
	}
	hasRow, err := rows.Statement.Step()
	if err != nil {
		return err
	}
	if !hasRow {
		rows.Finished = true
		return io.EOF
	}
	for index := range destination {
		switch rows.Statement.ColumnType(index) {
		case SQLiteColumnInteger():
			destination[index] = rows.Statement.ColumnInt64(index)
		case SQLiteColumnFloat():
			destination[index] = rows.Statement.ColumnFloat64(index)
		case SQLiteColumnText():
			value := string([]byte(rows.Statement.ColumnTextView(index)))
			if index < len(rows.ColumnsValue) && rows.ColumnsValue[index] == "tstamp" {
				parsed, err := time.Parse("2006-01-02 15:04:05", value)
				if err != nil {
					return fmt.Errorf("parse sqlite timestamp %q: %w", value, err)
				}
				destination[index] = parsed
			} else {
				destination[index] = value
			}
		case SQLiteColumnBlob():
			view := rows.Statement.ColumnBlobView(index)
			value := make([]byte, view.Length)
			copy(value, view.Slice())
			destination[index] = value
		case SQLiteColumnNull():
			destination[index] = nil
		default:
			return fmt.Errorf("unsupported sqlite column type")
		}
	}
	return nil
}

func (result DatabaseSQLResult) LastInsertId() (int64, error) {
	return result.LastInsertIDValue, nil
}

func (result DatabaseSQLResult) RowsAffected() (int64, error) {
	return result.RowsAffectedValue, nil
}

func DatabaseSQLNamedValues(values []driver.Value) []driver.NamedValue {
	result := make([]driver.NamedValue, len(values))
	for index, value := range values {
		result[index] = driver.NamedValue{Ordinal: index + 1, Value: value}
	}
	return result
}

func BindDatabaseSQLNamedValues(statement *Statement, values []driver.NamedValue) error {
	for _, value := range values {
		index := value.Ordinal
		var err error
		switch typed := value.Value.(type) {
		case nil:
			err = statement.BindNull(index)
		case int64:
			err = statement.BindInt64(index, typed)
		case float64:
			err = statement.BindText(index, fmt.Sprintf("%g", typed))
		case bool:
			integer := int64(0)
			if typed {
				integer = 1
			}
			err = statement.BindInt64(index, integer)
		case []byte:
			err = statement.BindBlob(index, unsafeutil.SpanFromSlice[int](typed))
		case string:
			err = statement.BindText(index, typed)
		case time.Time:
			err = statement.BindText(index, typed.Format(time.RFC3339Nano))
		default:
			err = fmt.Errorf("unsupported sqlite argument type %T", value.Value)
		}
		if err != nil {
			return fmt.Errorf("bind sqlite argument %d: %w", index, err)
		}
	}
	return nil
}
