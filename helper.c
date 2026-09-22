#include <stdio.h>
#include <stdarg.h>

#define TEMP_BUF_SIZE 1024 * 1024
static char __temp_sprintf_buf[TEMP_BUF_SIZE];

const char *temp_sprintf(const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    vsnprintf(&__temp_sprintf_buf[0], TEMP_BUF_SIZE, fmt, args);
    va_end(args);
    return &__temp_sprintf_buf[0];
}

