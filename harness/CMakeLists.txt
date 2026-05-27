cmake_minimum_required(VERSION 3.14)
project(crossfuzz VERSION 0.0.1 LANGUAGES C CXX)

add_subdirectory(c)
add_subdirectory(cpp)

include(GNUInstallDirs)
include(CMakePackageConfigHelpers)

configure_package_config_file(
    cmake/crossfuzz-config.cmake.in
    ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-config.cmake
    INSTALL_DESTINATION ${CMAKE_INSTALL_LIBDIR}/cmake/crossfuzz
)

write_basic_package_version_file(
    ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-version.cmake
    VERSION ${PROJECT_VERSION}
    COMPATIBILITY SameMajorVersion
)

install(EXPORT crossfuzz-targets
    NAMESPACE crossfuzz::
    DESTINATION ${CMAKE_INSTALL_LIBDIR}/cmake/crossfuzz
)

install(FILES
    ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-config.cmake
    ${CMAKE_CURRENT_BINARY_DIR}/crossfuzz-version.cmake
    DESTINATION ${CMAKE_INSTALL_LIBDIR}/cmake/crossfuzz
)
