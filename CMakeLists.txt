cmake_minimum_required(VERSION 3.22)
project(qui LANGUAGES CXX)

find_package(PkgConfig REQUIRED)
pkg_check_modules(QUI_DEPS REQUIRED IMPORTED_TARGET sdl3 freetype2 harfbuzz fontconfig)

add_library(qui STATIC app.cpp cpu.cpp engine.cpp font_manager.cpp gl.cpp text_shaper.cpp)
target_compile_features(qui PUBLIC cxx_std_20)
target_include_directories(qui PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(qui PRIVATE PkgConfig::QUI_DEPS)

if(NOT CMAKE_BUILD_TYPE AND NOT CMAKE_CONFIGURATION_TYPES)
  set(CMAKE_BUILD_TYPE RelWithDebInfo CACHE STRING "Build type" FORCE)
endif()
