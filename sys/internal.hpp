#pragma once

#include "qui_sys.h"

#include <atomic>
#include <cstdint>
#include <string>
#include <vector>

namespace qui::sys {

int fail(std::string message);
std::string state_file(const std::string &id, const std::string &name);
bool write_private_file(const std::string &path, const std::string &content);
std::string read_file(const std::string &path);

struct PortalSession {
  std::string handle;
  int pipewire_fd = -1;
  std::vector<qsys_stream_info> streams;
  std::atomic<bool> closed{false};
  unsigned subscription = 0;
  bool had_permission = false;
};

bool portal_start_screen(const qsys_screen_options &options, PortalSession &session);
void portal_stop(PortalSession &session);
void portal_pump();

}  // namespace qui::sys
