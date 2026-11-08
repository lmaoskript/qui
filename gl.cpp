//go:build cgo

#include "engine.hpp"

#define GL_GLES_PROTOTYPES 0
#include <GLES3/gl3.h>

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <string>
#include <utility>
#include <unordered_map>

namespace qui {

namespace {

#define QUI_GL_FUNCTIONS(X)                                       \
  X(ActiveTexture, ACTIVETEXTURE)                                 \
  X(AttachShader, ATTACHSHADER)                                   \
  X(BindBuffer, BINDBUFFER)                                       \
  X(BindFramebuffer, BINDFRAMEBUFFER)                             \
  X(BindRenderbuffer, BINDRENDERBUFFER)                           \
  X(BindTexture, BINDTEXTURE)                                     \
  X(BindVertexArray, BINDVERTEXARRAY)                             \
  X(BlendFunc, BLENDFUNC)                                         \
  X(BufferData, BUFFERDATA)                                       \
  X(Clear, CLEAR)                                                 \
  X(ClearColor, CLEARCOLOR)                                       \
  X(CompileShader, COMPILESHADER)                                 \
  X(CreateProgram, CREATEPROGRAM)                                 \
  X(CreateShader, CREATESHADER)                                   \
  X(DeleteProgram, DELETEPROGRAM)                                 \
  X(DeleteShader, DELETESHADER)                                   \
  X(Disable, DISABLE)                                             \
  X(DrawArrays, DRAWARRAYS)                                       \
  X(DrawArraysInstanced, DRAWARRAYSINSTANCED)                     \
  X(Enable, ENABLE)                                               \
  X(EnableVertexAttribArray, ENABLEVERTEXATTRIBARRAY)             \
  X(FramebufferRenderbuffer, FRAMEBUFFERRENDERBUFFER)             \
  X(GenBuffers, GENBUFFERS)                                       \
  X(GenFramebuffers, GENFRAMEBUFFERS)                             \
  X(GenRenderbuffers, GENRENDERBUFFERS)                           \
  X(GenTextures, GENTEXTURES)                                     \
  X(GenVertexArrays, GENVERTEXARRAYS)                             \
  X(GetActiveUniform, GETACTIVEUNIFORM)                           \
  X(GetProgramInfoLog, GETPROGRAMINFOLOG)                         \
  X(GetProgramiv, GETPROGRAMIV)                                   \
  X(GetShaderInfoLog, GETSHADERINFOLOG)                           \
  X(GetShaderiv, GETSHADERIV)                                     \
  X(GetUniformLocation, GETUNIFORMLOCATION)                       \
  X(LinkProgram, LINKPROGRAM)                                     \
  X(PixelStorei, PIXELSTOREI)                                     \
  X(ReadPixels, READPIXELS)                                       \
  X(RenderbufferStorage, RENDERBUFFERSTORAGE)                     \
  X(Scissor, SCISSOR)                                             \
  X(ShaderSource, SHADERSOURCE)                                   \
  X(TexImage2D, TEXIMAGE2D)                                       \
  X(TexParameteri, TEXPARAMETERI)                                 \
  X(TexSubImage2D, TEXSUBIMAGE2D)                                 \
  X(Uniform1f, UNIFORM1F)                                         \
  X(Uniform1i, UNIFORM1I)                                         \
  X(Uniform2f, UNIFORM2F)                                         \
  X(Uniform3f, UNIFORM3F)                                         \
  X(Uniform4f, UNIFORM4F)                                         \
  X(UseProgram, USEPROGRAM)                                       \
  X(VertexAttribDivisor, VERTEXATTRIBDIVISOR)                     \
  X(VertexAttribIPointer, VERTEXATTRIBIPOINTER)                   \
  X(VertexAttribPointer, VERTEXATTRIBPOINTER)                     \
  X(Viewport, VIEWPORT)

struct Gl {
#define X(name, upper) PFNGL##upper##PROC name = nullptr;
  QUI_GL_FUNCTIONS(X)
#undef X

