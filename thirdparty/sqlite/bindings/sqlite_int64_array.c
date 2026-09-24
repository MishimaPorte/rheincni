#include "sqlite_int64_array.h"

#include <stdint.h>

typedef struct SQLiteInt64ArrayState {
    unsigned char *Data;
    sqlite3_uint64 Capacity;
    uint32_t Count;
    int Failed;
} SQLiteInt64ArrayState;

void SQLiteInt64ArrayStep(
    sqlite3_context *context,
    int argumentCount,
    sqlite3_value **arguments
) {
    SQLiteInt64ArrayState *state;
    sqlite3_uint64 required;
    sqlite3_uint64 capacity;
    unsigned char *data;
    uint64_t value;
    uint32_t byteIndex;

    state = sqlite3_aggregate_context(context, sizeof(*state));
    if (state == 0) {
        sqlite3_result_error_nomem(context);
        return;
    }
    if (state->Failed) {
        return;
    }
    if (argumentCount != 1 || sqlite3_value_type(arguments[0]) != SQLITE_INTEGER) {
        state->Failed = 1;
        sqlite3_result_error(context, "int64_array_agg expects one non-null integer", -1);
        return;
    }
    if (state->Count == UINT32_MAX) {
        state->Failed = 1;
        sqlite3_result_error_toobig(context);
        return;
    }

    required = 8 + ((sqlite3_uint64)state->Count + 1) * 8;
    if (required > state->Capacity) {
        capacity = state->Capacity == 0 ? 136 : state->Capacity * 2;
        if (capacity < required) {
            capacity = required;
        }
        data = sqlite3_realloc64(state->Data, capacity);
        if (data == 0) {
            state->Failed = 1;
            sqlite3_result_error_nomem(context);
            return;
        }
        state->Data = data;
        state->Capacity = capacity;
    }

    value = (uint64_t)sqlite3_value_int64(arguments[0]);
    for (byteIndex = 0; byteIndex < 8; byteIndex++) {
        state->Data[8 + (sqlite3_uint64)state->Count * 8 + byteIndex] =
            (unsigned char)(value >> (byteIndex * 8));
    }
    state->Count++;
}

void SQLiteInt64ArrayFinal(sqlite3_context *context) {
    SQLiteInt64ArrayState *state;
    unsigned char empty[8] = {'s', 'i', 'a', '1', 0, 0, 0, 0};
    uint32_t count;

    state = sqlite3_aggregate_context(context, 0);
    if (state == 0) {
        sqlite3_result_blob(context, empty, sizeof(empty), SQLITE_TRANSIENT);
        return;
    }
    if (state->Failed) {
        sqlite3_free(state->Data);
        state->Data = 0;
        return;
    }

    state->Data[0] = 's';
    state->Data[1] = 'i';
    state->Data[2] = 'a';
    state->Data[3] = '1';
    count = state->Count;
    state->Data[4] = (unsigned char)count;
    state->Data[5] = (unsigned char)(count >> 8);
    state->Data[6] = (unsigned char)(count >> 16);
    state->Data[7] = (unsigned char)(count >> 24);

    sqlite3_result_blob64(
        context,
        state->Data,
        8 + (sqlite3_uint64)state->Count * 8,
        sqlite3_free
    );
    state->Data = 0;
}

int SQLiteRegisterInt64ArrayAggregate(sqlite3 *database) {
    return sqlite3_create_function_v2(
        database,
        "int64_array_agg",
        1,
        SQLITE_UTF8 | SQLITE_INNOCUOUS,
        0,
        0,
        SQLiteInt64ArrayStep,
        SQLiteInt64ArrayFinal,
        0
    );
}
