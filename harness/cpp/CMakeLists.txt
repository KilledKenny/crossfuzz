cmake_minimum_required(VERSION 3.14)

if(NOT TARGET crossfuzz::c)
    add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../c
                     ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz_c)
endif()

add_library(crossfuzz_cpp STATIC crossfuzz.cpp)
set_target_properties(crossfuzz_cpp PROPERTIES EXPORT_NAME cpp)
add_library(crossfuzz::cpp ALIAS crossfuzz_cpp)

target_include_directories(crossfuzz_cpp PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>
    $<INSTALL_INTERFACE:include/crossfuzz>
)

target_link_libraries(crossfuzz_cpp PUBLIC crossfuzz_c)
target_compile_features(crossfuzz_cpp PUBLIC cxx_std_23)

if(NOT PROJECT_VERSION)
    message(FATAL_ERROR "crossfuzz/cpp: PROJECT_VERSION not set — include this via harness/CMakeLists.txt")
endif()

include(GNUInstallDirs)

install(TARGETS crossfuzz_cpp
    EXPORT crossfuzz-targets
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
)

install(FILES crossfuzz.hpp
    DESTINATION ${CMAKE_INSTALL_INCLUDEDIR}/crossfuzz
)

configure_file(crossfuzz-cpp.pc.in crossfuzz-cpp.pc @ONLY)
install(FILES ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-cpp.pc
    DESTINATION ${CMAKE_INSTALL_LIBDIR}/pkgconfig
)