  bool load(void *(*get)(const char *)) {
    bool ok = true;
#define X(fn, upper)                                     \
  fn = reinterpret_cast<PFNGL##upper##PROC>(get("gl" #fn)); \
  if (!fn) {                                             \
    std::fprintf(stderr, "qui: missing GL function gl" #fn "\n"); \
    ok = false;                                          \
  }
    QUI_GL_FUNCTIONS(X)
#undef X
    return ok;
  }
};

const char *const kUiVertex = R"(
layout(location = 0) in vec4 a_rect;
layout(location = 1) in vec4 a_uv;
layout(location = 2) in vec4 a_color;
layout(location = 3) in uint a_flags;
uniform vec2 u_fb;
flat out vec4 v_color;
flat out uint v_flags;
flat out vec2 v_size;
flat out vec2 v_params;
out vec2 v_local;
void main() {
  vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
  vec2 pos = a_rect.xy + corner * a_rect.zw;
  vec2 ndc = pos / u_fb * 2.0 - 1.0;
  gl_Position = vec4(ndc.x, -ndc.y, 0.0, 1.0);
  v_color = a_color;
  v_flags = a_flags;
  v_size = a_rect.zw;
  v_params = a_uv.xy;
  v_local = corner * a_rect.zw;
}
)";

const char *const kUiFragment = R"(
precision highp float;
precision highp int;
uniform sampler2D u_atlas;
flat in vec4 v_color;
flat in uint v_flags;
flat in vec2 v_size;
flat in vec2 v_params;
in vec2 v_local;
out vec4 out_color;
void main() {
  const uint solid = 1u;
  const uint color_glyph = 4u;
  const uint rounded = 8u;
  if ((v_flags & solid) != 0u) {
    float cover = 1.0;
    if ((v_flags & rounded) != 0u) {
      vec2 half_size = v_size * 0.5;
      float r = min(v_params.x, min(half_size.x, half_size.y));
      vec2 q = abs(v_local - half_size) - half_size + vec2(r);
      float dist = length(max(q, vec2(0.0))) + min(max(q.x, q.y), 0.0) - r;
      float stroke = v_params.y;
      cover = stroke > 0.0 ? smoothstep(0.5, -0.5, abs(dist + stroke * 0.5) - stroke * 0.5)
                           : smoothstep(0.5, -0.5, dist);
      if (cover <= 0.0) discard;
    }
    float a = v_color.a * cover;
    out_color = vec4(v_color.rgb * a, a);
    return;
  }
  ivec2 glyph_size = max(ivec2(round(v_size)), ivec2(1));
  ivec2 local = clamp(ivec2(floor(v_local)), ivec2(0), glyph_size - ivec2(1));
  vec4 texel = texelFetch(u_atlas, ivec2(v_params) + local, 0);
  if ((v_flags & color_glyph) != 0u) {
    out_color = texel * v_color.a;
    return;
  }
  float a = v_color.a * texel.a;
  out_color = vec4(v_color.rgb * a, a);
}
)";

const char *const kShaderVertex = R"(
uniform vec4 qui_rect;
uniform vec2 qui_fb;
out vec2 v_uv;
void main() {
  vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
  v_uv = corner;
  vec2 ndc = (qui_rect.xy + corner * qui_rect.zw) / qui_fb * 2.0 - 1.0;
  gl_Position = vec4(ndc.x, -ndc.y, 0.0, 1.0);
}
)";

const char *const kShaderHeader = R"(
precision highp float;
precision highp int;
uniform float u_time;
uniform vec2 u_resolution;
in vec2 v_uv;
out vec4 qui_out;
)";

const char *const kShaderFooter = R"(
void main() { qui_out = shade(v_uv); }
)";

constexpr size_t kMaxPrograms = 32;

class GlBackend final : public Backend {
 public:
  GlBackend(const Gl &gl, bool offscreen, bool es) : gl_(gl), offscreen_(offscreen), version_(es ? "#version 300 es\n" : "#version 330 core\n") {
    program_ = link(kUiVertex, kUiFragment, "ui");
    u_fb_ = gl_.GetUniformLocation(program_, "u_fb");
    u_atlas_ = gl_.GetUniformLocation(program_, "u_atlas");
    gl_.GenVertexArrays(1, &vao_);
    gl_.GenBuffers(1, &vbo_);
    gl_.BindVertexArray(vao_);
    gl_.BindBuffer(GL_ARRAY_BUFFER, vbo_);
    for (GLuint i = 0; i < 4; ++i) {
      gl_.EnableVertexAttribArray(i);
      gl_.VertexAttribDivisor(i, 1);
    }
    gl_.PixelStorei(GL_UNPACK_ALIGNMENT, 1);
  }

