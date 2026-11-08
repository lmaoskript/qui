#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct qui_app qui_app;

/* Sets the identity used for the Wayland app id. Call before qui_open. It must
 * match the basename of the app's .desktop file. */
void qui_set_app_id(const char *id);

qui_app *qui_open(const char *title, int width, int height);
void qui_close(qui_app *app);

/* Requests a frame. Safe to call from any thread while the app is open. */
void qui_wake(qui_app *app);

/* Blocks until a frame is needed. Returns the results table, or NULL on quit.
 * The pointer stays valid until the next qui_frame_begin. */
const uint8_t *qui_frame_begin(qui_app *app, size_t *len);

/* Consumes one wire-format frame. The engine does not retain `wire`. */
void qui_frame_end(qui_app *app, const uint8_t *wire, size_t len);

#ifdef __cplusplus
}
#endif
