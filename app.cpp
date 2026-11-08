//go:build cgo

#include "engine.hpp"
#include "qui_c.h"

#include <SDL3/SDL.h>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <string>

namespace {

using namespace qui;

constexpr float kDragThreshold = 4;
constexpr float kWheelPixels = 48;

std::string g_app_id;

std::string env(const char *name) {
  const char *value = std::getenv(name);
  return value ? value : "";
}

uint32_t crc32(const uint8_t *data, size_t len, uint32_t crc = 0xffffffffu) {
  static const auto table = [] {
    std::vector<uint32_t> t(256);
    for (uint32_t i = 0; i < 256; ++i) {
      uint32_t c = i;
      for (int k = 0; k < 8; ++k) c = (c & 1) ? 0xedb88320u ^ (c >> 1) : c >> 1;
      t[i] = c;
    }
    return t;
  }();
  for (size_t i = 0; i < len; ++i) crc = table[(crc ^ data[i]) & 0xff] ^ (crc >> 8);
  return crc;
}

void put_be32(std::vector<uint8_t> &out, uint32_t v) {
  out.push_back(static_cast<uint8_t>(v >> 24));
  out.push_back(static_cast<uint8_t>(v >> 16));
  out.push_back(static_cast<uint8_t>(v >> 8));
  out.push_back(static_cast<uint8_t>(v));
}

void chunk(std::vector<uint8_t> &out, const char *type, const std::vector<uint8_t> &data) {
  put_be32(out, static_cast<uint32_t>(data.size()));
  const size_t start = out.size();
  out.insert(out.end(), type, type + 4);
  out.insert(out.end(), data.begin(), data.end());
  put_be32(out, ~crc32(out.data() + start, out.size() - start));
}

bool write_png(const std::string &path, const uint32_t *pixels, int width, int height) {
  std::vector<uint8_t> raw;
  raw.reserve((static_cast<size_t>(width) * 4 + 1) * static_cast<size_t>(height));
  for (int y = 0; y < height; ++y) {
    raw.push_back(0);
    const auto *row = reinterpret_cast<const uint8_t *>(pixels + static_cast<size_t>(y) * width);
    raw.insert(raw.end(), row, row + static_cast<size_t>(width) * 4);
  }
  std::vector<uint8_t> z{0x78, 0x01};
  uint32_t a = 1, b = 0;
  for (const uint8_t v : raw) {
    a = (a + v) % 65521;
    b = (b + a) % 65521;
  }
  for (size_t at = 0; at < raw.size();) {
    const size_t n = std::min<size_t>(raw.size() - at, 65535);
    z.push_back(at + n == raw.size() ? 1 : 0);
    z.push_back(static_cast<uint8_t>(n));
    z.push_back(static_cast<uint8_t>(n >> 8));
    z.push_back(static_cast<uint8_t>(~n));
    z.push_back(static_cast<uint8_t>(~n >> 8));
    z.insert(z.end(), raw.begin() + static_cast<long>(at), raw.begin() + static_cast<long>(at + n));
    at += n;
  }
  put_be32(z, (b << 16) | a);

  std::vector<uint8_t> png{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a};
  std::vector<uint8_t> header;
  put_be32(header, static_cast<uint32_t>(width));
  put_be32(header, static_cast<uint32_t>(height));
  header.insert(header.end(), {8, 6, 0, 0, 0});
  chunk(png, "IHDR", header);
  chunk(png, "IDAT", z);
  chunk(png, "IEND", {});
  std::ofstream file(path, std::ios::binary);
  file.write(reinterpret_cast<const char *>(png.data()), static_cast<std::streamsize>(png.size()));
  return file.good();
}

int headless_frames() {
  const std::string value = env("QUI_FRAMES");
  return value.empty() ? 4 : std::atoi(value.c_str());
}

void *gl_proc(const char *name) { return reinterpret_cast<void *>(SDL_GL_GetProcAddress(name)); }

}  // namespace

