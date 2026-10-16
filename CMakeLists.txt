cmake_minimum_required(VERSION 3.13)

# PICO_SDK_PATH must point at a checkout of raspberrypi/pico-sdk.
include($ENV{PICO_SDK_PATH}/external/pico_sdk_import.cmake)

project(dotmatrix C CXX ASM)
set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 17)

pico_sdk_init()

add_executable(dotmatrix main.c)
pico_generate_pio_header(dotmatrix ${CMAKE_CURRENT_LIST_DIR}/hub75.pio)
target_link_libraries(dotmatrix pico_stdlib hardware_gpio hardware_pio)

# USB serial is useful for the startup/status messages; no UART adapter needed.
pico_enable_stdio_usb(dotmatrix 1)
pico_enable_stdio_uart(dotmatrix 0)
pico_add_extra_outputs(dotmatrix)

add_executable(diagnostic diagnostic.c)
target_link_libraries(diagnostic pico_stdlib hardware_gpio)
if(PICO_BOARD STREQUAL "pico2_w")
    target_link_libraries(diagnostic pico_cyw43_arch_none)
endif()
pico_add_extra_outputs(diagnostic)

if(PICO_BOARD STREQUAL "pico2_w")
    add_executable(blink blink.c)
    target_link_libraries(blink pico_stdlib pico_cyw43_arch_none)
    pico_add_extra_outputs(blink)
endif()

# Bit-banged test that tries both Seengreat Pico pinouts (see hub75_test.c).
add_executable(hub75_test hub75_test.c)
target_link_libraries(hub75_test pico_stdlib hardware_gpio)
pico_enable_stdio_usb(hub75_test 1)
pico_enable_stdio_uart(hub75_test 0)
pico_add_extra_outputs(hub75_test)

# USB-serial framebuffer driven by the Go library/server in go/.
add_executable(matrix_fw matrix_fw.c)
target_link_libraries(matrix_fw pico_stdlib pico_multicore hardware_gpio)
pico_enable_stdio_usb(matrix_fw 1)
pico_enable_stdio_uart(matrix_fw 0)
pico_add_extra_outputs(matrix_fw)
