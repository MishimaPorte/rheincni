package sqlite3

/*
#cgo CFLAGS: -I${SRCDIR}/.. -D_GNU_SOURCE -DSQLITE_OMIT_LOAD_EXTENSION
#include <stdlib.h>
#include <string.h>
#include "sqlite3.c"
#include "sqlite_int64_array.h"

static int sqlite3_bind_text_transient_go(
    sqlite3_stmt *statement,
    int index,
    const char *value,
    int length
) {
    if (value == NULL) {
        value = "";
    }
    return sqlite3_bind_text(statement, index, value, length, SQLITE_TRANSIENT);
}

static int sqlite3_bind_blob_transient_go(
    sqlite3_stmt *statement,
    int index,
    const void *value,
    int length
) {
    return sqlite3_bind_blob(statement, index, value, length, SQLITE_TRANSIENT);
}

static int sqlite3_unit_of_work_authorizer(
    void *read_only,
    int action,
    const char *first,
    const char *second,
    const char *database,
    const char *trigger
) {
    (void)first;
    (void)second;
    (void)database;
    (void)trigger;
    if (action == SQLITE_TRANSACTION || action == SQLITE_SAVEPOINT) {
        return SQLITE_DENY;
    }
    if (read_only == NULL) {
        return SQLITE_OK;
    }
    switch (action) {
    case SQLITE_CREATE_INDEX:
    case SQLITE_CREATE_TABLE:
    case SQLITE_CREATE_TEMP_INDEX:
    case SQLITE_CREATE_TEMP_TABLE:
    case SQLITE_CREATE_TEMP_TRIGGER:
    case SQLITE_CREATE_TEMP_VIEW:
    case SQLITE_CREATE_TRIGGER:
    case SQLITE_CREATE_VIEW:
    case SQLITE_DELETE:
    case SQLITE_DROP_INDEX:
    case SQLITE_DROP_TABLE:
    case SQLITE_DROP_TEMP_INDEX:
    case SQLITE_DROP_TEMP_TABLE:
    case SQLITE_DROP_TEMP_TRIGGER:
    case SQLITE_DROP_TEMP_VIEW:
    case SQLITE_DROP_TRIGGER:
    case SQLITE_DROP_VIEW:
    case SQLITE_INSERT:
    case SQLITE_PRAGMA:
    case SQLITE_UPDATE:
    case SQLITE_ATTACH:
    case SQLITE_DETACH:
    case SQLITE_ALTER_TABLE:
    case SQLITE_REINDEX:
    case SQLITE_ANALYZE:
    case SQLITE_CREATE_VTABLE:
    case SQLITE_DROP_VTABLE:
        return SQLITE_DENY;
    default:
        return SQLITE_OK;
    }
}

static int sqlite3_set_unit_of_work_authorizer(sqlite3 *database, int read_only) {
    return sqlite3_set_authorizer(
        database,
        sqlite3_unit_of_work_authorizer,
        read_only ? (void *)database : NULL
    );
}

static int sqlite3_clear_unit_of_work_authorizer(sqlite3 *database) {
    return sqlite3_set_authorizer(database, NULL, NULL);
}
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	stdunsafe "unsafe"

	"rheincni/thirdparty/unsafe"
)

var ErrClosed = errors.New("sqlite: database is closed")

var ErrStatementClosed = errors.New("sqlite: statement is closed")

var ErrEmptyQuery = errors.New("sqlite: query is empty")

var ErrOperationClosed = errors.New("sqlite: operation is closed")

var ErrInvalidInt64Array = errors.New("sqlite: invalid int64 array")

var ErrConstraint = errors.New("sqlite: constraint failed")

var ErrForeignKeyConstraint = errors.New("sqlite: foreign key constraint failed")

type ResultCode int

const (
	ResultOK            ResultCode = C.SQLITE_OK
	ResultError         ResultCode = C.SQLITE_ERROR
	ResultInternal      ResultCode = C.SQLITE_INTERNAL
	ResultPermission    ResultCode = C.SQLITE_PERM
	ResultAbort         ResultCode = C.SQLITE_ABORT
	ResultBusy          ResultCode = C.SQLITE_BUSY
	ResultLocked        ResultCode = C.SQLITE_LOCKED
	ResultNoMemory      ResultCode = C.SQLITE_NOMEM
	ResultReadOnly      ResultCode = C.SQLITE_READONLY
	ResultInterrupt     ResultCode = C.SQLITE_INTERRUPT
	ResultIOError       ResultCode = C.SQLITE_IOERR
	ResultCorrupt       ResultCode = C.SQLITE_CORRUPT
	ResultNotFound      ResultCode = C.SQLITE_NOTFOUND
	ResultFull          ResultCode = C.SQLITE_FULL
	ResultCannotOpen    ResultCode = C.SQLITE_CANTOPEN
	ResultProtocol      ResultCode = C.SQLITE_PROTOCOL
	ResultEmpty         ResultCode = C.SQLITE_EMPTY
	ResultSchema        ResultCode = C.SQLITE_SCHEMA
	ResultTooBig        ResultCode = C.SQLITE_TOOBIG
	ResultConstraint    ResultCode = C.SQLITE_CONSTRAINT
	ResultMismatch      ResultCode = C.SQLITE_MISMATCH
	ResultMisuse        ResultCode = C.SQLITE_MISUSE
	ResultNoLargeFile   ResultCode = C.SQLITE_NOLFS
	ResultAuthorization ResultCode = C.SQLITE_AUTH
	ResultFormat        ResultCode = C.SQLITE_FORMAT
	ResultRange         ResultCode = C.SQLITE_RANGE
	ResultNotDatabase   ResultCode = C.SQLITE_NOTADB
	ResultNotice        ResultCode = C.SQLITE_NOTICE
	ResultWarning       ResultCode = C.SQLITE_WARNING
	ResultRow           ResultCode = C.SQLITE_ROW
	ResultDone          ResultCode = C.SQLITE_DONE
)

const (
	ResultConstraintCheck        ResultCode = C.SQLITE_CONSTRAINT_CHECK
	ResultConstraintCommitHook   ResultCode = C.SQLITE_CONSTRAINT_COMMITHOOK
	ResultConstraintForeignKey   ResultCode = C.SQLITE_CONSTRAINT_FOREIGNKEY
	ResultConstraintFunction     ResultCode = C.SQLITE_CONSTRAINT_FUNCTION
	ResultConstraintNotNull      ResultCode = C.SQLITE_CONSTRAINT_NOTNULL
	ResultConstraintPrimaryKey   ResultCode = C.SQLITE_CONSTRAINT_PRIMARYKEY
	ResultConstraintTrigger      ResultCode = C.SQLITE_CONSTRAINT_TRIGGER
	ResultConstraintUnique       ResultCode = C.SQLITE_CONSTRAINT_UNIQUE
	ResultConstraintVirtualTable ResultCode = C.SQLITE_CONSTRAINT_VTAB
	ResultConstraintRowID        ResultCode = C.SQLITE_CONSTRAINT_ROWID
	ResultConstraintPinned       ResultCode = C.SQLITE_CONSTRAINT_PINNED
	ResultConstraintDataType     ResultCode = C.SQLITE_CONSTRAINT_DATATYPE
)

type SQLiteError struct {
	ResultCode         ResultCode
	ExtendedResultCode ResultCode
	Message            string
}

func (value *SQLiteError) Error() string {
	if value == nil {
		return "sqlite: unknown error"
	}
	if value.Message != "" {
		return fmt.Sprintf("sqlite: %s", value.Message)
	}
	return fmt.Sprintf("sqlite: error %d", value.ExtendedResultCode)
}

func (value *SQLiteError) Is(target error) bool {
	if value == nil {
		return false
	}
	switch target {
	case ErrConstraint:
		return value.ResultCode == ResultConstraint
	case ErrForeignKeyConstraint:
		return value.ExtendedResultCode == ResultConstraintForeignKey
	}
	targetValue, ok := target.(*SQLiteError)
	if !ok || targetValue == nil {
		return false
	}
	if targetValue.ResultCode != ResultOK && value.ResultCode != targetValue.ResultCode {
		return false
	}
	return targetValue.ExtendedResultCode == ResultOK || value.ExtendedResultCode == targetValue.ExtendedResultCode
}

type SignedInteger interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type DB struct {
	Handle         *C.sqlite3
	OperationMutex sync.Mutex
	OperationState *DBOperationState
}

type DBOperationState struct {
	Mutex  *sync.Mutex
	Handle *C.sqlite3
}

type SQLiteOperation struct {
	Exec                      func(string) error
	Prepare                   func(string) (*Statement, error)
	SetUnitOfWorkAuthorizer   func(bool) error
	ClearUnitOfWorkAuthorizer func() error
}

type Statement struct {
	State *StatementState
}

type StatementState struct {
	Database                  *DB
	Handle                    *C.sqlite3_stmt
	Mutex                     sync.Mutex
	OperationEnter            func() (func(), error)
	OwnerEnter                func() (func(), error)
	DatabaseOperationMutex    *sync.Mutex
	DatabaseOperationIsLocked bool
}

func Sqlite3_Free[P any](p *P) {
	C.sqlite3_free(stdunsafe.Pointer(p))
}

func Open(path string) (*DB, error) {
	cPath := C.CString(path)
	defer unsafe.Cfree(cPath)

	var handle *C.sqlite3
	if code := C.sqlite3_open(cPath, &handle); code != C.SQLITE_OK {
		err := NewSQLiteError(handle, code)
		if handle != nil {
			C.sqlite3_close(handle)
		}
		return nil, err
	}
	if code := C.sqlite3_extended_result_codes(handle, 1); code != C.SQLITE_OK {
		err := NewSQLiteError(handle, code)
		C.sqlite3_close(handle)
		return nil, err
	}

	db := &DB{Handle: handle}
	db.OperationState = &DBOperationState{Mutex: &db.OperationMutex, Handle: handle}
	if err := RegisterInt64ArrayAggregate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func RegisterInt64ArrayAggregate(db *DB) error {
	if db == nil {
		return ErrClosed
	}
	operationMutex := db.SharedOperationMutex()
	operationMutex.Lock()
	defer operationMutex.Unlock()
	handle := db.ConnectionHandle()
	if handle == nil {
		return ErrClosed
	}

	if code := C.SQLiteRegisterInt64ArrayAggregate(handle); code != C.SQLITE_OK {
		return NewSQLiteError(handle, code)
	}
	return nil
}

func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	operationMutex := db.SharedOperationMutex()
	operationMutex.Lock()
	defer operationMutex.Unlock()
	handle := db.ConnectionHandle()
	if handle == nil {
		return nil
	}

	if code := C.sqlite3_close(handle); code != C.SQLITE_OK {
		return NewSQLiteError(handle, code)
	}

	db.Handle = nil
	if db.OperationState != nil {
		db.OperationState.Handle = nil
	}
	return nil
}

func (db *DB) Exec(query string) error {
	operation, leave, err := db.EnterOperation()
	if err != nil {
		return err
	}
	defer leave()
	return operation.Exec(query)
}

func (db *DB) Prepare(query string) (*Statement, error) {
	if db == nil {
		return nil, ErrClosed
	}
	connectionMutex := db.SharedOperationMutex()
	connectionMutex.Lock()
	defer connectionMutex.Unlock()
	databaseHandle := db.ConnectionHandle()
	if databaseHandle == nil {
		return nil, ErrClosed
	}
	var queryData *C.char
	if len(query) > 0 {
		queryData = (*C.char)(stdunsafe.Pointer(stdunsafe.StringData(query)))
	}
	var handle *C.sqlite3_stmt
	code := C.sqlite3_prepare_v2(databaseHandle, queryData, C.int(len(query)), &handle, nil)
	runtime.KeepAlive(query)
	if code != C.SQLITE_OK {
		return nil, NewSQLiteError(databaseHandle, code)
	}
	if handle == nil {
		return nil, ErrEmptyQuery
	}
	return &Statement{State: &StatementState{
		Database:               db,
		Handle:                 handle,
		DatabaseOperationMutex: connectionMutex,
	}}, nil
}

func (db *DB) SharedOperationMutex() *sync.Mutex {
	if db == nil {
		return nil
	}
	if db.OperationState != nil && db.OperationState.Mutex != nil {
		return db.OperationState.Mutex
	}
	return &db.OperationMutex
}

func (db *DB) ConnectionHandle() *C.sqlite3 {
	if db == nil {
		return nil
	}
	if db.OperationState != nil {
		return db.OperationState.Handle
	}
	return db.Handle
}

func (db *DB) EnterOperation() (SQLiteOperation, func(), error) {
	if db == nil {
		return SQLiteOperation{}, nil, ErrClosed
	}
	connectionMutex := db.SharedOperationMutex()
	connectionMutex.Lock()
	if db.ConnectionHandle() == nil {
		connectionMutex.Unlock()
		return SQLiteOperation{}, nil, ErrClosed
	}
	var operationMutex sync.Mutex
	active := true
	statements := make(map[*StatementState]struct{})
	enter := func() (func(), error) {
		operationMutex.Lock()
		if !active {
			operationMutex.Unlock()
			return nil, ErrOperationClosed
		}
		return operationMutex.Unlock, nil
	}
	exec := func(query string) error {
		leave, err := enter()
		if err != nil {
			return err
		}
		defer leave()
		handle := db.ConnectionHandle()
		if handle == nil {
			return ErrClosed
		}
		cQuery := C.CString(query)
		defer unsafe.Cfree(cQuery)
		var message *C.char
		code := C.sqlite3_exec(handle, cQuery, nil, nil, &message)
		if message != nil {
			defer Sqlite3_Free(message)
		}
		if code == C.SQLITE_OK {
			return nil
		}
		sqliteError := NewSQLiteError(handle, code)
		if message != nil {
			sqliteError.Message = strings.Clone(unsafe.String2AnyPtr(message, C.strlen(message)))
		}
		return sqliteError
	}
	prepare := func(query string) (*Statement, error) {
		leave, err := enter()
		if err != nil {
			return nil, err
		}
		defer leave()
		databaseHandle := db.ConnectionHandle()
		if databaseHandle == nil {
			return nil, ErrClosed
		}
		var queryData *C.char
		if len(query) > 0 {
			queryData = (*C.char)(stdunsafe.Pointer(stdunsafe.StringData(query)))
		}
		var handle *C.sqlite3_stmt
		code := C.sqlite3_prepare_v2(databaseHandle, queryData, C.int(len(query)), &handle, nil)
		runtime.KeepAlive(query)
		if code != C.SQLITE_OK {
			return nil, NewSQLiteError(databaseHandle, code)
		}
		if handle == nil {
			return nil, ErrEmptyQuery
		}
		statementState := &StatementState{
			Database:       db,
			Handle:         handle,
			OperationEnter: enter,
		}
		statements[statementState] = struct{}{}
		return &Statement{State: statementState}, nil
	}
	setUnitOfWorkAuthorizer := func(readOnly bool) error {
		leave, err := enter()
		if err != nil {
			return err
		}
		defer leave()
		handle := db.ConnectionHandle()
		if handle == nil {
			return ErrClosed
		}
		readOnlyValue := C.int(0)
		if readOnly {
			readOnlyValue = 1
		}
		if code := C.sqlite3_set_unit_of_work_authorizer(handle, readOnlyValue); code != C.SQLITE_OK {
			return NewSQLiteError(handle, code)
		}
		return nil
	}
	clearUnitOfWorkAuthorizer := func() error {
		leave, err := enter()
		if err != nil {
			return err
		}
		defer leave()
		handle := db.ConnectionHandle()
		if handle == nil {
			return ErrClosed
		}
		if code := C.sqlite3_clear_unit_of_work_authorizer(handle); code != C.SQLITE_OK {
			return NewSQLiteError(handle, code)
		}
		return nil
	}
	leave := func() {
		operationMutex.Lock()
		if !active {
			operationMutex.Unlock()
			return
		}
		for statement := range statements {
			statement.Mutex.Lock()
			if statement.Handle != nil {
				handle := statement.Handle
				statement.Handle = nil
				_ = C.sqlite3_finalize(handle)
			}
			statement.Mutex.Unlock()
			delete(statements, statement)
		}
		if handle := db.ConnectionHandle(); handle != nil {
			_ = C.sqlite3_clear_unit_of_work_authorizer(handle)
		}
		active = false
		operationMutex.Unlock()
		connectionMutex.Unlock()
	}
	return SQLiteOperation{
		Exec:                      exec,
		Prepare:                   prepare,
		SetUnitOfWorkAuthorizer:   setUnitOfWorkAuthorizer,
		ClearUnitOfWorkAuthorizer: clearUnitOfWorkAuthorizer,
	}, leave, nil
}

func (statement *Statement) SharedState() *StatementState {
	if statement == nil {
		return nil
	}
	return statement.State
}

func (statement *Statement) SetOwnerEnter(ownerEnter func() (func(), error)) error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.Handle == nil {
		return ErrStatementClosed
	}
	state.OwnerEnter = ownerEnter
	return nil
}

func (state *StatementState) LockDatabaseOperation() bool {
	if state.DatabaseOperationMutex == nil || state.DatabaseOperationIsLocked {
		return false
	}
	state.DatabaseOperationMutex.Lock()
	return true
}

func (state *StatementState) UnlockDatabaseOperation(locked bool) {
	if locked {
		state.DatabaseOperationMutex.Unlock()
	}
}

func (state *StatementState) ReleaseDatabaseOperation() {
	if state.DatabaseOperationIsLocked {
		state.DatabaseOperationIsLocked = false
		state.DatabaseOperationMutex.Unlock()
	}
}

func (statement *Statement) EnterOperationGuard() (func(), error) {
	state := statement.SharedState()
	if state == nil {
		return nil, ErrStatementClosed
	}
	state.Mutex.Lock()
	if state.Handle == nil {
		state.Mutex.Unlock()
		return nil, ErrStatementClosed
	}
	ownerEnter := state.OwnerEnter
	operationEnter := state.OperationEnter
	state.Mutex.Unlock()
	var ownerLeave func()
	if ownerEnter != nil {
		var err error
		ownerLeave, err = ownerEnter()
		if err != nil {
			return nil, err
		}
	}
	var operationLeave func()
	if operationEnter != nil {
		var err error
		operationLeave, err = operationEnter()
		if err != nil {
			if ownerLeave != nil {
				ownerLeave()
			}
			return nil, err
		}
	}
	return func() {
		if operationLeave != nil {
			operationLeave()
		}
		if ownerLeave != nil {
			ownerLeave()
		}
	}, nil
}

func (statement *Statement) Close() error {
	state := statement.SharedState()
	if state == nil {
		return nil
	}
	state.Mutex.Lock()
	operationEnter := state.OperationEnter
	state.Mutex.Unlock()
	var operationLeave func()
	if operationEnter != nil {
		var err error
		operationLeave, err = operationEnter()
		if err != nil {
			state.Mutex.Lock()
			closed := state.Handle == nil
			state.Mutex.Unlock()
			if closed {
				return nil
			}
			return err
		}
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	defer state.ReleaseDatabaseOperation()
	if state.Handle == nil {
		return nil
	}

	handle := state.Handle
	state.Handle = nil
	if code := C.sqlite3_finalize(handle); code != C.SQLITE_OK {
		if state.Database != nil {
			return NewSQLiteError(state.Database.ConnectionHandle(), code)
		}
		return NewSQLiteError(nil, code)
	}
	return nil
}

func (statement *Statement) BindText(index int, value string) error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		return ErrClosed
	}

	var valueData *C.char
	if len(value) > 0 {
		valueData = (*C.char)(stdunsafe.Pointer(stdunsafe.StringData(value)))
	}
	code := C.sqlite3_bind_text_transient_go(
		state.Handle,
		C.int(index),
		valueData,
		C.int(len(value)),
	)
	runtime.KeepAlive(value)
	if code != C.SQLITE_OK {
		return NewSQLiteError(state.Database.ConnectionHandle(), code)
	}
	return nil
}

func (statement *Statement) BindInt64(index int, value int64) error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		return ErrClosed
	}

	if code := C.sqlite3_bind_int64(state.Handle, C.int(index), C.sqlite3_int64(value)); code != C.SQLITE_OK {
		return NewSQLiteError(state.Database.ConnectionHandle(), code)
	}
	return nil
}

func (statement *Statement) BindBlob(index int, value unsafe.Span[byte]) error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		return ErrClosed
	}
	var valueData stdunsafe.Pointer
	if value.Length > 0 {
		valueData = stdunsafe.Pointer(value.Items)
	}
	code := C.sqlite3_bind_blob_transient_go(
		state.Handle,
		C.int(index),
		valueData,
		C.int(value.Length),
	)
	runtime.KeepAlive(value)
	if code != C.SQLITE_OK {
		return NewSQLiteError(state.Database.ConnectionHandle(), code)
	}
	return nil
}

func (statement *Statement) BindNull(index int) error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		return ErrClosed
	}
	if code := C.sqlite3_bind_null(state.Handle, C.int(index)); code != C.SQLITE_OK {
		return NewSQLiteError(state.Database.ConnectionHandle(), code)
	}
	return nil
}

func (statement *Statement) Reset() error {
	state := statement.SharedState()
	if state == nil {
		return ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	defer state.ReleaseDatabaseOperation()
	if state.Handle == nil {
		return ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		return ErrClosed
	}

	if code := C.sqlite3_reset(state.Handle); code != C.SQLITE_OK {
		return NewSQLiteError(state.Database.ConnectionHandle(), code)
	}
	return nil
}

func (statement *Statement) Step() (bool, error) {
	state := statement.SharedState()
	if state == nil {
		return false, ErrStatementClosed
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return false, operationErr
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.DatabaseOperationMutex != nil && !state.DatabaseOperationIsLocked {
		state.DatabaseOperationMutex.Lock()
		state.DatabaseOperationIsLocked = true
	}
	if state.Handle == nil {
		state.ReleaseDatabaseOperation()
		return false, ErrStatementClosed
	}
	if state.Database == nil || state.Database.ConnectionHandle() == nil {
		state.ReleaseDatabaseOperation()
		return false, ErrClosed
	}
	databaseHandle := state.Database.ConnectionHandle()
	code := C.sqlite3_step(state.Handle)
	switch code {
	case C.SQLITE_ROW:
		return true, nil
	case C.SQLITE_DONE:
		state.ReleaseDatabaseOperation()
		return false, nil
	default:
		sqliteError := NewSQLiteError(databaseHandle, code)
		state.ReleaseDatabaseOperation()
		return false, sqliteError
	}
}

func (statement *Statement) ColumnInt64(index int) int64 {
	state := statement.SharedState()
	if state == nil {
		return 0
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return 0
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return 0
	}
	return int64(C.sqlite3_column_int64(state.Handle, C.int(index)))
}

func (statement *Statement) ColumnCount() int {
	state := statement.SharedState()
	if state == nil {
		return 0
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.Handle == nil {
		return 0
	}
	return int(C.sqlite3_column_count(state.Handle))
}

func (statement *Statement) ColumnName(index int) string {
	state := statement.SharedState()
	if state == nil {
		return ""
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.Handle == nil {
		return ""
	}
	value := C.sqlite3_column_name(state.Handle, C.int(index))
	if value == nil {
		return ""
	}
	return C.GoString(value)
}

func (statement *Statement) ColumnType(index int) int {
	state := statement.SharedState()
	if state == nil {
		return int(C.SQLITE_NULL)
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.Handle == nil {
		return int(C.SQLITE_NULL)
	}
	return int(C.sqlite3_column_type(state.Handle, C.int(index)))
}

func (statement *Statement) ColumnFloat64(index int) float64 {
	state := statement.SharedState()
	if state == nil {
		return 0
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if state.Handle == nil {
		return 0
	}
	return float64(C.sqlite3_column_double(state.Handle, C.int(index)))
}

func SQLiteColumnInteger() int {
	return int(C.SQLITE_INTEGER)
}

func SQLiteColumnFloat() int {
	return int(C.SQLITE_FLOAT)
}

func SQLiteColumnText() int {
	return int(C.SQLITE_TEXT)
}

func SQLiteColumnBlob() int {
	return int(C.SQLITE_BLOB)
}

func SQLiteColumnNull() int {
	return int(C.SQLITE_NULL)
}

func (db *DB) Changes() int64 {
	if db == nil || db.ConnectionHandle() == nil {
		return 0
	}
	return int64(C.sqlite3_changes64(db.ConnectionHandle()))
}

func (db *DB) LastInsertRowID() int64 {
	if db == nil || db.ConnectionHandle() == nil {
		return 0
	}
	return int64(C.sqlite3_last_insert_rowid(db.ConnectionHandle()))
}

func (statement *Statement) ColumnTextView(index int) string {
	state := statement.SharedState()
	if state == nil {
		return ""
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return ""
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return ""
	}
	value := C.sqlite3_column_text(state.Handle, C.int(index))
	if value == nil {
		return ""
	}
	length := C.sqlite3_column_bytes(state.Handle, C.int(index))
	return unsafe.String2AnyPtr(value, length)
}

func (statement *Statement) ColumnBlobView(index int) unsafe.Span[byte] {
	state := statement.SharedState()
	if state == nil {
		return unsafe.Span[byte]{}
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return unsafe.Span[byte]{}
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return unsafe.Span[byte]{}
	}
	value := C.sqlite3_column_blob(state.Handle, C.int(index))
	if value == nil {
		return unsafe.Span[byte]{}
	}
	length := C.sqlite3_column_bytes(state.Handle, C.int(index))
	return unsafe.Span[byte]{
		Items:  (*byte)(value),
		Length: int(length),
	}
}

func (statement *Statement) ColumnIsNull(index int) bool {
	state := statement.SharedState()
	if state == nil {
		return true
	}
	operationLeave, operationErr := statement.EnterOperationGuard()
	if operationErr != nil {
		return true
	}
	if operationLeave != nil {
		defer operationLeave()
	}
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	locked := state.LockDatabaseOperation()
	defer state.UnlockDatabaseOperation(locked)
	if state.Handle == nil {
		return true
	}
	return C.sqlite3_column_type(state.Handle, C.int(index)) == C.SQLITE_NULL
}

func DecodeInt64Array[T SignedInteger](data unsafe.Span[byte]) (unsafe.Span[T], error) {
	return DecodeInt64ArrayInArena[T](data, nil)
}

func DecodeInt64ArrayInArena[T SignedInteger](data unsafe.Span[byte], arena *unsafe.Arena) (unsafe.Span[T], error) {
	var values unsafe.Span[T]
	if data.Length < 8 || data.At(0) != 's' || data.At(1) != 'i' || data.At(2) != 'a' || data.At(3) != '1' {
		return values, ErrInvalidInt64Array
	}

	dataSlice := data.Slice()
	count := binary.LittleEndian.Uint32(dataSlice[4:8])
	expectedLength := uint64(8) + uint64(count)*8
	if uint64(data.Length) != expectedLength {
		return values, fmt.Errorf("%w: length %d does not match element count %d", ErrInvalidInt64Array, data.Length, count)
	}
	if count == 0 {
		return values, nil
	}

	if arena == nil {
		values = unsafe.NewSpanDefault[T](int(count))
	} else {
		values = unsafe.Allocn[T](arena, uintptr(count))
	}
	for index := range count {
		offset := 8 + int(index)*8
		value := int64(binary.LittleEndian.Uint64(dataSlice[offset : offset+8]))
		converted := T(value)
		if int64(converted) != value {
			return unsafe.Span[T]{}, fmt.Errorf("%w: value %d overflows target type", ErrInvalidInt64Array, value)
		}
		*values.RefAt(int(index)) = converted
	}
	return values, nil
}

func NewSQLiteError(handle *C.sqlite3, code C.int) *SQLiteError {
	extendedResultCode := ResultCode(code)
	message := ""
	if handle != nil {
		handleExtendedResultCode := ResultCode(C.sqlite3_extended_errcode(handle))
		if handleExtendedResultCode&0xff == ResultCode(code)&0xff {
			extendedResultCode = handleExtendedResultCode
		}
		messageData := C.sqlite3_errmsg(handle)
		message = strings.Clone(unsafe.String2AnyPtr(messageData, C.strlen(messageData)))
	}
	return &SQLiteError{
		ResultCode:         extendedResultCode & 0xff,
		ExtendedResultCode: extendedResultCode,
		Message:            message,
	}
}
