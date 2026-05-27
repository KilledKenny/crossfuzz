cmake_minimum_required(VERSION 3.14)

option(CROSSFUZZ_INSTRUMENT
    "Propagate -fsanitize-coverage=trace-pc-guard to targets that link this library"
    ON
)

add_library(crossfuzz_c STATIC crossfuzz.c)
set_target_properties(crossfuzz_c PROPERTIES EXPORT_NAME c)
add_library(crossfuzz::c ALIAS crossfuzz_c)

target_include_directories(crossfuzz_c PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>
    $<INSTALL_INTERFACE:include/crossfuzz>
)

if(CROSSFUZZ_INSTRUMENT)
    if(NOT CMAKE_C_COMPILER_ID MATCHES "Clang")
        message(WARNING
            "crossfuzz: -fsanitize-coverage=trace-pc-guard requires Clang. "
            "Pass -DCMAKE_C_COMPILER=clang to use coverage instrumentation.")
    endif()
    target_compile_options(crossfuzz_c PUBLIC
        -fsanitize-coverage=trace-pc-guard
    )
endif()

if(NOT PROJECT_VERSION)
    message(FATAL_ERROR "crossfuzz/c: PROJECT_VERSION not set — include this via harness/CMakeLists.txt")
endif()

include(GNUInstallDirs)

install(TARGETS crossfuzz_c
    EXPORT crossfuzz-targets
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
)

install(FILES crossfuzz.h
    DESTINATION ${CMAKE_INSTALL_INCLUDEDIR}/crossfuzz
)

configure_file(crossfuzz-c.pc.in crossfuzz-c.pc @ONLY)
install(FILES ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-c.pc
    DESTINATION ${CMAKE_INSTALL_LIBDIR}/pkgconfig
)
