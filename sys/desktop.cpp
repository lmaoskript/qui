//go:build linux && !android && cgo

#include "internal.hpp"

#include <fcntl.h>
#include <spawn.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

#include <cctype>
#include <cerrno>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <vector>

extern char **environ;

namespace fs = std::filesystem;

namespace qui::sys {

namespace {

thread_local std::string g_error;

std::string env_or(const char *name, const std::string &fallback) {
  const char *value = std::getenv(name);
  return value && *value ? value : fallback;
}

std::string home() { return env_or("HOME", "/tmp"); }
std::string data_home() { return env_or("XDG_DATA_HOME", home() + "/.local/share"); }
std::string config_home() { return env_or("XDG_CONFIG_HOME", home() + "/.config"); }
std::string state_home() { return env_or("XDG_STATE_HOME", home() + "/.local/state"); }

bool valid_id(const std::string &id) {
  if (id.empty() || !std::isalnum(static_cast<unsigned char>(id[0]))) return false;
  for (const char c : id)
    if (!std::isalnum(static_cast<unsigned char>(c)) && c != '.' && c != '_' && c != '-') return false;
  return true;
}

std::string text(const char *value) { return value ? value : ""; }

std::string desktop_path(const std::string &id) { return data_home() + "/applications/" + id + ".desktop"; }

std::string unit_name(const std::string &id) {
  std::string escaped;
  for (const char c : id) {
    if (c == '-') escaped += "\\x2d";
    else escaped.push_back(c);
  }
  return "app-" + escaped + ".service";
}

std::string unit_path(const std::string &id) { return config_home() + "/systemd/user/" + unit_name(id); }

bool write_if_changed(const std::string &path, const std::string &content, mode_t mode) {
  if (read_file(path) == content && fs::exists(path)) return true;
  std::error_code code;
  fs::create_directories(fs::path(path).parent_path(), code);
  const std::string temporary = path + ".tmp";
  {
    std::ofstream file(temporary, std::ios::binary | std::ios::trunc);
    file << content;
    if (!file.good()) return false;
  }
  chmod(temporary.c_str(), mode);
  fs::rename(temporary, path, code);
  return !code;
}

int run(const std::vector<std::string> &arguments) {
  std::vector<char *> argv;
  for (const std::string &argument : arguments) argv.push_back(const_cast<char *>(argument.c_str()));
  argv.push_back(nullptr);
  posix_spawn_file_actions_t actions;
  posix_spawn_file_actions_init(&actions);
  posix_spawn_file_actions_addopen(&actions, 1, "/dev/null", O_WRONLY, 0);
  posix_spawn_file_actions_addopen(&actions, 2, "/dev/null", O_WRONLY, 0);
  pid_t pid = 0;
  const int spawned = posix_spawnp(&pid, argv[0], &actions, nullptr, argv.data(), environ);
  posix_spawn_file_actions_destroy(&actions);
  if (spawned != 0) return -1;
  int status = 0;
  while (waitpid(pid, &status, 0) < 0 && errno == EINTR) {
  }
  return WIFEXITED(status) ? WEXITSTATUS(status) : -1;
}

int systemctl(const char *verb, const std::string &unit = "") {
  std::vector<std::string> arguments{"systemctl", "--user", verb};
  if (!unit.empty()) arguments.push_back(unit);
  return run(arguments);
}

std::string quote_exec(const std::string &value) {
  if (value.find_first_of(" \t\"'\\$`") == std::string::npos) return value;
  std::string out = "\"";
  for (const char c : value) {
    if (c == '"' || c == '`' || c == '$' || c == '\\') out.push_back('\\');
    out.push_back(c);
  }
  return out + "\"";
}

std::string escape_percent(std::string value) {
  for (size_t at = value.find('%'); at != std::string::npos; at = value.find('%', at + 2)) value.insert(at, "%");
  return value;
}

std::string resolve_exec(const qsys_app &app) {
  std::string path = text(app.exec);
  if (path.empty()) {
    std::error_code code;
    path = fs::read_symlink("/proc/self/exe", code).string();
  }
  return path;
}

}  // namespace

int fail(std::string message) {
  g_error = std::move(message);
  return -1;
}

std::string read_file(const std::string &path) {
  std::ifstream file(path, std::ios::binary);
  std::stringstream out;
  out << file.rdbuf();
  return out.str();
}

std::string state_file(const std::string &id, const std::string &name) { return state_home() + "/" + id + "/" + name; }

bool write_private_file(const std::string &path, const std::string &content) {
  std::error_code code;
  const fs::path directory = fs::path(path).parent_path();
  fs::create_directories(directory, code);
  chmod(directory.c_str(), 0700);
  return write_if_changed(path, content, 0600);
}

}  // namespace qui::sys