  void resize(int width, int height) override {
    if (width == width_ && height == height_) return;
    width_ = width;
    height_ = height;
    if (!offscreen_) return;
    if (!fbo_) {
      gl_.GenFramebuffers(1, &fbo_);
      gl_.GenRenderbuffers(1, &color_);
    }
    gl_.BindRenderbuffer(GL_RENDERBUFFER, color_);
    gl_.RenderbufferStorage(GL_RENDERBUFFER, GL_RGBA8, width, height);
    gl_.BindFramebuffer(GL_FRAMEBUFFER, fbo_);
    gl_.FramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, color_);
  }

  void upload(const AtlasUpload &upload) override {
    if (upload.atlas_page >= pages_.size()) pages_.resize(upload.atlas_page + 1, 0);
    GLuint &texture = pages_[upload.atlas_page];
    gl_.ActiveTexture(GL_TEXTURE0);
    if (!texture) {
      gl_.GenTextures(1, &texture);
      gl_.BindTexture(GL_TEXTURE_2D, texture);
      gl_.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
      gl_.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
      gl_.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
      gl_.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
      gl_.TexImage2D(GL_TEXTURE_2D, 0, GL_RGBA8, static_cast<GLsizei>(upload.page_width), static_cast<GLsizei>(upload.page_height), 0, GL_RGBA, GL_UNSIGNED_BYTE, nullptr);
    }
    gl_.BindTexture(GL_TEXTURE_2D, texture);
    gl_.TexSubImage2D(GL_TEXTURE_2D, 0, static_cast<GLint>(upload.destination_x), static_cast<GLint>(upload.destination_y), static_cast<GLsizei>(upload.width), static_cast<GLsizei>(upload.height), GL_RGBA, GL_UNSIGNED_BYTE, upload.pixels.data());
  }

  void draw(const FrameData &frame, IRect, double time) override {
    gl_.BindFramebuffer(GL_FRAMEBUFFER, offscreen_ ? fbo_ : 0);
    gl_.Viewport(0, 0, width_, height_);
    gl_.Disable(GL_SCISSOR_TEST);
    const float r = static_cast<float>((frame.clear >> 24) & 0xff) / 255.f;
    const float g = static_cast<float>((frame.clear >> 16) & 0xff) / 255.f;
    const float b = static_cast<float>((frame.clear >> 8) & 0xff) / 255.f;
    gl_.ClearColor(r, g, b, 1);
    gl_.Clear(GL_COLOR_BUFFER_BIT);
    gl_.Enable(GL_BLEND);
    gl_.BlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA);
    gl_.Enable(GL_SCISSOR_TEST);

    gl_.BindVertexArray(vao_);
    gl_.BindBuffer(GL_ARRAY_BUFFER, vbo_);
    gl_.BufferData(GL_ARRAY_BUFFER, static_cast<GLsizeiptr>(frame.instances.size() * sizeof(Instance)), frame.instances.data(), GL_STREAM_DRAW);
    bind_ui();

    for (const Batch &batch : frame.batches) {
      gl_.Scissor(batch.clip.x, height_ - batch.clip.y - batch.clip.h, batch.clip.w, batch.clip.h);
      if (batch.shader >= 0) {
        draw_shader(frame.shaders[static_cast<size_t>(batch.shader)], time);
        bind_ui();
        continue;
      }
      if (batch.count == 0) continue;
      if (batch.page >= 0 && static_cast<size_t>(batch.page) < pages_.size()) gl_.BindTexture(GL_TEXTURE_2D, pages_[static_cast<size_t>(batch.page)]);
      const auto base = static_cast<uintptr_t>(batch.first) * sizeof(Instance);
      const auto at = [&](uintptr_t offset) { return reinterpret_cast<const void *>(base + offset); };
      gl_.VertexAttribPointer(0, 4, GL_FLOAT, GL_FALSE, sizeof(Instance), at(offsetof(Instance, x)));
      gl_.VertexAttribPointer(1, 4, GL_FLOAT, GL_FALSE, sizeof(Instance), at(offsetof(Instance, u0)));
      gl_.VertexAttribPointer(2, 4, GL_UNSIGNED_BYTE, GL_TRUE, sizeof(Instance), at(offsetof(Instance, rgba)));
      gl_.VertexAttribIPointer(3, 1, GL_UNSIGNED_INT, sizeof(Instance), at(offsetof(Instance, flags)));
      gl_.DrawArraysInstanced(GL_TRIANGLE_STRIP, 0, 4, static_cast<GLsizei>(batch.count));
    }
  }

  std::string_view shader_error(uint64_t hash) const override {
    const auto it = programs_.find(hash);
    return it == programs_.end() ? std::string_view{} : std::string_view(it->second.error);
  }

  bool take_compiled() override { return std::exchange(compiled_, false); }

  void read_pixels(std::vector<uint32_t> &out) override {
    out.resize(static_cast<size_t>(width_) * static_cast<size_t>(height_));
    gl_.BindFramebuffer(GL_FRAMEBUFFER, offscreen_ ? fbo_ : 0);
    gl_.ReadPixels(0, 0, width_, height_, GL_RGBA, GL_UNSIGNED_BYTE, out.data());
    std::vector<uint32_t> row(static_cast<size_t>(width_));
    for (int y = 0; y < height_ / 2; ++y) {
      uint32_t *a = out.data() + static_cast<size_t>(y) * width_;
      uint32_t *b = out.data() + static_cast<size_t>(height_ - 1 - y) * width_;
      std::copy_n(a, width_, row.data());
      std::copy_n(b, width_, a);
      std::copy_n(row.data(), width_, b);
    }
  }

 private:
  struct Program {
    GLuint id = 0;
    GLint rect = -1, fb = -1, time = -1, resolution = -1;
    uint64_t used = 0;
    std::string error;
    std::unordered_map<std::string, std::pair<GLint, GLenum>> uniforms;
  };

  void bind_ui() {
    gl_.UseProgram(program_);
    gl_.Uniform2f(u_fb_, static_cast<float>(width_), static_cast<float>(height_));
    gl_.Uniform1i(u_atlas_, 0);
    gl_.ActiveTexture(GL_TEXTURE0);
    gl_.BlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA);
  }

  GLuint compile(GLenum type, const std::string &source, const char *name, std::string *error) {
    const GLuint shader = gl_.CreateShader(type);
    const char *text = source.c_str();
    gl_.ShaderSource(shader, 1, &text, nullptr);
    gl_.CompileShader(shader);
    GLint ok = 0;
    gl_.GetShaderiv(shader, GL_COMPILE_STATUS, &ok);
    if (!ok) {
      char log[2048] = {};
      gl_.GetShaderInfoLog(shader, sizeof(log), nullptr, log);
      std::fprintf(stderr, "qui: shader '%s' failed to compile, drawing its background instead:\n%s\n", name, log);
      if (error) *error = log;
      gl_.DeleteShader(shader);
      return 0;
    }
    return shader;
  }

  GLuint link(const char *vertex, const char *fragment, const char *name, std::string *error = nullptr) {
    const GLuint vs = compile(GL_VERTEX_SHADER, version_ + vertex, name, error);
    const GLuint fs = compile(GL_FRAGMENT_SHADER, version_ + fragment, name, error);
    if (!vs || !fs) {
      if (vs) gl_.DeleteShader(vs);
      if (fs) gl_.DeleteShader(fs);
      return 0;
    }
    const GLuint program = gl_.CreateProgram();
    gl_.AttachShader(program, vs);
    gl_.AttachShader(program, fs);
    gl_.LinkProgram(program);
    gl_.DeleteShader(vs);
    gl_.DeleteShader(fs);
    GLint ok = 0;
    gl_.GetProgramiv(program, GL_LINK_STATUS, &ok);
    if (!ok) {
      char log[2048] = {};
      gl_.GetProgramInfoLog(program, sizeof(log), nullptr, log);
      std::fprintf(stderr, "qui: shader '%s' failed to link: %s\n", name, log);
      if (error) *error = log;
      gl_.DeleteProgram(program);
      return 0;
    }
    return program;
  }

  Program &program_for(const ShaderDraw &draw) {
    auto [it, inserted] = programs_.try_emplace(draw.hash);
    Program &program = it->second;
    program.used = ++use_clock_;
    if (!inserted) return program;
    compiled_ = true;
    if (programs_.size() > kMaxPrograms) {
      auto oldest = programs_.begin();
      for (auto i = programs_.begin(); i != programs_.end(); ++i)
        if (i->second.used < oldest->second.used) oldest = i;
      if (oldest->second.id) gl_.DeleteProgram(oldest->second.id);
      programs_.erase(oldest);
    }
    const std::string fragment = std::string(kShaderHeader) + "#line 1\n" + draw.src + kShaderFooter;
    program.id = link(kShaderVertex, fragment.c_str(), "custom", &program.error);
    if (!program.id) return program;
    program.rect = gl_.GetUniformLocation(program.id, "qui_rect");
    program.fb = gl_.GetUniformLocation(program.id, "qui_fb");
    program.time = gl_.GetUniformLocation(program.id, "u_time");
    program.resolution = gl_.GetUniformLocation(program.id, "u_resolution");
    GLint count = 0;
    gl_.GetProgramiv(program.id, GL_ACTIVE_UNIFORMS, &count);
    for (GLint i = 0; i < count; ++i) {
      char name[128] = {};
      GLint size = 0;
      GLenum type = 0;
      gl_.GetActiveUniform(program.id, static_cast<GLuint>(i), sizeof(name), nullptr, &size, &type, name);
      program.uniforms[name] = {gl_.GetUniformLocation(program.id, name), type};
    }
    return program;
  }

  void draw_shader(const ShaderDraw &draw, double time) {
    Program &program = program_for(draw);
    if (!program.id) return;
    gl_.UseProgram(program.id);
    gl_.BlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA);
    gl_.Uniform4f(program.rect, static_cast<float>(draw.rect.x), static_cast<float>(draw.rect.y), static_cast<float>(draw.rect.w), static_cast<float>(draw.rect.h));
    gl_.Uniform2f(program.fb, static_cast<float>(width_), static_cast<float>(height_));
    if (program.time >= 0) gl_.Uniform1f(program.time, static_cast<float>(time));
    if (program.resolution >= 0) gl_.Uniform2f(program.resolution, static_cast<float>(draw.rect.w), static_cast<float>(draw.rect.h));
    for (const ShaderUniform &uniform : draw.uniforms) {
      const auto it = program.uniforms.find(uniform.name);
      if (it == program.uniforms.end()) continue;
      const GLint location = it->second.first;
      if (it->second.second == GL_INT) {
        gl_.Uniform1i(location, static_cast<GLint>(uniform.v[0]));
        continue;
      }
      switch (uniform.n) {
        case 1: gl_.Uniform1f(location, uniform.v[0]); break;
        case 2: gl_.Uniform2f(location, uniform.v[0], uniform.v[1]); break;
        case 3: gl_.Uniform3f(location, uniform.v[0], uniform.v[1], uniform.v[2]); break;
        case 4: gl_.Uniform4f(location, uniform.v[0], uniform.v[1], uniform.v[2], uniform.v[3]); break;
        default: break;
      }
    }
    gl_.DrawArrays(GL_TRIANGLE_STRIP, 0, 4);
  }

  Gl gl_;
  bool offscreen_;
  std::string version_;
  GLuint program_ = 0, vao_ = 0, vbo_ = 0, fbo_ = 0, color_ = 0;
  GLint u_fb_ = -1, u_atlas_ = -1;
  int width_ = 0, height_ = 0;
  std::vector<GLuint> pages_;
  std::unordered_map<uint64_t, Program> programs_;
  uint64_t use_clock_ = 0;
  bool compiled_ = false;
};

}  // namespace

std::unique_ptr<Backend> make_gl_backend(void *(*get_proc)(const char *), bool offscreen, bool es) {
  Gl gl;
  if (!gl.load(get_proc)) return nullptr;
  return std::make_unique<GlBackend>(gl, offscreen, es);
}

}  // namespace qui
