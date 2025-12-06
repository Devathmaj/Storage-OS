#include "query.h"
#include <stdlib.h>
#include <string.h>
#include <regex.h>
#include <stdio.h>
#include <ctype.h>

// Token types for query parser
typedef enum {
    TOKEN_NAME,
    TOKEN_TYPE,
    TOKEN_SIZE,
    TOKEN_PATH,
    TOKEN_AND,
    TOKEN_OR,
    TOKEN_REGEX,
    TOKEN_GT,
    TOKEN_LT,
    TOKEN_EQ,
    TOKEN_VALUE,
    TOKEN_END
} TokenType;

typedef struct {
    TokenType type;
    char *value;
} Token;

// Optimized query parser
static Token* tokenize(const char *query_string, int *token_count) {
    Token *tokens = malloc(sizeof(Token) * 100);
    *token_count = 0;
    
    const char *p = query_string;
    while (*p) {
        while (isspace(*p)) p++;
        if (!*p) break;
        
        Token *tok = &tokens[(*token_count)++];
        
        if (strncmp(p, "AND", 3) == 0) {
            tok->type = TOKEN_AND;
            tok->value = NULL;
            p += 3;
        } else if (strncmp(p, "OR", 2) == 0) {
            tok->type = TOKEN_OR;
            tok->value = NULL;
            p += 2;
        } else if (strncmp(p, "name~", 5) == 0) {
            tok->type = TOKEN_REGEX;
            p += 5;
            const char *start = p;
            while (*p && !isspace(*p) && strncmp(p, "AND", 3) != 0 && strncmp(p, "OR", 2) != 0) p++;
            tok->value = strndup(start, p - start);
        } else if (strncmp(p, "name=", 5) == 0) {
            tok->type = TOKEN_NAME;
            p += 5;
            const char *start = p;
            while (*p && !isspace(*p) && strncmp(p, "AND", 3) != 0 && strncmp(p, "OR", 2) != 0) p++;
            tok->value = strndup(start, p - start);
        } else if (strncmp(p, "type=", 5) == 0) {
            tok->type = TOKEN_TYPE;
            p += 5;
            const char *start = p;
            while (*p && !isspace(*p) && strncmp(p, "AND", 3) != 0 && strncmp(p, "OR", 2) != 0) p++;
            tok->value = strndup(start, p - start);
        } else if (strncmp(p, "path=", 5) == 0) {
            tok->type = TOKEN_PATH;
            p += 5;
            const char *start = p;
            while (*p && !isspace(*p) && strncmp(p, "AND", 3) != 0 && strncmp(p, "OR", 2) != 0) p++;
            tok->value = strndup(start, p - start);
        } else if (strncmp(p, "size>", 5) == 0) {
            tok->type = TOKEN_GT;
            p += 5;
            const char *start = p;
            while (*p && isdigit(*p)) p++;
            tok->value = strndup(start, p - start);
        } else if (strncmp(p, "size<", 5) == 0) {
            tok->type = TOKEN_LT;
            p += 5;
            const char *start = p;
            while (*p && isdigit(*p)) p++;
            tok->value = strndup(start, p - start);
        } else {
            // Skip unknown
            p++;
        }
    }
    
    tokens[*token_count].type = TOKEN_END;
    return tokens;
}

// Advanced query with optimized parsing and execution
int query_advanced(FileIndex *index, const char *query_string, FileMetadata **results, size_t *result_count) {
    *result_count = 0;
    
    int token_count;
    Token *tokens = tokenize(query_string, &token_count);
    
    if (token_count == 0) {
        free(tokens);
        return 0;
    }
    
    // Allocate temporary result sets
    FileMetadata **temp_results = malloc(sizeof(FileMetadata*) * index->count);
    size_t temp_count = 0;
    
    // Execute first condition
    Token *tok = &tokens[0];
    if (tok->type == TOKEN_REGEX) {
        regex_t regex;
        if (regcomp(&regex, tok->value, REG_EXTENDED | REG_NOSUB) == 0) {
            for (size_t i = 0; i < index->count; i++) {
                if (regexec(&regex, index->files[i].name, 0, NULL, 0) == 0) {
                    temp_results[temp_count++] = &index->files[i];
                }
            }
            regfree(&regex);
        }
    } else if (tok->type == TOKEN_NAME) {
        query_by_name(index, tok->value, temp_results, &temp_count);
    } else if (tok->type == TOKEN_TYPE) {
        query_by_type(index, tok->value, temp_results, &temp_count);
    } else if (tok->type == TOKEN_PATH) {
        query_by_path(index, tok->value, temp_results, &temp_count);
    } else if (tok->type == TOKEN_GT || tok->type == TOKEN_LT) {
        size_t size_val = atoi(tok->value);
        size_t min = tok->type == TOKEN_GT ? size_val : 0;
        size_t max = tok->type == TOKEN_LT ? size_val : (size_t)-1;
        query_by_size_range(index, min, max, temp_results, &temp_count);
    }
    
    // Process additional conditions with AND/OR
    for (int i = 1; i < token_count; i++) {
        tok = &tokens[i];
        
        if (tok->type == TOKEN_AND) {
            // AND: filter current results
            if (i + 1 < token_count) {
                Token *next = &tokens[i + 1];
                size_t filtered_count = 0;
                
                for (size_t j = 0; j < temp_count; j++) {
                    int match = 0;
                    
                    if (next->type == TOKEN_TYPE && strcmp(temp_results[j]->type, next->value) == 0) {
                        match = 1;
                    } else if (next->type == TOKEN_GT && temp_results[j]->size > (size_t)atoi(next->value)) {
                        match = 1;
                    } else if (next->type == TOKEN_LT && temp_results[j]->size < (size_t)atoi(next->value)) {
                        match = 1;
                    } else if (next->type == TOKEN_NAME && strcmp(temp_results[j]->name, next->value) == 0) {
                        match = 1;
                    }
                    
                    if (match) {
                        temp_results[filtered_count++] = temp_results[j];
                    }
                }
                temp_count = filtered_count;
                i++;  // Skip next token as we processed it
            }
        } else if (tok->type == TOKEN_OR) {
            // OR: add more results
            if (i + 1 < token_count) {
                Token *next = &tokens[i + 1];
                FileMetadata **more_results = malloc(sizeof(FileMetadata*) * index->count);
                size_t more_count = 0;
                
                if (next->type == TOKEN_TYPE) {
                    query_by_type(index, next->value, more_results, &more_count);
                } else if (next->type == TOKEN_NAME) {
                    query_by_name(index, next->value, more_results, &more_count);
                }
                
                // Merge results (remove duplicates)
                for (size_t j = 0; j < more_count; j++) {
                    int found = 0;
                    for (size_t k = 0; k < temp_count; k++) {
                        if (temp_results[k] == more_results[j]) {
                            found = 1;
                            break;
                        }
                    }
                    if (!found) {
                        temp_results[temp_count++] = more_results[j];
                    }
                }
                free(more_results);
                i++;  // Skip next token
            }
        }
    }
    
    // Copy to results
    memcpy(results, temp_results, temp_count * sizeof(FileMetadata*));
    *result_count = temp_count;
    
    // Cleanup
    for (int i = 0; i < token_count; i++) {
        free(tokens[i].value);
    }
    free(tokens);
    free(temp_results);
    
    return 0;
}