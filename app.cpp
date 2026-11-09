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
#include <limits>
#include <string>
#include <string_view>
#include <utility>

namespace {

using namespace qui;

constexpr float kDragThreshold = 4;
constexpr size_t kResultsHeader = 64, kResultsSlot = 48;
constexpr float kWheelPixels = 48;
constexpr double kCaretBlink = 0.53;

std::string g_app_id;

uint32_t key_from_sdl(SDL_Keycode key) {
  switch (key) {
    case SDLK_LEFT: return KeyLeft;
    case SDLK_RIGHT: return KeyRight;
    case SDLK_UP: return KeyUp;
    case SDLK_DOWN: return KeyDown;
    case SDLK_HOME: return KeyHome;
    case SDLK_END: return KeyEnd;
    case SDLK_PAGEUP: return KeyPageUp;
    case SDLK_PAGEDOWN: return KeyPageDown;
    case SDLK_KP_ENTER: return KeyEnter;
    default: break;
  }
  if (key >= SDLK_F1 && key <= SDLK_F12) return KeyF1 + (key - SDLK_F1);
  if (key & SDLK_SCANCODE_MASK) return 0;
  return key;
}

uint32_t mods_from_sdl(SDL_Keymod mods) {
  uint32_t out = 0;
  if (mods & SDL_KMOD_SHIFT) out |= ModShift;
  if (mods & SDL_KMOD_CTRL) out |= ModCtrl;
  if (mods & SDL_KMOD_ALT) out |= ModAlt;
  if (mods & SDL_KMOD_GUI) out |= ModMeta;
  return out;
}

uint32_t key_from_name(std::string_view name) {
  static const std::pair<std::string_view, uint32_t> names[] = {
      {"backspace", KeyBackspace}, {"tab", KeyTab}, {"enter", KeyEnter}, {"escape", KeyEscape}, {"space", KeySpace},
      {"delete", KeyDelete}, {"left", KeyLeft}, {"right", KeyRight}, {"up", KeyUp}, {"down", KeyDown},
      {"home", KeyHome}, {"end", KeyEnd}, {"pageup", KeyPageUp}, {"pagedown", KeyPageDown}};
  for (const auto &[text, code] : names)
    if (text == name) return code;
  return name.size() == 1 ? static_cast<uint32_t>(static_cast<unsigned char>(name[0])) : 0;
}

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
  uint64_t drag_start = 0, drag_end = 0, scrollbar_hovered = 0, scrollbar_dragged = 0;
  float scrollbar_grab = 0;
  bool dragging = false, focus_ring = false, selecting = false, caret_on = true, text_cursor = false, pointer_down = false, pointer_tapped = false;
  float engine_ms = 0;
  std::vector<uint32_t> keys;
  std::string clipboard, text_blob;
  SDL_Rect last_ime_area{};
  SDL_Cursor *cursor_text = nullptr;

