cmake_minimum_required(VERSION 3.18)
project(grimlocker-sdk-cpp VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Header-only interface library
add_library(grimlocker_sdk INTERFACE)
target_include_directories(grimlocker_sdk INTERFACE
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)

# Optional: find libcurl + nlohmann/json if available
find_package(CURL REQUIRED)
find_package(nlohmann_json 3.10 REQUIRED)
target_link_libraries(grimlocker_sdk INTERFACE CURL::libcurl nlohmann_json::nlohmann_json)

# Example
option(GRIMLOCKER_BUILD_EXAMPLES "Build example programs" OFF)
if(GRIMLOCKER_BUILD_EXAMPLES)
    add_executable(grimlocker_example example/main.cpp)
    target_link_libraries(grimlocker_example PRIVATE grimlocker_sdk)
endif()