struct qui_app {
  Engine engine;
  std::unique_ptr<Backend> backend;
  CpuBackend *cpu = nullptr;
  SDL_Window *window = nullptr;
  SDL_GLContext gl = nullptr;

  bool headless = false, sdl_ready = false, quit = false, dirty = true, force_full = true;
  int frames = 0;
  int pixel_w = 0, pixel_h = 0;
  float dpr = 1, logical_w = 0, logical_h = 0;
  std::string shot_path, tree_path, wire_path;
  std::vector<uint8_t> last_wire, results;
  std::vector<uint32_t> capture;
  std::vector<std::string> drive;
  size_t drive_at = 0;

  float pointer_x = -1, pointer_y = -1;
  float press_x = 0, press_y = 0;
  float drag_dx = 0, drag_dy = 0;
  uint64_t hovered = 0, pressed = 0, focused = 0, clicked = 0, right_clicked = 0;
  uint64_t drag_start = 0, drag_end = 0;
  bool dragging = false;

  double next_tick = 0;
  std::chrono::steady_clock::time_point start = std::chrono::steady_clock::now();
  double last_frame_time = 0;
  float dt = 1.f / 60.f;

  double now() const { return std::chrono::duration<double>(std::chrono::steady_clock::now() - start).count(); }

  void read_size() {
    if (headless && !window) return;
    int w = 0, h = 0;
    SDL_GetWindowSizeInPixels(window, &w, &h);
    pixel_w = std::max(1, w);
    pixel_h = std::max(1, h);
    if (env("QUI_DPR").empty()) dpr = std::max(0.25f, SDL_GetWindowDisplayScale(window));
    logical_w = static_cast<float>(pixel_w) / dpr;
    logical_h = static_cast<float>(pixel_h) / dpr;
  }