  double next_tick = 0, blink_at = 0;
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
#if defined(_WIN32)
    bool es = false;
#else
    bool es = true;
#endif
    const auto attempt = [&](bool use_es) {
      es = use_es;
      SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, use_es ? SDL_GL_CONTEXT_PROFILE_ES : SDL_GL_CONTEXT_PROFILE_CORE);
      SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3);
      SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, use_es ? 0 : 3);
      gl = SDL_GL_CreateContext(window);
    };
    attempt(es);
    if (!gl) attempt(!es);
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
      if (!scroll && !it->press) continue;
      if (x < it->x || x >= it->x + it->w || y < it->y || y >= it->y + it->h || x < it->cx0 || x >= it->cx1 || y < it->cy0 || y >= it->cy1) continue;
      if (!scroll || it->scroll) return it->id;
      return it->scroll_parent >= 0 ? hits[static_cast<size_t>(it->scroll_parent)].id : 0;
    }
    return 0;
  }

  const HitEntry *scrollbar_at(float x, float y) const {
    const auto &hits = engine.hits;
    for (auto it = hits.rbegin(); it != hits.rend(); ++it) {
      if (!it->press && !it->scroll) continue;
      if (x < it->x || x >= it->x + it->w || y < it->y || y >= it->y + it->h || x < it->cx0 || x >= it->cx1 || y < it->cy0 || y >= it->cy1) continue;
      return it->scrollbar ? &*it : nullptr;
    }
    return nullptr;
  }

  void drag_scrollbar() {
    const auto &hits = engine.hits;
    const auto bar = std::find_if(hits.rbegin(), hits.rend(), [&](const HitEntry &e) { return e.scrollbar && e.id == scrollbar_dragged; });
    if (bar == hits.rend()) return;
    const float room = bar->h - bar->thumb_h;
    const float fraction = room > 0 ? std::clamp((pointer_y - bar->y - scrollbar_grab) / room, 0.f, 1.f) : 0;
    if (const auto input = engine.inputs.find(bar->id); input != engine.inputs.end()) {
      input->second.scroll_y = fraction * input->second.scroll_limit_y;
    } else if (const auto list = engine.lists.find(bar->id); list != engine.lists.end()) {
      list->second.jump(fraction);
    } else if (const auto scroll = engine.scrolls.find(bar->id); scroll != engine.scrolls.end()) {
      scroll->second.offset = fraction * std::max(0.f, scroll->second.content - scroll->second.viewport);
    }
    dirty = true;
  }

  void update_hover() {
    const HitEntry *bar = scrollbar_at(pointer_x, pointer_y);
    if (const uint64_t bar_id = bar ? bar->id : 0; bar_id != scrollbar_hovered) {
      scrollbar_hovered = bar_id;
      dirty = true;
    }
    const uint64_t id = bar ? 0 : hit_test(pointer_x, pointer_y, false);
    if (id != hovered) {
      hovered = id;
      dirty = true;
      const bool over_text = input_of(id) != nullptr;
      if (over_text != text_cursor && sdl_ready && window) {
        text_cursor = over_text;
        if (over_text && !cursor_text) cursor_text = SDL_CreateSystemCursor(SDL_SYSTEM_CURSOR_TEXT);
        SDL_SetCursor(over_text ? cursor_text : SDL_GetDefaultCursor());
      }
    }
  }

  InputState *input_of(uint64_t id) {
    if (!id) return nullptr;
    const auto it = engine.inputs.find(id);
    return it != engine.inputs.end() && it->second.frame == engine.frame_number ? &it->second : nullptr;
  }

  InputState *focused_input() { return input_of(focused); }

  void restart_blink() {
    caret_on = true;
    blink_at = now() + kCaretBlink;
  }

  void set_focus(uint64_t id, bool keyboard) {
    if (id == focused && keyboard == focus_ring) return;
    const bool changed = id != focused;
    focused = id;
    focus_ring = keyboard;
    dirty = true;
    restart_blink();
    if (!changed || !sdl_ready || !window) return;
    if (focused_input()) SDL_StartTextInput(window);
    else SDL_StopTextInput(window);
  }

  void focus_step(bool backwards) {
    std::vector<uint64_t> order;
    for (const HitEntry &hit : engine.hits)
      if (hit.focusable) order.push_back(hit.id);
    if (order.empty()) return;
    const auto at = std::find(order.begin(), order.end(), focused);
    size_t index = 0;
    if (at == order.end()) index = backwards ? order.size() - 1 : 0;
    else if (backwards) index = (static_cast<size_t>(at - order.begin()) + order.size() - 1) % order.size();
    else index = (static_cast<size_t>(at - order.begin()) + 1) % order.size();
    set_focus(order[index], true);
    if (InputState *state = focused_input(); state && !state->multiline) {
      state->anchor = 0;
      state->caret = state->text.size();
      engine.refresh_input(*state);
    }
  }

  void set_clipboard(std::string_view text) {
    clipboard.assign(text);
    if (sdl_ready) SDL_SetClipboardText(clipboard.c_str());
  }

  std::string read_clipboard() {
    if (!sdl_ready) return clipboard;
    char *text = SDL_GetClipboardText();
    std::string out = text ? text : "";
    SDL_free(text);
    return out;
  }

  void input_touched(InputState &state) {
    engine.refresh_input(state);
    restart_blink();
    dirty = true;
  }

  void move_caret(InputState &state, size_t to, bool extend) {
    state.caret = to;
    if (!extend) state.anchor = to;
    state.last_edit = EditKind::None;
    input_touched(state);
  }

  void insert_text(InputState &state, std::string_view raw, EditKind kind) {
    const std::string text = clean_input(raw, state.multiline);
    if (text.empty() && !state.has_selection()) return;
    state.replace_range(state.selection_start(), state.selection_end(), text, kind);
    input_touched(state);
  }

  void erase_to(InputState &state, size_t to) {
    if (state.has_selection()) {
      state.replace_range(state.selection_start(), state.selection_end(), "", EditKind::Other);
    } else if (to != state.caret) {
      state.replace_range(std::min(to, state.caret), std::max(to, state.caret), "", EditKind::Erase);
    }
    input_touched(state);
  }

  bool input_key(InputState &state, uint32_t key, uint32_t mods) {
    if (!state.preedit.empty()) return true;
    if (mods & (ModAlt | ModMeta)) return false;
    const bool ctrl = mods & ModCtrl, shift = mods & ModShift;
    const std::string_view text = state.text;
    switch (key) {
      case KeyLeft:
        if (state.has_selection() && !shift) move_caret(state, state.selection_start(), false);
        else {
          state.goal_x = -1;
          move_caret(state, ctrl ? previous_word(text, state.caret) : previous_boundary(text, state.caret), shift);
        }
        return true;
      case KeyRight:
        if (state.has_selection() && !shift) move_caret(state, state.selection_end(), false);
        else {
          state.goal_x = -1;
          move_caret(state, ctrl ? next_word(text, state.caret) : next_boundary(text, state.caret), shift);
        }
        return true;
      case KeyUp:
      case KeyDown:
        if (!state.multiline) return false;
        move_caret(state, engine.input_vertical(state, key == KeyUp ? -1 : 1), shift);
        return true;
      case KeyHome:
        state.goal_x = -1;
        move_caret(state, ctrl ? 0 : engine.input_line_edge(state, false), shift);
        return true;
      case KeyEnd:
        state.goal_x = -1;
        move_caret(state, ctrl ? state.text.size() : engine.input_line_edge(state, true), shift);
        return true;
      case KeyBackspace:
        erase_to(state, ctrl ? previous_word(text, state.caret) : previous_boundary(text, state.caret));
        return true;
      case KeyDelete:
        erase_to(state, ctrl ? next_word(text, state.caret) : next_boundary(text, state.caret));
        return true;
      case KeyEnter: {
        const bool submit = !state.multiline || (state.submit_on_enter ? !shift : ctrl);
        if (submit) {
          state.submitted = true;
          dirty = true;
        } else {
          insert_text(state, "\n", EditKind::Other);
        }
        return true;
      }
      case KeyEscape:
        return false;
      default:
        break;
    }
    if (ctrl) {
      switch (key) {
        case 'a':
          state.anchor = 0;
          state.caret = state.text.size();
          state.goal_x = -1;
          input_touched(state);
          return true;
        case 'c':
          if (state.has_selection() && !state.password) set_clipboard(state.selected());
          return true;
        case 'x':
          if (state.has_selection() && !state.password) {
            set_clipboard(state.selected());
            erase_to(state, state.caret);
          }
          return true;
        case 'v':
          insert_text(state, read_clipboard(), EditKind::Other);
          return true;
        case 'z':
          if (shift) state.restore(state.redo, state.undo);
          else state.restore(state.undo, state.redo);
          input_touched(state);
          return true;
        case 'y':
          state.restore(state.redo, state.undo);
          input_touched(state);
          return true;
        default:
          return false;
      }
    }
    return key < KeyLeft;
  }

  void on_key(uint32_t key, uint32_t mods) {
    if (!key) return;
    if (key == KeyTab && !(mods & (ModCtrl | ModAlt | ModMeta))) {
      focus_step(mods & ModShift);
      return;
    }
    if (InputState *state = focused_input()) {
      if (input_key(*state, key, mods)) return;
    } else if (focused && !mods && (key == KeyEnter || key == KeySpace)) {
      const auto hit = std::find_if(engine.hits.begin(), engine.hits.end(), [&](const HitEntry &e) { return e.id == focused && e.focusable; });
      if (hit != engine.hits.end()) {
        clicked = focused;
        dirty = true;
        return;
      }
    }
    keys.push_back(key);
    keys.push_back(mods);
    dirty = true;
  }

  void set_preedit(std::string_view text, size_t cursor) {
    InputState *state = focused_input();
    if (!state || state->password) return;
    state->preedit.assign(text);
    state->preedit_cursor = std::min(cursor, state->preedit.size());
    input_touched(*state);
  }

  void pointer_select(InputState &state, int clicks, bool extend) {
    const size_t byte = engine.input_byte_at(state, pointer_x, pointer_y);
    if (clicks == 2) {
      word_range(state.text, byte, state.anchor, state.caret);
    } else if (clicks >= 3) {
      state.anchor = 0;
      state.caret = state.text.size();
    } else {
      state.caret = byte;
      if (!extend) state.anchor = byte;
      selecting = true;
    }
    state.goal_x = -1;
    state.last_edit = EditKind::None;
    input_touched(state);
  }

  void wheel(float amount) {
    const uint64_t id = hit_test(pointer_x, pointer_y, true);
    if (!id) return;
    if (const auto input = engine.inputs.find(id); input != engine.inputs.end()) {
      input->second.scroll_y = std::max(0.f, input->second.scroll_y - amount * kWheelPixels * dpr);
      dirty = true;
    } else if (const auto it = engine.lists.find(id); it != engine.lists.end()) {
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
        if (scrollbar_dragged) {
          drag_scrollbar();
          break;
        }
        update_hover();
        if (selecting) {
          if (InputState *state = focused_input()) {
            state->caret = engine.input_byte_at(*state, x, y);
            input_touched(*state);
          }
        }
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
          pointer_down = true;
          pointer_tapped = true;
          if (const HitEntry *bar = scrollbar_at(pointer_x, pointer_y)) {
            scrollbar_dragged = bar->id;
            const bool on_thumb = pointer_y >= bar->thumb_y && pointer_y < bar->thumb_y + bar->thumb_h;
            scrollbar_grab = on_thumb ? pointer_y - bar->thumb_y : bar->thumb_h / 2;
            if (!on_thumb) drag_scrollbar();
            dirty = true;
            break;
          }
          pressed = hovered;
          set_focus(hovered, false);
          press_x = pointer_x;
          press_y = pointer_y;
          dirty = true;
          if (InputState *state = input_of(hovered)) pointer_select(*state, e.button.clicks, SDL_GetModState() & SDL_KMOD_SHIFT);
        } else if (e.button.button == SDL_BUTTON_MIDDLE && sdl_ready) {
          if (InputState *state = input_of(hovered)) {
            set_focus(hovered, false);
            state->caret = state->anchor = engine.input_byte_at(*state, pointer_x, pointer_y);
            char *text = SDL_GetPrimarySelectionText();
            if (text) insert_text(*state, text, EditKind::Other);
            SDL_free(text);
          }
        }
        break;
      }
      case SDL_EVENT_MOUSE_BUTTON_UP: {
        to_logical(e.button.x, e.button.y, pointer_x, pointer_y);
        update_hover();
        if (e.button.button == SDL_BUTTON_RIGHT && hovered) right_clicked = hovered;
        if (e.button.button == SDL_BUTTON_LEFT) {
          pointer_down = false;
          scrollbar_dragged = 0;
          if (dragging) drag_end = pressed;
          else if (pressed && pressed == hovered) clicked = pressed;
          pressed = 0;
          dragging = false;
          if (selecting && sdl_ready) {
            InputState *state = focused_input();
            if (state && state->has_selection() && !state->password) SDL_SetPrimarySelectionText(std::string(state->selected()).c_str());
          }
          selecting = false;
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
      case SDL_EVENT_KEY_DOWN:
        on_key(key_from_sdl(e.key.key), mods_from_sdl(e.key.mod));
        break;
      case SDL_EVENT_TEXT_INPUT:
        if (InputState *state = focused_input()) {
          state->preedit.clear();
          insert_text(*state, e.text.text, EditKind::Type);
        }
        break;
      case SDL_EVENT_TEXT_EDITING:
        set_preedit(e.edit.text ? e.edit.text : "", e.edit.start > 0 ? static_cast<size_t>(e.edit.start) : 0);
        break;
      case SDL_EVENT_WINDOW_FOCUS_LOST:
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
    float x = 0, y = 0, amount = 0, x2 = 0, y2 = 0;
    if (step.rfind("type:", 0) == 0) {
      SDL_Event e{};
      e.type = SDL_EVENT_TEXT_INPUT;
      e.text.text = step.c_str() + 5;
      handle(e);
      return;
    }
    if (step.rfind("ime:", 0) == 0) {
      set_preedit(step.c_str() + 4, step.size() - 4);
      return;
    }
    if (step.rfind("key:", 0) == 0) {
      std::string_view rest = std::string_view(step).substr(4);
      uint32_t mods = 0;
      for (size_t plus = rest.find('+'); plus != std::string_view::npos && rest.size() > 1; plus = rest.find('+')) {
        const std::string_view modifier = rest.substr(0, plus);
        if (modifier == "shift") mods |= ModShift;
        else if (modifier == "ctrl") mods |= ModCtrl;
        else if (modifier == "alt") mods |= ModAlt;
        else if (modifier == "meta") mods |= ModMeta;
        rest.remove_prefix(plus + 1);
      }
      on_key(key_from_name(rest), mods);
      return;
    }
    const auto motion = [&] {
      SDL_Event e{};
      e.type = SDL_EVENT_MOUSE_MOTION;
      e.motion.x = x * dpr;
      e.motion.y = y * dpr;
      handle(e);
    };
    const auto button = [&](Uint32 type, Uint8 which, Uint8 clicks = 1) {
      SDL_Event e{};
      e.type = type;
      e.button.button = which;
      e.button.clicks = clicks;
      e.button.x = x * dpr;
      e.button.y = y * dpr;
      handle(e);
    };
    if (std::sscanf(step.c_str(), "drag:%f,%f,%f,%f", &x, &y, &x2, &y2) == 4) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_LEFT);
      x = x2;
      y = y2;
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_LEFT);
    } else if (std::sscanf(step.c_str(), "dblclick:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_LEFT, 2);
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_LEFT, 2);
    } else if (std::sscanf(step.c_str(), "click:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_LEFT);
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_LEFT);
    } else if (std::sscanf(step.c_str(), "rclick:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_RIGHT);
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_RIGHT);
    } else if (std::sscanf(step.c_str(), "down:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_DOWN, SDL_BUTTON_LEFT);
    } else if (std::sscanf(step.c_str(), "up:%f,%f", &x, &y) == 2) {
      motion();
      button(SDL_EVENT_MOUSE_BUTTON_UP, SDL_BUTTON_LEFT);
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
      const bool blinking = focused_input() != nullptr;
      double deadline = std::numeric_limits<double>::infinity();
      if (interval > 0) deadline = next_tick;
      if (blinking) deadline = std::min(deadline, blink_at);
      bool got = false;
      if (std::isfinite(deadline)) {
        got = SDL_WaitEventTimeout(&event, static_cast<Sint32>(std::ceil(std::max(0.0, (deadline - now()) * 1000.0))));
      } else {
        got = SDL_WaitEvent(&event);
      }
      if (got) handle(event);
      if (blinking && now() >= blink_at) {
        caret_on = !caret_on;
        blink_at = now() + kCaretBlink;
        dirty = true;
      }
      if (interval > 0 && now() >= next_tick) {
        backend->draw(engine.previous, {0, 0, pixel_w, pixel_h}, now());
        present({0, 0, pixel_w, pixel_h});
        next_tick = now() + interval;
      }
    }
  }

  void put_slot(std::vector<uint8_t> &table, uint32_t capacity, uint64_t id, uint32_t flags, uint32_t first, uint32_t count, const float rect[4], bool has_rect) {
    for (uint32_t i = static_cast<uint32_t>(id) & (capacity - 1);; i = (i + 1) & (capacity - 1)) {
      uint8_t *slot = table.data() + kResultsHeader + i * kResultsSlot;
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
        if ((id == pressed && dragging) || id == drag_end) {
          std::memcpy(slot + 40, &drag_dx, 4);
          std::memcpy(slot + 44, &drag_dy, 4);
        }
        return;
      }
    }
  }

  void fill_results() {
    constexpr uint32_t kMagic = 0x31495551u;
    size_t needed = (engine.hits.size() + engine.lists.size() + engine.previous.shaders.size()) * 2;
    uint32_t capacity = 16;
    while (capacity < needed) capacity *= 2;

    text_blob.clear();
    bool edits = false;
    for (auto &[id, state] : engine.inputs) {
      if (state.changed) edits = true;
      if (state.submitted) edits = true;
    }

    const size_t keys_at = kResultsHeader + static_cast<size_t>(capacity) * kResultsSlot;
    const size_t text_at = keys_at + keys.size() * 4;
    for (const auto &[id, state] : engine.inputs)
      if (state.changed) text_blob += state.text;
    for (const ShaderDraw &draw : engine.previous.shaders) text_blob += backend->shader_error(draw.hash);
    results.assign(text_at + text_blob.size(), 0);
    uint8_t *p = results.data();
    const float header[] = {logical_w, logical_h, dpr, dt, pointer_x, pointer_y};
    const uint32_t offsets[] = {static_cast<uint32_t>(keys_at), static_cast<uint32_t>(keys.size() / 2), static_cast<uint32_t>(text_at), pointer_down || pointer_tapped ? 1u : 0u};
    pointer_tapped = false;
    std::memcpy(p, &kMagic, 4);
    std::memcpy(p + 4, &capacity, 4);
    std::memcpy(p + 8, header, sizeof(header));
    std::memcpy(p + 32, offsets, sizeof(offsets));
    std::memcpy(p + 48, &engine_ms, 4);
    if (!keys.empty()) std::memcpy(p + keys_at, keys.data(), keys.size() * 4);
    if (!text_blob.empty()) std::memcpy(p + text_at, text_blob.data(), text_blob.size());

    for (const HitEntry &hit : engine.hits) {
      if (hit.scrollbar) continue;
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
    uint32_t blob_at = 0;
    for (auto &[id, state] : engine.inputs) {
      if (!state.changed && !state.submitted) continue;
      uint32_t flags = 0;
      if (state.changed) flags |= wire::FlagChanged;
      if (state.submitted) flags |= wire::FlagSubmitted;
      const uint32_t length = state.changed ? static_cast<uint32_t>(state.text.size()) : 0;
      put_slot(results, capacity, id, flags, blob_at, length, nullptr, false);
      if (state.changed) {
        blob_at += length;
        state.seen = state.text;
      }
      state.changed = state.submitted = false;
    }
    for (const ShaderDraw &draw : engine.previous.shaders) {
      const std::string_view error = backend->shader_error(draw.hash);
      if (error.empty()) continue;
      put_slot(results, capacity, draw.id, wire::FlagShaderError, blob_at, static_cast<uint32_t>(error.size()), nullptr, false);
      blob_at += static_cast<uint32_t>(error.size());
    }
    if (clicked || right_clicked || drag_start || drag_end || edits || !keys.empty()) dirty = true;
    clicked = right_clicked = drag_start = drag_end = 0;
    drag_dx = drag_dy = 0;
    keys.clear();
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
    const double began = now();
    if (!wire_path.empty()) last_wire.assign(wire, wire + len);
    if (window && !headless) read_size();
    engine.decode(wire, len);
    engine.layout(logical_w, logical_h, dpr);
    if (engine.focus_request) {
      set_focus(engine.focus_request, false);
      engine.focus_request = 0;
    }
    engine.ui = {hovered, pressed, focused, focus_ring, headless || caret_on, scrollbar_dragged ? scrollbar_dragged : scrollbar_hovered};
    engine.paint();
    if (focused && std::none_of(engine.hits.begin(), engine.hits.end(), [&](const HitEntry &e) { return e.id == focused; })) set_focus(0, false);
    update_ime_area();
    for (const AtlasUpload &upload : engine.fonts.pending_uploads()) backend->upload(upload);
    engine.fonts.clear_pending_uploads();

    backend->resize(pixel_w, pixel_h);
    IRect damage = engine.swap_and_damage();
    const IRect full{0, 0, pixel_w, pixel_h};
    if (force_full) {
      damage = full;
      force_full = false;
    }
    if (!damage.empty()) backend->draw(engine.previous, damage, headless ? 1.0 : now());
    if (backend->take_compiled()) dirty = true;
    engine_ms = static_cast<float>((now() - began) * 1000);
    if (!damage.empty() && !headless) present(gl ? full : damage);
    if (animating()) next_tick = now() + tick_interval();
    if (engine.wants_another_frame) dirty = true;
    if (!headless && !scrollbar_dragged) update_hover();
    const double t = now();
    dt = static_cast<float>(t - last_frame_time);
    last_frame_time = t;
    ++frames;
  }

  void update_ime_area() {
    InputState *state = focused_input();
    if (!state || !sdl_ready || !window) return;
    float x = 0, y = 0, h = 0;
    engine.input_caret_rect(*state, x, y, h);
    const float density = SDL_GetWindowPixelDensity(window);
    const float scale = density > 0 ? dpr / density : 1;
    const SDL_Rect area{static_cast<int>(x * scale), static_cast<int>(y * scale), 1, static_cast<int>(std::ceil(h * scale))};
    if (area.x == last_ime_area.x && area.y == last_ime_area.y && area.h == last_ime_area.h) return;
    last_ime_area = area;
    SDL_SetTextInputArea(window, &area, 0);
  }

  void close() {
    if (cursor_text) SDL_DestroyCursor(cursor_text);
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
