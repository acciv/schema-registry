#ifndef SCHEMAR_CORE_H
#define SCHEMAR_CORE_H

#ifdef __cplusplus
extern "C" {
#endif

const char *schemar_version(void);
char *schemar_check(const char *old_schema_json, const char *new_schema_json);
char *schemar_fingerprint(const char *schema_json);
void schemar_free(char *ptr);

#ifdef __cplusplus
}
#endif

#endif
