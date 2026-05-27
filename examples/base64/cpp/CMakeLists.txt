cmake_minimum_required(VERSION 3.14)
project(cpp_base64 CXX)

find_package(crossfuzz REQUIRED)

add_executable(cpp_target cpp_target.cpp)
target_link_libraries(cpp_target PRIVATE crossfuzz::cpp)
