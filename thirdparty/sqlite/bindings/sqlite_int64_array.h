#ifndef SQLITE_INT64_ARRAY_H
#define SQLITE_INT64_ARRAY_H

#include "../sqlite3.h"

void SQLiteInt64ArrayStep(sqlite3_context *context, int argumentCount, sqlite3_value **arguments);
void SQLiteInt64ArrayFinal(sqlite3_context *context);
int SQLiteRegisterInt64ArrayAggregate(sqlite3 *database);

#endif
