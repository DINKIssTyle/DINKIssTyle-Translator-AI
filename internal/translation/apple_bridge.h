#ifndef DKST_APPLE_TRANSLATION_BRIDGE_H
#define DKST_APPLE_TRANSLATION_BRIDGE_H

#include <stdint.h>

int dkst_apple_translation_available(const char *library_path);
int dkst_apple_translation_submit(const char *library_path, const char *request_json, uint64_t request_id);
int dkst_apple_translation_languages(const char *library_path, uint64_t request_id);
int dkst_apple_translation_cancel(const char *library_path, uint64_t request_id);

int dkst_google_translation_available(void);
int dkst_google_translation_submit(const char *request_json, uint64_t request_id);
int dkst_google_translation_cancel(uint64_t request_id);

#endif
