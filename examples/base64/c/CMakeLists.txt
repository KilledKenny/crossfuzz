cmake_minimum_required(VERSION 3.14)
project(c_base64 C)

find_package(crossfuzz REQUIRED)

add_executable(c_target c_target.c)
target_link_libraries(c_target PRIVATE crossfuzz::c)