using namespace qui::sys;

extern "C" {

const char *qsys_last_error(void) { return g_error.c_str(); }

int qsys_install(const qsys_app *app) {
  const std::string id = text(app->id);
  if (!valid_id(id)) return fail("invalid app id '" + id + "': use letters, digits, '.', '_' or '-'");

  std::string exec = resolve_exec(*app);
  if (exec.empty() || exec[0] != '/') return fail("exec must be an absolute path, because the portal resolves it against its own PATH");

  if (app->install_as && *app->install_as) {
    if (!valid_id(app->install_as)) return fail("invalid install name '" + text(app->install_as) + "'");
    const std::string target = home() + "/.local/bin/" + app->install_as;
    if (exec != target) {
      const std::string binary = read_file(exec);
      if (binary.empty() || !write_if_changed(target, binary, 0755)) return fail("could not install the executable to " + target);
    }
    exec = target;
  }

  const std::string name = app->name && *app->name ? app->name : id;
  std::string desktop = "[Desktop Entry]\nType=Application\nName=" + name + "\n";
  if (app->comment && *app->comment) desktop += "Comment=" + text(app->comment) + "\n";
  if (app->icon && *app->icon) desktop += "Icon=" + text(app->icon) + "\n";
  desktop += "Exec=" + escape_percent(quote_exec(exec));
  if (app->args && *app->args) desktop += " " + text(app->args);
  desktop += "\nTerminal=false\nStartupWMClass=" + id + "\n";
  if (app->categories && *app->categories) desktop += "Categories=" + text(app->categories) + "\n";
  if (app->hidden) desktop += "NoDisplay=true\n";
  if (!write_if_changed(desktop_path(id), desktop, 0644)) return fail("could not write " + desktop_path(id));
  run({"update-desktop-database", data_home() + "/applications"});

  if (app->service) {
    std::string after = app->service_headless ? "" : "graphical-session.target";
    if (app->service_needs_pipewire) after += (after.empty() ? "" : " ") + std::string("pipewire.service");
    std::string unit = "[Unit]\nDescription=" + name + "\n";
    if (!after.empty()) unit += "After=" + after + "\n";
    if (app->service_needs_pipewire) unit += "Wants=pipewire.service\n";
    if (!app->service_headless) unit += "PartOf=graphical-session.target\n";
    unit += "\n[Service]\nType=simple\nExecStart=" + escape_percent(quote_exec(exec));
    if (app->service_args && *app->service_args) unit += " " + escape_percent(app->service_args);
    unit += "\nRestart=" + std::string(app->service_always_restart ? "always" : "on-failure") + "\nRestartSec=5\n";
    unit += "\n[Install]\nWantedBy=" + std::string(app->service_headless ? "default.target" : "graphical-session.target") + "\n";
    const bool changed = read_file(unit_path(id)) != unit;
    if (!write_if_changed(unit_path(id), unit, 0644)) return fail("could not write " + unit_path(id));
    if (changed) {
      if (systemctl("daemon-reload") != 0) return fail("systemctl --user daemon-reload failed");
      if (systemctl("enable", unit_name(id)) != 0) return fail("could not enable " + unit_name(id));
    }
  }
  return 0;
}

int qsys_uninstall(const qsys_app *app) {
  const std::string id = text(app->id);
  if (!valid_id(id)) return fail("invalid app id '" + id + "'");
  std::error_code code;
  if (fs::exists(unit_path(id), code)) {
    run({"systemctl", "--user", "disable", "--now", unit_name(id)});
    fs::remove(unit_path(id), code);
    systemctl("daemon-reload");
  }
  fs::remove(desktop_path(id), code);
  run({"update-desktop-database", data_home() + "/applications"});
  if (app->install_as && valid_id(app->install_as)) fs::remove(home() + "/.local/bin/" + app->install_as, code);
  const fs::path state = state_home() + "/" + id;
  for (const fs::directory_entry &entry : fs::directory_iterator(state, code))
    if (entry.path().filename().string().starts_with("restore-token-")) fs::remove(entry.path(), code);
  fs::remove(state, code);
  return 0;
}

int qsys_service(const char *id, int action) {
  static const char *const verbs[] = {"start", "stop", "restart", "enable", "disable"};
  if (action < 0 || action > QSYS_SERVICE_DISABLE) return fail("unknown service action");
  const int status = systemctl(verbs[action], unit_name(id));
  return status == 0 ? 0 : fail(std::string("systemctl ") + verbs[action] + " " + unit_name(id) + " failed");
}

int qsys_service_active(const char *id) { return systemctl("is-active", unit_name(id)) == 0; }

}  // extern "C"