  bool create_gl(Uint64 flags, const char *title, int width, int height) {
    window = SDL_CreateWindow(title, width, height, flags | SDL_WINDOW_OPENGL);
    if (!window) return false;
    SDL_GL_SetAttribute(SDL_GL_DEPTH_SIZE, 0);
    SDL_GL_SetAttribute(SDL_GL_STENCIL_SIZE, 0);
    SDL_GL_SetAttribute(SDL_GL_DOUBLEBUFFER, 1);
    bool es = true;
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_ES);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 0);
    gl = SDL_GL_CreateContext(window);
    if (!gl) {
      es = false;
      SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_CORE);
      SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 3);
      gl = SDL_GL_CreateContext(window);
    }
    if (gl) {
      SDL_GL_MakeCurrent(window, gl);
      SDL_GL_SetSwapInterval(1);
      backend = make_gl_backend(gl_proc, headless, es);
    }
    if (backend) return true;
    std::fprintf(stderr, "qui: OpenGL is unavailable (%s), using the CPU renderer\n", SDL_GetError());
    if (gl) SDL_GL_DestroyContext(gl);
    gl = nullptr;
    SDL_DestroyWindow(window);
    window = nullptr;
    return false;
  }

  void use_cpu() {
    auto backend_cpu = make_cpu_backend();
    cpu = backend_cpu.get();
    backend = std::move(backend_cpu);
  }

  void open(const char *title, int width, int height) {
    shot_path = env("QUI_SHOT");
    tree_path = env("QUI_TREE");
    wire_path = env("QUI_WIRE");
    headless = !shot_path.empty() || !tree_path.empty() || !wire_path.empty();
    std::string requested = env("QUI_BACKEND");
    if (const std::string size = env("QUI_SIZE"); !size.empty()) std::sscanf(size.c_str(), "%dx%d", &width, &height);
    if (const std::string scale = env("QUI_DPR"); !scale.empty()) dpr = std::stof(scale);
    std::string steps = env("QUI_DRIVE");
    for (size_t at = 0; at < steps.size();) {
      const size_t end = std::min(steps.find(';', at), steps.size());
      if (end > at) drive.push_back(steps.substr(at, end - at));
      at = end + 1;
    }

    if (headless && requested != "gl") {
      pixel_w = static_cast<int>(std::lround(static_cast<float>(width) * dpr));
      pixel_h = static_cast<int>(std::lround(static_cast<float>(height) * dpr));
      logical_w = static_cast<float>(width);
      logical_h = static_cast<float>(height);
      use_cpu();
      return;
    }

    if (!g_app_id.empty()) SDL_SetAppMetadata(title, nullptr, g_app_id.c_str());
    if (!SDL_Init(SDL_INIT_VIDEO)) {
      std::fprintf(stderr, "qui: could not start SDL: %s\n", SDL_GetError());
      quit = true;
      use_cpu();
      return;
    }
    sdl_ready = true;
    Uint64 flags = SDL_WINDOW_RESIZABLE | SDL_WINDOW_HIGH_PIXEL_DENSITY;
    if (headless) flags |= SDL_WINDOW_HIDDEN;
    if (requested == "cpu" || !create_gl(flags, title, width, height)) {
      window = SDL_CreateWindow(title, width, height, flags);
      if (!window) {
        std::fprintf(stderr, "qui: could not create a window: %s\n", SDL_GetError());
        quit = true;
      }
      use_cpu();
    }
    if (window) read_size();
    if (window && headless) {
      if (!env("QUI_DPR").empty()) {
        pixel_w = static_cast<int>(std::lround(static_cast<float>(width) * dpr));
        pixel_h = static_cast<int>(std::lround(static_cast<float>(height) * dpr));
      } else {
        pixel_w = width;
        pixel_h = height;
        dpr = 1;
      }
      logical_w = static_cast<float>(width);
      logical_h = static_cast<float>(height);
    }
  }

  uint64_t hit_test(float x, float y, bool scroll) const {
    const auto &hits = engine.hits;
    for (auto it = hits.rbegin(); it != hits.rend(); ++it) {
      if (scroll ? !it->scroll : !it->press) continue;
      if (x >= it->x && x < it->x + it->w && y >= it->y && y < it->y + it->h && x >= it->cx0 && x < it->cx1 && y >= it->cy0 && y < it->cy1)
        return it->id;
    }
    return 0;
  }

  void update_hover() {
    const uint64_t id = hit_test(pointer_x, pointer_y, false);
    if (id != hovered) {
      hovered = id;
      dirty = true;
    }
  }

  void wheel(float amount) {
    const uint64_t id = hit_test(pointer_x, pointer_y, true);
    if (!id) return;
    if (const auto it = engine.lists.find(id); it != engine.lists.end()) {
      it->second.scroll_by(-amount * kWheelPixels);
      if (amount < 0 && it->second.bottom && it->second.anchor + 1 >= it->second.count) it->second.stuck = true;
      dirty = true;
    } else if (const auto scroll = engine.scrolls.find(id); scroll != engine.scrolls.end()) {
      const float limit = std::max(0.f, scroll->second.content - scroll->second.viewport);
      scroll->second.offset = std::clamp(scroll->second.offset - amount * kWheelPixels, 0.f, limit);
      dirty = true;
    }
  }

  void handle(const SDL_Event &e) {
    const auto to_logical = [&](float x, float y, float &out_x, float &out_y) {
      const float density = window ? SDL_GetWindowPixelDensity(window) : 1;
      out_x = x * density / dpr;
      out_y = y * density / dpr;
    };
    switch (e.type) {
      case SDL_EVENT_QUIT:
      case SDL_EVENT_WINDOW_CLOSE_REQUESTED:
        quit = true;
        break;
      case SDL_EVENT_WINDOW_RESIZED:
      case SDL_EVENT_WINDOW_PIXEL_SIZE_CHANGED:
      case SDL_EVENT_WINDOW_DISPLAY_SCALE_CHANGED:
      case SDL_EVENT_WINDOW_EXPOSED:
        dirty = true;
        force_full = true;
        break;
      case SDL_EVENT_MOUSE_MOTION: {
        float x, y;
        to_logical(e.motion.x, e.motion.y, x, y);
        const float dx = x - pointer_x, dy = y - pointer_y;
        pointer_x = x;
        pointer_y = y;
        update_hover();
        if (pressed) {
          if (!dragging && std::hypot(x - press_x, y - press_y) >= kDragThreshold) {
            dragging = true;
            drag_start = pressed;
          }
          if (dragging) {
            drag_dx += dx;
            drag_dy += dy;
            dirty = true;
          }
        }
        break;
      }
      case SDL_EVENT_WINDOW_MOUSE_LEAVE:
        pointer_x = pointer_y = -1;
        update_hover();
        break;
      case SDL_EVENT_MOUSE_BUTTON_DOWN: {
        to_logical(e.button.x, e.button.y, pointer_x, pointer_y);
        update_hover();
        if (e.button.button == SDL_BUTTON_LEFT) {
          pressed = hovered;
          focused = hovered;
          press_x = pointer_x;
          press_y = pointer_y;
          dirty = true;
        }
        break;
      }
      case SDL_EVENT_MOUSE_BUTTON_UP: {
        to_logical(e.button.x, e.button.y, pointer_x, pointer_y);
        update_hover();
        if (e.button.button == SDL_BUTTON_RIGHT && hovered) right_clicked = hovered;
        if (e.button.button == SDL_BUTTON_LEFT) {
          if (dragging) drag_end = pressed;
          else if (pressed && pressed == hovered) clicked = pressed;
          pressed = 0;
          dragging = false;
        }
        dirty = true;
        break;
      }
      case SDL_EVENT_MOUSE_WHEEL:
        wheel(e.wheel.y);
        break;
      case SDL_EVENT_USER:
        dirty = true;
        break;
      default:
        break;
    }
  }

  int total_frames() const { return std::max(headless_frames(), 6 + static_cast<int>(drive.size()) * 2); }

  void drive_step() {
    if (drive_at >= drive.size() || frames < 2 || frames % 2 != 0) return;
    const std::string &step = drive[drive_at++];
    float x = 0, y = 0, amount = 0;
    const auto motion = [&] {
      SDL_Event e{};
      e.type = SDL_EVENT_MOUSE_MOTION;
      e.motion.x = x * dpr;
      e.motion.y = y * dpr;
      handle(e);
    };
    const auto button = [&](Uint32 type, Uint8 which) {
      SDL_Event e{};
      e.type = type;
      e.button.button = which;
      e.button.x = x * dpr;
      e.button.y = y * dpr;
      handle(e);
    };
    if (std::sscanf(step.c_str(), "click:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_LEFT);
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_LEFT);
    } else if (std::sscanf(step.c_str(), "rclick:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_RIGHT);
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_RIGHT);
    } else if (std::sscanf(step.c_str(), "move:%f,%f", &x, &y) == 2) {
      motion();
    } else if (std::sscanf(step.c_str(), "wheel:%f,%f,%f", &x, &y, &amount) == 3) {
      motion();
      SDL_Event e{};
      e.type = SDL_EVENT_MOUSE_WHEEL;
      e.wheel.y = amount;
      handle(e);
    } else {
      std::fprintf(stderr, "qui: ignoring QUI_DRIVE step '%s'\n", step.c_str());
    }
  }

  bool animating() const { return !cpu && !engine.previous.shaders.empty(); }

  double tick_interval() const {
    double interval = 0;
    for (const ShaderDraw &draw : engine.previous.shaders)
      if (draw.fps > 0) interval = interval == 0 ? 1.0 / draw.fps : std::min(interval, 1.0 / draw.fps);
    return interval;
  }

  void present(IRect damage) {
    if (!window) return;
    if (gl) {
      SDL_GL_SwapWindow(window);
      return;
    }
    SDL_Surface *surface = SDL_GetWindowSurface(window);
    if (!surface) return;
    damage = intersect(damage, {0, 0, std::min(surface->w, pixel_w), std::min(surface->h, pixel_h)});
    if (damage.empty()) return;
    SDL_ConvertPixels(damage.w, damage.h, SDL_PIXELFORMAT_RGBA32,
                      cpu->pixels() + static_cast<size_t>(damage.y) * pixel_w + damage.x, pixel_w * 4,
                      surface->format,
                      static_cast<uint8_t *>(surface->pixels) + static_cast<size_t>(damage.y) * surface->pitch + static_cast<size_t>(damage.x) * 4, surface->pitch);
    const SDL_Rect rect{damage.x, damage.y, damage.w, damage.h};
    SDL_UpdateWindowSurfaceRects(window, &rect, 1);
  }

  bool wait() {
    for (;;) {
      SDL_Event event;
      while (SDL_PollEvent(&event)) handle(event);
      if (quit) return false;
      if (dirty) {
        dirty = false;
        return true;
      }
      const double interval = animating() ? tick_interval() : 0;
      bool got = false;
      if (interval > 0) {
        const auto delay = static_cast<Sint32>(std::max(0.0, (next_tick - now()) * 1000.0));
        got = SDL_WaitEventTimeout(&event, delay);
      } else {
        got = SDL_WaitEvent(&event);
      }
      if (got) handle(event);
      if (interval > 0 && now() >= next_tick) {
        backend->draw(engine.previous, {0, 0, pixel_w, pixel_h}, now());
        present({0, 0, pixel_w, pixel_h});
        next_tick = now() + interval;
      }
    }
  }

  void put_slot(std::vector<uint8_t> &table, uint32_t capacity, uint64_t id, uint32_t flags, uint32_t first, uint32_t count, const float rect[4], bool has_rect) {
    constexpr size_t kHeader = 32, kSlot = 48;
    for (uint32_t i = static_cast<uint32_t>(id) & (capacity - 1);; i = (i + 1) & (capacity - 1)) {
      uint8_t *slot = table.data() + kHeader + i * kSlot;
      uint64_t key = 0;
      std::memcpy(&key, slot, 8);
      if (key == 0 || key == id) {
        std::memcpy(slot, &id, 8);
        uint32_t existing = 0;
        std::memcpy(&existing, slot + 8, 4);
        flags |= existing;
        std::memcpy(slot + 8, &flags, 4);
        if (count) {
          std::memcpy(slot + 12, &first, 4);
          std::memcpy(slot + 16, &count, 4);
        }
        if (has_rect) std::memcpy(slot + 24, rect, 16);
        if (id == pressed && dragging) {
          std::memcpy(slot + 40, &drag_dx, 4);
          std::memcpy(slot + 44, &drag_dy, 4);
        }
        return;
      }
    }
  }

  void fill_results() {
    constexpr uint32_t kMagic = 0x31495551u;
    size_t needed = (engine.hits.size() + engine.lists.size()) * 2;
    uint32_t capacity = 16;
    while (capacity < needed) capacity *= 2;
    results.assign(32 + static_cast<size_t>(capacity) * 48, 0);
    uint8_t *p = results.data();
    const float header[] = {logical_w, logical_h, dpr, dt, pointer_x, pointer_y};
    std::memcpy(p, &kMagic, 4);
    std::memcpy(p + 4, &capacity, 4);
    std::memcpy(p + 8, header, sizeof(header));

    for (const HitEntry &hit : engine.hits) {
      uint32_t flags = 0;
      if (hit.id == hovered) flags |= wire::FlagHovered;
      if (hit.id == pressed) flags |= wire::FlagPressed;
      if (hit.id == clicked) flags |= wire::FlagClicked;
      if (hit.id == right_clicked) flags |= wire::FlagRightClicked;
      if (hit.id == focused) flags |= wire::FlagFocused;
      if (hit.id == drag_start) flags |= wire::FlagDragStart;
      if (hit.id == pressed && dragging) flags |= wire::FlagDragging;
      if (hit.id == drag_end) flags |= wire::FlagDragEnd;
      const float rect[] = {hit.x, hit.y, hit.w, hit.h};
      put_slot(results, capacity, hit.id, flags, 0, 0, rect, true);
    }
    for (auto &[id, state] : engine.lists) {
      if (state.frame != engine.frame_number) continue;
      state.compute_range();
      put_slot(results, capacity, id, 0, state.first, std::max(1u, state.visible), nullptr, false);
    }
    if (clicked || right_clicked || drag_start || drag_end) dirty = true;
    clicked = right_clicked = drag_start = drag_end = 0;
    drag_dx = drag_dy = 0;
  }

  void finish() {
    if (!shot_path.empty()) {
      backend->read_pixels(capture);
      if (write_png(shot_path, capture.data(), pixel_w, pixel_h))
        std::fprintf(stderr, "qui: wrote %s (%dx%d, %zu nodes, %zu draw instances)\n", shot_path.c_str(), pixel_w, pixel_h, engine.nodes.size() - 1, engine.frame.instances.size());
    }
    if (!tree_path.empty()) std::ofstream(tree_path) << engine.tree_json();
    if (!wire_path.empty()) std::ofstream(wire_path, std::ios::binary).write(reinterpret_cast<const char *>(last_wire.data()), static_cast<std::streamsize>(last_wire.size()));
  }

  void end_frame(const uint8_t *wire, size_t len) {
    if (!wire_path.empty()) last_wire.assign(wire, wire + len);
    if (window && !headless) read_size();
    engine.decode(wire, len);
    engine.layout(logical_w, logical_h, dpr);
    engine.paint(hovered, pressed);
    for (const AtlasUpload &upload : engine.fonts.pending_uploads()) backend->upload(upload);
    engine.fonts.clear_pending_uploads();

    backend->resize(pixel_w, pixel_h);
    IRect damage = engine.swap_and_damage();
    const IRect full{0, 0, pixel_w, pixel_h};
    if (force_full) {
      damage = full;
      force_full = false;
    }
    if (!damage.empty()) {
      const double time = headless ? 1.0 : now();
      backend->draw(engine.previous, damage, time);
      if (!headless) present(gl ? full : damage);
    }
    if (animating()) next_tick = now() + tick_interval();
    if (engine.wants_another_frame) dirty = true;
    if (!headless) update_hover();
    const double t = now();
    dt = static_cast<float>(t - last_frame_time);
    last_frame_time = t;
    ++frames;
  }

  void close() {
    backend.reset();
    if (gl) SDL_GL_DestroyContext(gl);
    if (window) SDL_DestroyWindow(window);
    if (sdl_ready) SDL_Quit();
  }
};

extern "C" {

void qui_set_app_id(const char *id) { g_app_id = id ? id : ""; }

void qui_wake(qui_app *app) {
  if (!app || app->headless) return;
  SDL_Event event{};
  event.type = SDL_EVENT_USER;
  SDL_PushEvent(&event);
}

qui_app *qui_open(const char *title, int width, int height) {
  auto *app = new qui_app();
  app->open(title, width, height);
  return app;
}

void qui_close(qui_app *app) {
  app->close();
  delete app;
}

const uint8_t *qui_frame_begin(qui_app *app, size_t *len) {
  if (app->headless) {
    if (app->frames >= app->total_frames()) {
      app->finish();
      return nullptr;
    }
    app->drive_step();
  } else if (!app->wait()) {
    return nullptr;
  }
  app->fill_results();
  *len = app->results.size();
  return app->results.data();
}

void qui_frame_end(qui_app *app, const uint8_t *wire, size_t len) { app->end_frame(wire, len); }

}  // extern "C"
