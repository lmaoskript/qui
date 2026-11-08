//go:build linux && !android && cgo

#include "internal.hpp"

#include <gio/gio.h>
#include <gio/gunixfdlist.h>
#include <unistd.h>

#include <cstdio>
#include <mutex>
#include <random>

namespace qui::sys {

namespace {

constexpr const char *kBus = "org.freedesktop.portal.Desktop";
constexpr const char *kPath = "/org/freedesktop/portal/desktop";
constexpr const char *kRegistry = "org.freedesktop.host.portal.Registry";
constexpr const char *kScreenCast = "org.freedesktop.portal.ScreenCast";
constexpr int kDefaultTimeoutMs = 5 * 60 * 1000;

struct Portal {
  std::mutex mutex;
  GDBusConnection *connection = nullptr;
  GMainContext *context = nullptr;
  std::string sender, registered;

  bool connect() {
    if (connection) return true;
    GError *error = nullptr;
    char *address = g_dbus_address_get_for_bus_sync(G_BUS_TYPE_SESSION, nullptr, &error);
    if (address) {
      connection = g_dbus_connection_new_for_address_sync(
          address, static_cast<GDBusConnectionFlags>(G_DBUS_CONNECTION_FLAGS_AUTHENTICATION_CLIENT | G_DBUS_CONNECTION_FLAGS_MESSAGE_BUS_CONNECTION),
          nullptr, nullptr, &error);
      g_free(address);
    }
    if (!connection) {
      fail(std::string("could not reach the session bus: ") + (error ? error->message : "unknown error"));
      if (error) g_error_free(error);
      return false;
    }
    g_dbus_connection_set_exit_on_close(connection, FALSE);
    context = g_main_context_new();
    sender = g_dbus_connection_get_unique_name(connection);
    sender.erase(0, sender.rfind(':') + 1);
    for (char &c : sender)
      if (c == '.') c = '_';
    return true;
  }
};

Portal &portal() {
  static Portal instance;
  return instance;
}

struct Reply {
  uint32_t status = 2;
  GVariant *results = nullptr;
  bool done = false;
};

void on_response(GDBusConnection *, const gchar *, const gchar *, const gchar *, const gchar *, GVariant *parameters, gpointer data) {
  auto *reply = static_cast<Reply *>(data);
  g_variant_get(parameters, "(u@a{sv})", &reply->status, &reply->results);
  reply->done = true;
}

gboolean on_timeout(gpointer data) {
  *static_cast<bool *>(data) = true;
  return G_SOURCE_REMOVE;
}

void on_session_closed(GDBusConnection *, const gchar *, const gchar *, const gchar *, const gchar *, GVariant *, gpointer data) {
  static_cast<PortalSession *>(data)->closed = true;
}

std::string token(const char *prefix) {
  static std::mt19937 generator{std::random_device{}()};
  return std::string(prefix) + std::to_string(generator());
}

bool request(Portal &p, const char *method, const std::string &handle_token, GVariant *parameters, int timeout_ms, Reply &reply) {
  const std::string path = std::string(kPath) + "/request/" + p.sender + "/" + handle_token;
  const guint subscription = g_dbus_connection_signal_subscribe(p.connection, kBus, "org.freedesktop.portal.Request", "Response", path.c_str(), nullptr,
                                                                G_DBUS_SIGNAL_FLAGS_NONE, on_response, &reply, nullptr);
  GError *error = nullptr;
  GVariant *result = g_dbus_connection_call_sync(p.connection, kBus, kPath, kScreenCast, method, parameters, G_VARIANT_TYPE("(o)"),
                                                 G_DBUS_CALL_FLAGS_NONE, -1, nullptr, &error);
  if (!result) {
    fail(std::string(method) + " failed: " + (error ? error->message : "unknown error"));
    if (error) g_error_free(error);
    g_dbus_connection_signal_unsubscribe(p.connection, subscription);
    return false;
  }
  g_variant_unref(result);

  bool timed_out = false;
  GSource *timer = g_timeout_source_new(static_cast<guint>(timeout_ms));
  g_source_set_callback(timer, on_timeout, &timed_out, nullptr);
  g_source_attach(timer, p.context);
  while (!reply.done && !timed_out) g_main_context_iteration(p.context, TRUE);
  g_source_destroy(timer);
  g_source_unref(timer);
  g_dbus_connection_signal_unsubscribe(p.connection, subscription);

  if (timed_out) return fail(std::string(method) + " timed out waiting for the portal"), false;
  if (reply.status == 1) return fail("the screen sharing request was cancelled"), false;
  if (reply.status != 0) return fail(std::string(method) + " was refused by the portal"), false;
  return true;
}

bool register_locked(Portal &p, const std::string &id) {
  if (p.registered == id) return true;
  if (!p.connect()) return false;
  GVariantBuilder options;
  g_variant_builder_init(&options, G_VARIANT_TYPE("a{sv}"));
  GError *error = nullptr;
  GVariant *result = g_dbus_connection_call_sync(p.connection, kBus, kPath, kRegistry, "Register", g_variant_new("(sa{sv})", id.c_str(), &options),
                                                 G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 5000, nullptr, &error);
  if (!result) {
    fail("could not register '" + id + "' with the portal, is it installed? " + (error ? error->message : "unknown error"));
    if (error) g_error_free(error);
    return false;
  }
  g_variant_unref(result);
  p.registered = id;
  return true;
}

}  // namespace

bool portal_start_screen(const qsys_screen_options &options, PortalSession &session) {
  Portal &p = portal();
  std::lock_guard lock(p.mutex);
  if (!p.connect()) return false;

  const std::string id = options.app_id ? options.app_id : "";
  if (!id.empty() && !register_locked(p, id)) std::fprintf(stderr, "qui/sys: %s\n", qsys_last_error());

  g_main_context_push_thread_default(p.context);
  const auto done = [&](bool value) {
    g_main_context_pop_thread_default(p.context);
    return value;
  };
  const int timeout = options.timeout_ms > 0 ? options.timeout_ms : kDefaultTimeoutMs;

  std::string handle_token = token("qui_create_");
  GVariantBuilder builder;
  g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));
  g_variant_builder_add(&builder, "{sv}", "session_handle_token", g_variant_new_string(token("qui_session_").c_str()));
  g_variant_builder_add(&builder, "{sv}", "handle_token", g_variant_new_string(handle_token.c_str()));
  Reply created;
  if (!request(p, "CreateSession", handle_token, g_variant_new("(a{sv})", &builder), timeout, created)) return done(false);
  const char *handle = nullptr;
  if (!created.results || !g_variant_lookup(created.results, "session_handle", "&s", &handle)) {
    if (created.results) g_variant_unref(created.results);
    fail("the portal did not return a session");
    return done(false);
  }
  session.handle = handle;
  g_variant_unref(created.results);
  session.subscription = g_dbus_connection_signal_subscribe(p.connection, kBus, "org.freedesktop.portal.Session", "Closed", session.handle.c_str(), nullptr,
                                                            G_DBUS_SIGNAL_FLAGS_NONE, on_session_closed, &session, nullptr);

  const std::string slot = std::string("restore-token-") + (options.slot && *options.slot ? options.slot : "screen");
  const std::string token_path = id.empty() ? "" : state_file(id, slot);
  std::string saved = token_path.empty() || options.no_persist ? "" : read_file(token_path);
  while (!saved.empty() && (saved.back() == '\n' || saved.back() == ' ')) saved.pop_back();
  session.had_permission = !saved.empty();

  handle_token = token("qui_select_");
  g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));
  g_variant_builder_add(&builder, "{sv}", "handle_token", g_variant_new_string(handle_token.c_str()));
  g_variant_builder_add(&builder, "{sv}", "types", g_variant_new_uint32((options.windows ? 2u : 0u) | 1u));
  g_variant_builder_add(&builder, "{sv}", "multiple", g_variant_new_boolean(options.multiple != 0));
  g_variant_builder_add(&builder, "{sv}", "cursor_mode", g_variant_new_uint32(options.show_cursor ? 2u : 1u));
  g_variant_builder_add(&builder, "{sv}", "persist_mode", g_variant_new_uint32(options.no_persist ? 0u : 2u));
  if (!saved.empty()) g_variant_builder_add(&builder, "{sv}", "restore_token", g_variant_new_string(saved.c_str()));
  Reply selected;
  if (!request(p, "SelectSources", handle_token, g_variant_new("(oa{sv})", session.handle.c_str(), &builder), timeout, selected)) return done(false);
  if (selected.results) g_variant_unref(selected.results);

  handle_token = token("qui_start_");
  g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));
  g_variant_builder_add(&builder, "{sv}", "handle_token", g_variant_new_string(handle_token.c_str()));
  Reply started;
  if (!request(p, "Start", handle_token, g_variant_new("(osa{sv})", session.handle.c_str(), "", &builder), timeout, started)) return done(false);
  if (!started.results) {
    fail("the portal returned no streams");
    return done(false);
  }

  const char *new_token = nullptr;
  if (!token_path.empty() && !options.no_persist && g_variant_lookup(started.results, "restore_token", "&s", &new_token))
    write_private_file(token_path, new_token);

  if (GVariant *streams = g_variant_lookup_value(started.results, "streams", G_VARIANT_TYPE("a(ua{sv})"))) {
    GVariantIter iter;
    g_variant_iter_init(&iter, streams);
    uint32_t node = 0;
    GVariant *properties = nullptr;
    while (g_variant_iter_loop(&iter, "(u@a{sv})", &node, &properties)) {
      qsys_stream_info info{node, 0, 0, 0, 0};
      g_variant_lookup(properties, "position", "(ii)", &info.x, &info.y);
      g_variant_lookup(properties, "size", "(ii)", &info.width, &info.height);
      session.streams.push_back(info);
    }
    g_variant_unref(streams);
  }
  g_variant_unref(started.results);
  if (session.streams.empty()) {
    fail("the portal returned no streams");
    return done(false);
  }

  GVariantBuilder empty;
  g_variant_builder_init(&empty, G_VARIANT_TYPE("a{sv}"));
  GError *error = nullptr;
  GUnixFDList *fds = nullptr;
  GVariant *remote = g_dbus_connection_call_with_unix_fd_list_sync(p.connection, kBus, kPath, kScreenCast, "OpenPipeWireRemote",
                                                                   g_variant_new("(oa{sv})", session.handle.c_str(), &empty), G_VARIANT_TYPE("(h)"),
                                                                   G_DBUS_CALL_FLAGS_NONE, 5000, nullptr, &fds, nullptr, &error);
  if (!remote) {
    fail(std::string("could not open the PipeWire remote: ") + (error ? error->message : "unknown error"));
    if (error) g_error_free(error);
    return done(false);
  }
  gint32 index = 0;
  g_variant_get(remote, "(h)", &index);
  session.pipewire_fd = g_unix_fd_list_get(fds, index, &error);
  g_variant_unref(remote);
  g_object_unref(fds);
  if (session.pipewire_fd < 0) {
    fail(std::string("could not read the PipeWire remote: ") + (error ? error->message : "unknown error"));
    if (error) g_error_free(error);
    return done(false);
  }
  return done(true);
}

void portal_stop(PortalSession &session) {
  Portal &p = portal();
  std::lock_guard lock(p.mutex);
  if (!p.connection) return;
  if (session.subscription) {
    g_dbus_connection_signal_unsubscribe(p.connection, session.subscription);
    session.subscription = 0;
  }
  if (!session.handle.empty() && !session.closed) {
    GVariant *result = g_dbus_connection_call_sync(p.connection, kBus, session.handle.c_str(), "org.freedesktop.portal.Session", "Close", nullptr,
                                                   G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 2000, nullptr, nullptr);
    if (result) g_variant_unref(result);
  }
  if (session.pipewire_fd >= 0) close(session.pipewire_fd);
  session.pipewire_fd = -1;
}

void portal_pump() {
  Portal &p = portal();
  std::unique_lock lock(p.mutex, std::try_to_lock);
  if (!lock || !p.context) return;
  while (g_main_context_iteration(p.context, FALSE)) {
  }
}

}  // namespace qui::sys

extern "C" int qsys_portal_register(const char *id) {
  using namespace qui::sys;
  if (!id || !*id) return fail("an app id is required");
  Portal &p = portal();
  std::lock_guard lock(p.mutex);
  return register_locked(p, id) ? 0 : -1;
}
