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

/* Requests a frame after `ms` milliseconds. */
void qui_wake_after(qui_app *app, uint32_t ms);

/* Tells the backend the app acted on this frame's Escape key. On Android an
 * unclaimed Back press sends the app to the background. */
void qui_consume_back(qui_app *app);

/* Opens the system file picker without blocking. Chosen paths are queued and
 * the app is woken; take them one at a time with qui_picked_file. */
void qui_pick_files(qui_app *app, int multiple);

/* Pops the next chosen path, or NULL when none. Valid until the next call. */
const char *qui_picked_file(qui_app *app);

/* Puts UTF-8 text on the system clipboard. */
void qui_set_clipboard(qui_app *app, const char *text);

/* Blocks until a frame is needed. Returns the results table, or NULL on quit.
 * The pointer stays valid until the next qui_frame_begin. */
const uint8_t *qui_frame_begin(qui_app *app, size_t *len);

/* Consumes one wire-format frame. The engine does not retain `wire`. */
void qui_frame_end(qui_app *app, const uint8_t *wire, size_t len);

#ifdef __cplusplus
}
#endif
