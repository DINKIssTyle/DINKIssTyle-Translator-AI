//go:build darwin || ios

#include "apple_bridge.h"

#include <dlfcn.h>
#include <stddef.h>
#include <TargetConditionals.h>

extern void DKSTAppleTranslationDidComplete(uint64_t request_id, char *result_json);

typedef int (*available_function)(void);
typedef void (*completion_function)(uint64_t, const char *);
typedef void (*submit_function)(const char *, uint64_t, completion_function);
typedef void (*languages_function)(uint64_t, completion_function);
typedef void (*cancel_function)(uint64_t);

static void completion(uint64_t request_id, const char *result_json) {
    DKSTAppleTranslationDidComplete(request_id, (char *)result_json);
}

#if TARGET_OS_IPHONE

// The iOS bridge is compiled into the application executable. Archive builds
// strip its dynamic symbol table, so dlsym(RTLD_DEFAULT, ...) can fail even
// though the Swift functions are linked and directly callable. Keep explicit
// references for iOS; macOS continues to load the optional bridge dylib below.
extern int DKSTAppleTranslationAvailable(void);
extern void DKSTAppleTranslationSubmit(const char *, uint64_t, completion_function);
extern void DKSTAppleTranslationLanguages(uint64_t, completion_function);
extern void DKSTAppleTranslationCancel(uint64_t);
extern int DKSTGoogleTranslationAvailable(void);
extern void DKSTGoogleTranslationSubmit(const char *, uint64_t, completion_function);
extern void DKSTGoogleTranslationCancel(uint64_t);

int dkst_apple_translation_available(const char *library_path) {
    (void)library_path;
    return DKSTAppleTranslationAvailable();
}

int dkst_apple_translation_submit(const char *library_path, const char *request_json, uint64_t request_id) {
    (void)library_path;
    DKSTAppleTranslationSubmit(request_json, request_id, completion);
    return 1;
}

int dkst_apple_translation_languages(const char *library_path, uint64_t request_id) {
    (void)library_path;
    DKSTAppleTranslationLanguages(request_id, completion);
    return 1;
}

int dkst_apple_translation_cancel(const char *library_path, uint64_t request_id) {
    (void)library_path;
    DKSTAppleTranslationCancel(request_id);
    return 1;
}

int dkst_google_translation_available(void) {
    return DKSTGoogleTranslationAvailable();
}

int dkst_google_translation_submit(const char *request_json, uint64_t request_id) {
    DKSTGoogleTranslationSubmit(request_json, request_id, completion);
    return 1;
}

int dkst_google_translation_cancel(uint64_t request_id) {
    DKSTGoogleTranslationCancel(request_id);
    return 1;
}

#else

int dkst_google_translation_available(void) {
    return 0;
}

int dkst_google_translation_submit(const char *request_json, uint64_t request_id) {
    (void)request_json;
    (void)request_id;
    return 0;
}

int dkst_google_translation_cancel(uint64_t request_id) {
    (void)request_id;
    return 0;
}

static void *translation_library = NULL;

static void *resolve_symbol(const char *library_path, const char *name) {
    void *symbol = dlsym(RTLD_DEFAULT, name);
    if (symbol != NULL) {
        return symbol;
    }
    if (translation_library == NULL && library_path != NULL && library_path[0] != '\0') {
        translation_library = dlopen(library_path, RTLD_NOW | RTLD_LOCAL);
    }
    return translation_library == NULL ? NULL : dlsym(translation_library, name);
}

int dkst_apple_translation_available(const char *library_path) {
    available_function function = (available_function)resolve_symbol(
        library_path, "DKSTAppleTranslationAvailable");
    return function == NULL ? 0 : function();
}

int dkst_apple_translation_submit(const char *library_path, const char *request_json, uint64_t request_id) {
    submit_function function = (submit_function)resolve_symbol(
        library_path, "DKSTAppleTranslationSubmit");
    if (function == NULL) {
        return 0;
    }
    function(request_json, request_id, completion);
    return 1;
}

int dkst_apple_translation_languages(const char *library_path, uint64_t request_id) {
    languages_function function = (languages_function)resolve_symbol(
        library_path, "DKSTAppleTranslationLanguages");
    if (function == NULL) {
        return 0;
    }
    function(request_id, completion);
    return 1;
}

int dkst_apple_translation_cancel(const char *library_path, uint64_t request_id) {
    cancel_function function = (cancel_function)resolve_symbol(
        library_path, "DKSTAppleTranslationCancel");
    if (function == NULL) {
        return 0;
    }
    function(request_id);
    return 1;
}

#endif
